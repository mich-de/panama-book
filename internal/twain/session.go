package twain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
	"time"
	"unsafe"
)

// transport e' il confine con il sistema: DSM_Entry, memoria globale e pompa dei messaggi di Windows.
// L'implementazione reale e' in win.go; i test usano un DSM finto, cosi' nessun test tocca lo scanner.
type transport interface {
	// Entry chiama DSM_Entry(origin, dest, dg, dat, msg, data). dest puo' essere nil (= DSM). data e' la
	// memoria della struttura (puo' essere nil); il suo contenuto viene letto/scritto dal DSM.
	Entry(origin, dest []byte, dg uint32, dat, msg uint16, data []byte) uint16
	Alloc(size int) (uintptr, error) // memoria globale mobile azzerata (GlobalAlloc)
	Lock(h uintptr) []byte           // visione sulla memoria dell'handle, lunga quanto l'allocazione
	Unlock(h uintptr)
	Free(h uintptr)
	Parent() uintptr                               // finestra nascosta usata come genitore
	Next(deadline time.Time) (msg []byte, ok bool) // prossimo messaggio di Windows; ok=false se scade il tempo
	Dispatch(msg []byte)                           // TranslateMessage + DispatchMessage per i messaggi non del DS
	PtrSize() int                                  // 4 o 8
}

type session struct {
	t          transport
	l          layout
	app        []byte // identita' dell'applicazione
	ds         []byte // identita' del data source aperto (nil se chiuso)
	dsmOpen    bool
	noDSHint   string // dove e' stato cercato il data source, per il messaggio d'errore
	parent     []byte // finestra passata a OPENDSM (da riusare in CLOSEDSM)
	frameDef   *Frame // riquadro predefinito, per ripristinarlo
	recipeUsed string // ricetta di ICAP_FRAMES accettata dal driver (vuota se nessuna area richiesta)
}

func newSession(t transport) *session {
	return &session{t: t, l: layout{ptr: t.PtrSize()}, app: newIdentity("scanlibretto", "libretto", "libretto")}
}

// call esegue una chiamata. Restituisce il codice di ritorno; l'errore e' non nil per rc di fallimento
// (FAILURE, INFONOTSUPPORTED, DATANOTAVAILABLE, BUSY, SCANNERLOCKED), con la condizione letta da DAT_STATUS.
func (s *session) call(op string, dest []byte, dg uint32, dat, msg uint16, data []byte) (uint16, error) {
	rc := s.t.Entry(s.app, dest, dg, dat, msg, data)
	switch rc {
	case rcFailure, rcInfoNotSupport, rcDataNotAvail, rcBusy, rcScannerLocked:
		e := &Error{Op: op, RC: rc}
		if rc == rcFailure {
			st := make([]byte, statusSize)
			if s.t.Entry(s.app, dest, dgControl, datStatus, msgGet, st) == rcSuccess {
				e.CC = binary.LittleEndian.Uint16(st)
			}
		}
		return rc, e
	}
	return rc, nil
}

// ---- DSM e data source ----

func (s *session) openDSM() error {
	s.parent = make([]byte, s.l.ptr)
	putPtr(s.parent, 0, s.t.Parent(), s.l.ptr)
	if _, err := s.call("apertura del DSM TWAIN", nil, dgControl, datParent, msgOpenDSM, s.parent); err != nil {
		return err
	}
	s.dsmOpen = true
	return nil
}

func (s *session) closeDSM() {
	if s.dsmOpen {
		_, _ = s.call("chiusura del DSM TWAIN", nil, dgControl, datParent, msgCloseDSM, s.parent)
		s.dsmOpen = false
	}
}

// listDS elenca i data source installati (senza aprirli, senza interfaccia).
func (s *session) listDS() ([][]byte, error) {
	var out [][]byte
	msg := uint16(msgGetFirst)
	for i := 0; i < 64; i++ {
		id := make([]byte, identitySize)
		rc, err := s.call("elenco dei data source", nil, dgControl, datIdentity, msg, id)
		var te *Error
		if errors.As(err, &te) && i == 0 && te.CC == 3 { // TWCC_NODS: nessun data source
			return nil, nil
		}
		if err != nil {
			return out, err
		}
		if rc == rcEndOfList {
			break
		}
		out = append(out, id)
		msg = msgGetNext
	}
	return out, nil
}

// openDS apre il primo data source il cui nome contiene match (senza distinguere maiuscole).
func (s *session) openDS(match string) error {
	list, err := s.listDS()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("twain: nessun data source TWAIN installato (%s)", s.noDSHint)
	}
	var names []string
	var chosen []byte
	for _, id := range list {
		n := identityProduct(id)
		names = append(names, n)
		if chosen == nil && strings.Contains(strings.ToLower(n+" "+identityFamily(id)), strings.ToLower(match)) {
			chosen = id
		}
	}
	if chosen == nil {
		return fmt.Errorf("twain: nessun data source contiene %q (trovati: %q; %s)", match, names, s.noDSHint)
	}
	if _, err := s.call("apertura dello scanner", nil, dgControl, datIdentity, msgOpenDS, chosen); err != nil {
		return err
	}
	s.ds = chosen
	return nil
}

func (s *session) closeDS() {
	if s.ds != nil {
		_, _ = s.call("chiusura dello scanner", nil, dgControl, datIdentity, msgCloseDS, s.ds)
		s.ds = nil
	}
}

// ---- capability ----

func capLabel(id uint16) string {
	if n, ok := capNames[id]; ok {
		return fmt.Sprintf("%s (0x%04X)", n, id)
	}
	return fmt.Sprintf("capability 0x%04X", id)
}

// getCap legge una capability (msg = msgGet, msgGetCurrent, msgGetDefault o msgQuerySupport).
func (s *session) getCap(id uint16, msg uint16) (Container, error) {
	buf := s.l.encodeCapability(id, conDontCare, 0)
	if _, err := s.call("lettura di "+capLabel(id), s.ds, dgControl, datCapability, msg, buf); err != nil {
		return Container{}, err
	}
	_, con, h := s.l.decodeCapability(buf)
	if h == 0 {
		return Container{}, fmt.Errorf("twain: %s: il driver non ha restituito il valore", capLabel(id))
	}
	mem := s.t.Lock(h)
	c, err := parseContainer(mem, con, s.l.ptr)
	s.t.Unlock(h)
	s.t.Free(h)
	if err != nil {
		return Container{}, fmt.Errorf("twain: %s: %w", capLabel(id), err)
	}
	return c, nil
}

// putCap imposta una capability con un container OneValue (la memoria del container resta a carico nostro).
func (s *session) putCap(id uint16, ty uint16, item uint32, frame *Frame) error {
	data := oneValueBytes(ty, item, frame, s.l.ptr)
	h, err := s.t.Alloc(len(data))
	if err != nil {
		return err
	}
	defer s.t.Free(h)
	mem := s.t.Lock(h)
	copy(mem, data)
	s.t.Unlock(h)
	buf := s.l.encodeCapability(id, conOneValue, h)
	_, err = s.call("impostazione di "+capLabel(id), s.ds, dgControl, datCapability, msgSet, buf)
	return err
}

// setVerify imposta un valore numerico e lo rilegge: se il driver ne tiene un altro e' un errore.
func (s *session) setVerify(id uint16, ty uint16, v float64) error {
	item := uint32(int64(v))
	if ty == tyFix32 {
		item = fix32Encode(v)
	}
	if err := s.putCap(id, ty, item, nil); err != nil {
		return err
	}
	c, err := s.getCap(id, msgGetCurrent)
	if err != nil {
		return err
	}
	got, ok := c.Current()
	if !ok || math.Abs(got-v) > 0.01 {
		return fmt.Errorf("twain: il driver non ha accettato %s=%g (letto %v)", capLabel(id), v, c)
	}
	return nil
}

func framesClose(a, b Frame, tol float64) bool {
	return math.Abs(a.Left-b.Left) <= tol && math.Abs(a.Top-b.Top) <= tol &&
		math.Abs(a.Right-b.Right) <= tol && math.Abs(a.Bottom-b.Bottom) <= tol
}

// fieldMM legge il campo di scansione in mm da ICAP_PHYSICALWIDTH/HEIGHT, che il driver esprime nell'unita'
// corrente (scale = unita' per pollice: 1 pollici, 2,54 cm, dpi pixel); se non leggibile usa 8,5x14 pollici.
func (s *session) fieldMM(scale float64) (w, h float64) {
	w, h = defaultFieldWmm, defaultFieldHmm
	if c, err := s.getCap(icapPhysicalWidth, msgGetCurrent); err == nil {
		if v, ok := c.Current(); ok && v > 1 {
			w = unitsToMM(v, scale)
		}
	}
	if c, err := s.getCap(icapPhysicalHeigh, msgGetCurrent); err == nil {
		if v, ok := c.Current(); ok && v > 1 {
			h = unitsToMM(v, scale)
		}
	}
	return
}

// unitsToMM converte un valore nell'unita' corrente in millimetri (scale = unita' per pollice).
func unitsToMM(v, scale float64) float64 { return v / scale * mmPerInch }

// ---- scansione ----

// prepare imposta tutto cio' che serve PRIMA di avviare lo scanner. Nessun motore si avvia qui.
func (s *session) prepare(o Options) error {
	// una sola facciata: il duplex deve risultare spento, altrimenti non si procede
	if err := s.setVerify(capDuplexEnabled, tyBool, 0); err != nil {
		return fmt.Errorf("impossibile garantire una sola facciata, scansione annullata: %w", err)
	}
	if err := s.setVerify(capXferCount, tyInt16, 1); err != nil {
		return fmt.Errorf("impossibile limitare la scansione a una pagina: %w", err)
	}
	for _, st := range []struct {
		id uint16
		ty uint16
		v  float64
	}{{icapXferMech, tyUint16, xferMechN}, {icapUnits, tyUint16, unitInch}} {
		if err := s.setVerify(st.id, st.ty, st.v); err != nil {
			return err
		}
	}
	px := float64(pixelRGB)
	if o.Gray {
		px = pixelGray
	}
	if err := s.setVerify(icapPixelType, tyUint16, px); err != nil {
		return err
	}
	// risoluzione: controllo contro l'intervallo dichiarato, prima di impostarla
	if c, err := s.getCap(icapXResolution, msgGet); err == nil && !c.Allows(float64(o.DPI)) {
		return fmt.Errorf("risoluzione %d dpi non ammessa dal driver (%v)", o.DPI, c)
	}
	for _, id := range []uint16{icapXResolution, icapYResolution} {
		if err := s.setVerify(id, tyFix32, float64(o.DPI)); err != nil {
			return err
		}
	}
	if err := s.setVerify(capFeederEnabled, tyBool, 1); err != nil {
		return err
	}
	// facoltativi: il ritaglio/raddrizzamento automatico altererebbe la geometria; l'indicatore e' solo una finestra
	for _, id := range []uint16{icapAutoBorder, icapAutoDeskew, icapAutoRotate, capIndicators} {
		_ = s.setVerify(id, tyBool, 0)
	}
	if !o.IgnoreFeederLoaded {
		if c, err := s.getCap(capFeederLoaded, msgGetCurrent); err == nil {
			if v, ok := c.Current(); ok && v == 0 {
				return &Error{Op: "controllo dell'alimentatore", RC: rcFailure, CC: ccNoMedia}
			}
		}
	}
	if o.areaRequested() {
		// il campo (mm) dipende dalla ricetta: setAreaFrame lo ricava e prova le configurazioni in ordine
		if err := s.setAreaFrame(o); err != nil {
			return err
		}
	}
	return nil
}

// scan e' l'intera sequenza: DSM, data source, preparazione, acquisizione. Il transfer non viene mai ritentato.
func (s *session) scan(o Options, deadline time.Time) (image.Image, error) {
	if err := s.openDSM(); err != nil {
		return nil, err
	}
	defer s.closeDSM()
	if err := s.openDS(o.DeviceMatch); err != nil {
		return nil, err
	}
	defer s.closeDS()
	if err := s.prepare(o); err != nil {
		return nil, err
	}
	return s.enableAndTransfer(deadline)
}

// enableAndTransfer e' l'UNICO punto in cui lo scanner parte (MSG_ENABLEDS).
func (s *session) enableAndTransfer(deadline time.Time) (image.Image, error) {
	ui := s.l.encodeUI(false, false, s.t.Parent()) // nessuna finestra del driver
	if _, err := s.call("avvio della scansione", s.ds, dgControl, datUserInterface, msgEnableDS, ui); err != nil {
		return nil, err
	}
	defer func() {
		_, _ = s.call("arresto della scansione", s.ds, dgControl, datUserInterface, msgDisableDS, ui)
	}()
	for {
		raw, ok := s.t.Next(deadline)
		if !ok {
			return nil, fmt.Errorf("%w in attesa dell'immagine dallo scanner", ErrTimeout)
		}
		rc, twMsg, err := s.processEvent(raw)
		if err != nil {
			return nil, err
		}
		if rc == rcNotDSEvent {
			s.t.Dispatch(raw)
			continue
		}
		switch twMsg {
		case msgXferReady:
			return s.transfer()
		case msgCloseDSReq, msgCloseDSOK:
			return nil, errors.New("twain: il driver ha chiesto di chiudere lo scanner prima dell'immagine")
		}
	}
}

// processEvent passa un messaggio di Windows al DSM (DAT_EVENT/MSG_PROCESSEVENT).
func (s *session) processEvent(raw []byte) (rc, twMsg uint16, err error) {
	ev := s.l.encodeEvent(uintptr(unsafe.Pointer(&raw[0])))
	rc, err = s.call("elaborazione eventi", s.ds, dgControl, datEvent, msgProcessEvent, ev)
	if err != nil {
		return rc, 0, err
	}
	return rc, s.l.eventMessage(ev), nil
}

// transfer scarica l'immagine (trasferimento nativo DIB). Una sola prova: niente tentativi ripetuti.
func (s *session) transfer() (image.Image, error) {
	info := make([]byte, imageInfoSize)
	_, _ = s.call("lettura delle informazioni dell'immagine", s.ds, dgImage, datImageInfo, msgGet, info)

	hbuf := make([]byte, s.l.ptr)
	rc, err := s.call("trasferimento dell'immagine", s.ds, dgImage, datImageNative, msgGet, hbuf)
	var img image.Image
	var derr error
	if err == nil && rc == rcXferDone {
		if h := getPtr(hbuf, 0, s.l.ptr); h != 0 {
			img, derr = decodeDIB(s.t.Lock(h))
			s.t.Unlock(h)
			s.t.Free(h)
		} else {
			derr = errors.New("twain: il driver non ha restituito l'immagine")
		}
	}
	// chiude il trasferimento e scarta eventuali altre pagine in coda (una sola pagina richiesta)
	pend := make([]byte, pendingSize)
	if _, perr := s.call("fine del trasferimento", s.ds, dgControl, datPendingXfers, msgEndXfer, pend); perr == nil {
		if int16(binary.LittleEndian.Uint16(pend)) != 0 {
			_, _ = s.call("scarto delle pagine in coda", s.ds, dgControl, datPendingXfers, msgReset, make([]byte, pendingSize))
		}
	}
	switch {
	case err != nil:
		return nil, err
	case rc == rcCancel:
		return nil, errors.New("twain: trasferimento annullato dal driver o dallo scanner")
	case rc != rcXferDone:
		return nil, fmt.Errorf("twain: trasferimento terminato con codice inatteso %d", rc)
	case derr != nil:
		return nil, derr
	}
	return img, nil
}
