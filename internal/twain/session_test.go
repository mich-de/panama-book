package twain

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

// fakeDSM e' un DSM/data source finto: registra le chiamate e simula le risposte, cosi' i test non toccano
// lo scanner reale. Memoria globale simulata con una tabella di slice.
type fakeDSM struct {
	ptr   int
	calls []string
	mem   map[uintptr][]byte
	next  uintptr

	caps      map[uint16][]byte // container OneValue correnti
	ranges    map[uint16][]byte // container Range restituiti da MSG_GET
	lastCC    uint16
	noDS      bool
	failCaps  map[uint16]uint16 // capability il cui SET fallisce -> CC
	ignoreTop bool              // il driver ignora l'alto del riquadro
	a4Limit   bool              // accetta un riquadro solo se ICAP_SUPPORTEDSIZES = NONE (0)
	defaults  map[uint16][]byte
	loaded    bool
	failXfer  bool
	pending   int16
	events    int // quanti PROCESSEVENT prima di XFERREADY
	evCount   int
	noEvents  bool // Next scade subito
	dib       []byte
	enableN   int
	xferN     int
}

func newFake(ptr int) *fakeDSM {
	f := &fakeDSM{ptr: ptr, mem: map[uintptr][]byte{}, next: 0x1000, caps: map[uint16][]byte{}, ranges: map[uint16][]byte{},
		failCaps: map[uint16]uint16{}, loaded: true, events: 2}
	one := func(ty uint16, v uint32) []byte { return oneValueBytes(ty, v, nil, ptr) }
	f.caps[capDuplexEnabled] = one(tyBool, 1) // il driver parte con duplex attivo: il codice deve spegnerlo
	f.caps[capXferCount] = one(tyInt16, 0xFFFF)
	f.caps[icapXferMech] = one(tyUint16, 1)
	f.caps[icapUnits] = one(tyUint16, 1)
	f.caps[icapPixelType] = one(tyUint16, 0)
	f.caps[icapXResolution] = one(tyFix32, fix32Encode(200))
	f.caps[icapYResolution] = one(tyFix32, fix32Encode(200))
	f.caps[capFeederEnabled] = one(tyBool, 0)
	f.caps[capFeederLoaded] = one(tyBool, 1)
	f.caps[icapPhysicalWidth] = one(tyFix32, fix32Encode(8.5))
	f.caps[icapPhysicalHeigh] = one(tyFix32, fix32Encode(14))
	fr := Frame{0, 0, 8.5, 14}
	f.caps[icapFrames] = oneValueBytes(tyFrame, 0, &fr, ptr)
	f.ranges[icapXResolution] = cat(u16(tyFix32), u32(fix32Encode(75)), u32(fix32Encode(600)), u32(fix32Encode(1)), u32(fix32Encode(200)), u32(fix32Encode(200)))
	f.caps[capSupportedCaps] = cat(u16(tyUint16), u32(3), u16(capDuplexEnabled), u16(icapFrames), u16(capExtPaperProtect))
	f.caps[icapSupportedSize] = one(tyUint16, 1)
	f.defaults = map[uint16][]byte{}
	for k, v := range f.caps {
		f.defaults[k] = append([]byte(nil), v...)
	}
	// DIB 2x2 a 24 bit
	f.dib = append(dibHeader(2, 2, 24, 0), make([]byte, 16)...)
	return f
}

func (f *fakeDSM) PtrSize() int    { return f.ptr }
func (f *fakeDSM) Parent() uintptr { return 0x77 }
func (f *fakeDSM) Alloc(n int) (uintptr, error) {
	h := f.next
	f.next += 0x10
	f.mem[h] = make([]byte, n)
	return h, nil
}
func (f *fakeDSM) Lock(h uintptr) []byte { return f.mem[h] }
func (f *fakeDSM) Unlock(h uintptr)      {}
func (f *fakeDSM) Free(h uintptr)        { delete(f.mem, h) }
func (f *fakeDSM) Dispatch(msg []byte)   {}
func (f *fakeDSM) Next(deadline time.Time) ([]byte, bool) {
	if f.noEvents {
		return nil, false
	}
	return make([]byte, 28), true
}

func (f *fakeDSM) put(data []byte, capID, con uint16, container []byte) {
	h, _ := f.Alloc(len(container))
	copy(f.mem[h], container)
	copy(data, layout{ptr: f.ptr}.encodeCapability(capID, con, h))
}

func (f *fakeDSM) fail(cc uint16) uint16 { f.lastCC = cc; return rcFailure }

func (f *fakeDSM) Entry(origin, dest []byte, dg uint32, dat, msg uint16, data []byte) uint16 {
	l := layout{ptr: f.ptr}
	f.calls = append(f.calls, fmt.Sprintf("%d/%04x/%04x", dg, dat, msg))
	switch {
	case dat == datParent:
		return rcSuccess
	case dat == datStatus:
		binary.LittleEndian.PutUint16(data, f.lastCC)
		return rcSuccess
	case dat == datIdentity:
		switch msg {
		case msgGetFirst:
			if f.noDS {
				return f.fail(3)
			}
			copy(data, newIdentity("PFU", "PaperStream", "PaperStream IP fi-800R"))
			return rcSuccess
		case msgGetNext:
			return rcEndOfList
		case msgOpenDS:
			binary.LittleEndian.PutUint32(data, 99)
			return rcSuccess
		case msgCloseDS:
			return rcSuccess
		}
	case dat == datCapability:
		id, _, h := l.decodeCapability(data)
		switch msg {
		case msgGet, msgGetCurrent, msgGetDefault:
			if id == capFeederLoaded {
				v := uint32(0)
				if f.loaded {
					v = 1
				}
				f.put(data, id, conOneValue, oneValueBytes(tyBool, v, nil, f.ptr))
				return rcSuccess
			}
			if msg == msgGet {
				if r, ok := f.ranges[id]; ok {
					f.put(data, id, conRange, r)
					return rcSuccess
				}
			}
			if id == capSupportedCaps {
				f.put(data, id, conArray, f.caps[id])
				return rcSuccess
			}
			c, ok := f.caps[id]
			if !ok {
				return f.fail(13)
			}
			f.put(data, id, conOneValue, c)
			return rcSuccess
		case msgQuerySupport:
			if _, ok := f.caps[id]; !ok {
				return f.fail(13)
			}
			f.put(data, id, conOneValue, oneValueBytes(tyUint32, 0x1F, nil, f.ptr))
			return rcSuccess
		case msgSet:
			if cc, bad := f.failCaps[id]; bad {
				return f.fail(cc)
			}
			c := append([]byte(nil), f.mem[h]...)
			if id == icapFrames && f.a4Limit {
				if sz, _ := parseContainer(f.caps[icapSupportedSize], conOneValue, f.ptr); len(sz.Items) == 1 && sz.Items[0].U != twssNone {
					return f.fail(10)
				}
			}
			if id == icapFrames && f.ignoreTop {
				fr := decodeFrame(c[2:])
				fr.Top = 0
				copy(c[2:], fr.encode())
			}
			f.caps[id] = c
			return rcSuccess
		case msgReset:
			d, ok := f.defaults[id]
			if !ok {
				return f.fail(13)
			}
			f.caps[id] = append([]byte(nil), d...)
			f.put(data, id, conOneValue, f.caps[id])
			return rcSuccess
		}
	case dat == datUserInterface && msg == msgEnableDS:
		f.enableN++
		return rcSuccess
	case dat == datUserInterface && msg == msgDisableDS:
		return rcSuccess
	case dat == datEvent:
		f.evCount++
		if f.evCount >= f.events {
			binary.LittleEndian.PutUint16(data[f.ptr:], msgXferReady)
			return rcDSEvent
		}
		return rcNotDSEvent
	case dat == datImageInfo:
		return rcSuccess
	case dat == datImageLayout:
		copy(data, Frame{0, 0, 8.5, 14}.encode())
		return rcSuccess
	case dat == datImageNative:
		f.xferN++
		if f.failXfer {
			return f.fail(20) // inceppamento
		}
		h, _ := f.Alloc(len(f.dib))
		copy(f.mem[h], f.dib)
		putPtr(data, 0, h, f.ptr)
		f.pending = 1
		return rcXferDone
	case dat == datPendingXfers && msg == msgEndXfer:
		binary.LittleEndian.PutUint16(data, uint16(f.pending))
		return rcSuccess
	case dat == datPendingXfers && msg == msgReset:
		f.pending = 0
		return rcSuccess
	}
	return f.fail(5)
}

func (f *fakeDSM) has(sub string) int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			n++
		}
	}
	return n
}

func (f *fakeDSM) index(call string) int {
	for i, c := range f.calls {
		if c == call {
			return i
		}
	}
	return -1
}

var (
	callEnable   = fmt.Sprintf("%d/%04x/%04x", dgControl, datUserInterface, msgEnableDS)
	callXfer     = fmt.Sprintf("%d/%04x/%04x", dgImage, datImageNative, msgGet)
	callEndXfer  = fmt.Sprintf("%d/%04x/%04x", dgControl, datPendingXfers, msgEndXfer)
	callResetPnd = fmt.Sprintf("%d/%04x/%04x", dgControl, datPendingXfers, msgReset)
	callCloseDS  = fmt.Sprintf("%d/%04x/%04x", dgControl, datIdentity, msgCloseDS)
	callCloseDSM = fmt.Sprintf("%d/%04x/%04x", dgControl, datParent, msgCloseDSM)
)

func runScan(f *fakeDSM, o Options) (int, error) {
	o, err := o.normalized()
	if err != nil {
		return 0, err
	}
	s := newSession(f)
	s.noDSHint = "cercato in C:\\Windows\\twain_64"
	img, err := s.scan(o, time.Now().Add(time.Minute))
	if err != nil {
		return 0, err
	}
	return img.Bounds().Dx(), nil
}

func TestScanSequenceBothPointerSizes(t *testing.T) {
	for _, ptr := range []int{4, 8} {
		f := newFake(ptr)
		w, err := runScan(f, Options{DPI: 300})
		if err != nil {
			t.Fatalf("ptr=%d: %v", ptr, err)
		}
		if w != 2 {
			t.Errorf("ptr=%d: immagine larga %d", ptr, w)
		}
		if f.enableN != 1 || f.xferN != 1 {
			t.Errorf("ptr=%d: ENABLEDS=%d transfer=%d, attesi 1 e 1", ptr, f.enableN, f.xferN)
		}
		// il duplex e' stato spento e riletto PRIMA dell'avvio
		if v, _ := (Container{Con: conOneValue, Type: tyBool, Items: []Item{{U: uint32(f.caps[capDuplexEnabled][2])}}}).Current(); v != 0 {
			t.Errorf("ptr=%d: duplex ancora attivo", ptr)
		}
		// risoluzione e unita' impostate; una sola pagina
		c, _ := parseContainer(f.caps[icapXResolution], conOneValue, ptr)
		if v, _ := c.Current(); v != 300 {
			t.Errorf("ptr=%d: risoluzione %v", ptr, v)
		}
		if f.pending != 0 {
			t.Errorf("ptr=%d: pagine in coda non scartate", ptr)
		}
		if f.has(callResetPnd) == 0 || f.index(callEndXfer) < 0 {
			t.Errorf("ptr=%d: ENDXFER/RESET mancanti: %v", ptr, f.calls)
		}
		// chiusura in ordine: DS poi DSM, dopo il trasferimento
		if !(f.index(callXfer) < f.index(callCloseDS) && f.index(callCloseDS) < f.index(callCloseDSM)) {
			t.Errorf("ptr=%d: ordine di chiusura errato: %v", ptr, f.calls)
		}
	}
}

func TestScanSetsDuplexBeforeEnabling(t *testing.T) {
	f := newFake(4)
	if _, err := runScan(f, Options{}); err != nil {
		t.Fatal(err)
	}
	// la prima SET e' sul duplex e precede ENABLEDS
	setCall := fmt.Sprintf("%d/%04x/%04x", dgControl, datCapability, msgSet)
	if f.index(setCall) < 0 || f.index(setCall) > f.index(callEnable) {
		t.Errorf("ordine: %v", f.calls)
	}
}

func TestScanDuplexRefusedNoAcquisition(t *testing.T) {
	f := newFake(4)
	f.failCaps[capDuplexEnabled] = 13
	_, err := runScan(f, Options{})
	if err == nil || !strings.Contains(err.Error(), "una sola facciata") {
		t.Fatalf("errore atteso, avuto %v", err)
	}
	if f.enableN != 0 || f.xferN != 0 {
		t.Error("lo scanner non doveva partire")
	}
	if f.index(callCloseDS) < 0 || f.index(callCloseDSM) < 0 {
		t.Error("DS e DSM vanno chiusi anche in caso di errore")
	}
}

func TestScanFrameAccepted(t *testing.T) {
	f := newFake(8)
	if _, err := runScan(f, Options{StartMM: 91, HeightMM: 87}); err != nil {
		t.Fatal(err)
	}
	c, _ := parseContainer(f.caps[icapFrames], conOneValue, 8)
	fr, _ := c.CurrentFrame()
	if absf(fr.Top*mmPerInch-91) > 0.1 || absf(fr.Bottom*mmPerInch-178) > 0.1 {
		t.Errorf("riquadro impostato: alto %.2f mm basso %.2f mm", fr.Top*mmPerInch, fr.Bottom*mmPerInch)
	}
	if f.enableN != 1 {
		t.Error("scansione non avviata")
	}
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestScanDriverIgnoresTopStopsBeforeScanning(t *testing.T) {
	f := newFake(4)
	f.ignoreTop = true
	_, err := runScan(f, Options{StartMM: 91, HeightMM: 87})
	if err == nil || !strings.Contains(err.Error(), "non ha accettato nessun riquadro") {
		t.Fatalf("errore atteso, avuto %v", err)
	}
	if f.enableN != 0 {
		t.Error("lo scanner non doveva partire con un riquadro diverso da quello chiesto")
	}
}

func TestScanFrameNeedsPaperSizeNone(t *testing.T) {
	// il driver accetta il riquadro solo dopo ICAP_SUPPORTEDSIZES=NONE: la prima ricetta fallisce, viene
	// ripristinata, la seconda riesce e viene tenuta
	f := newFake(4)
	f.a4Limit = true
	s := newSession(f)
	o, _ := Options{StartMM: 91, HeightMM: 87}.normalized()
	if _, err := s.scan(o, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.recipeUsed, "NONE") {
		t.Errorf("ricetta usata: %q", s.recipeUsed)
	}
	c, _ := parseContainer(f.caps[icapFrames], conOneValue, 4)
	fr, _ := c.CurrentFrame()
	if absf(fr.Top*mmPerInch-91) > 0.1 {
		t.Errorf("alto %.2f mm", fr.Top*mmPerInch)
	}
	if f.enableN != 1 {
		t.Error("scansione non avviata")
	}
}

func TestScanFeederEmpty(t *testing.T) {
	f := newFake(4)
	f.loaded = false
	_, err := runScan(f, Options{})
	if !IsPaperEmpty(err) || f.enableN != 0 {
		t.Fatalf("alimentatore vuoto: err=%v enable=%d", err, f.enableN)
	}
	// con IgnoreFeederLoaded si procede
	f = newFake(4)
	f.loaded = false
	if _, err := runScan(f, Options{IgnoreFeederLoaded: true}); err != nil || f.enableN != 1 {
		t.Errorf("IgnoreFeederLoaded: err=%v enable=%d", err, f.enableN)
	}
}

func TestScanTransferFailureNoRetry(t *testing.T) {
	f := newFake(4)
	f.failXfer = true
	_, err := runScan(f, Options{})
	if !IsPaperJam(err) {
		t.Fatalf("inceppamento atteso, avuto %v", err)
	}
	if f.xferN != 1 {
		t.Errorf("il transfer e' stato ritentato: %d chiamate", f.xferN)
	}
	if f.index(callEndXfer) < 0 || f.index(callCloseDS) < 0 || f.index(callCloseDSM) < 0 {
		t.Errorf("chiusura incompleta: %v", f.calls)
	}
}

func TestScanResolutionOutOfRange(t *testing.T) {
	f := newFake(4)
	_, err := runScan(f, Options{DPI: 1000})
	if err == nil || !strings.Contains(err.Error(), "non ammessa") {
		t.Fatalf("errore atteso, avuto %v", err)
	}
	if f.enableN != 0 {
		t.Error("lo scanner non doveva partire")
	}
}

func TestScanTimeout(t *testing.T) {
	f := newFake(4)
	f.noEvents = true
	_, err := runScan(f, Options{})
	if !errorsIs(err, ErrTimeout) {
		t.Fatalf("timeout atteso, avuto %v", err)
	}
	if f.xferN != 0 {
		t.Error("nessun trasferimento dopo il timeout")
	}
}

func TestScanNoDataSource(t *testing.T) {
	f := newFake(8)
	f.noDS = true
	_, err := runScan(f, Options{})
	if err == nil || !strings.Contains(err.Error(), "nessun data source TWAIN installato") || !strings.Contains(err.Error(), "twain_64") {
		t.Fatalf("messaggio errato: %v", err)
	}
}

func TestScanDataSourceNameMismatch(t *testing.T) {
	f := newFake(4)
	_, err := runScan(f, Options{DeviceMatch: "altro-scanner"})
	if err == nil || !strings.Contains(err.Error(), "altro-scanner") || !strings.Contains(err.Error(), "fi-800R") {
		t.Fatalf("messaggio errato: %v", err)
	}
}

func TestProbeNeverStartsScanner(t *testing.T) {
	for _, try := range []bool{false, true} {
		f := newFake(4)
		s := newSession(f)
		var out []string
		if err := s.probe("fi-800R", ProbeOptions{TrySetFrame: try}, &out); err != nil {
			t.Fatal(err)
		}
		if f.enableN != 0 || f.xferN != 0 || f.has(callEnable) != 0 {
			t.Errorf("try=%v: Probe ha avviato lo scanner", try)
		}
		text := strings.Join(out, "\n")
		for _, want := range []string{"capability supportate: 3", "CAP_EXT_PAPERPROTECTION", "ICAP_FRAMES"} {
			if !strings.Contains(text, want) {
				t.Errorf("try=%v: manca %q nell'output:\n%s", try, want, text)
			}
		}
		if try && !strings.Contains(text, "ESITO: SI") {
			t.Errorf("manca l'esito della prova di scrittura:\n%s", text)
		}
		if f.index(callCloseDSM) < 0 {
			t.Error("DSM non chiuso")
		}
	}
}

func TestProbeTryFramesStopsAtFirstOKAndRestores(t *testing.T) {
	f := newFake(4)
	f.a4Limit = true
	s := newSession(f)
	var out []string
	if err := s.probe("fi-800R", ProbeOptions{TrySetFrame: true}, &out); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(out, "\n")
	if !strings.Contains(text, "prova 1:") || !strings.Contains(text, "ESITO: NO") || !strings.Contains(text, "ESITO: SI") ||
		!strings.Contains(text, "RISULTATO: prima ricetta accettata: ICAP_SUPPORTEDSIZES=NONE") {
		t.Errorf("output inatteso:\n%s", text)
	}
	if strings.Contains(text, "prova 3:") {
		t.Errorf("senza TryAll doveva fermarsi alla prima accettata:\n%s", text)
	}
	// ripristino: il formato e il riquadro tornano ai predefiniti
	c, _ := parseContainer(f.caps[icapSupportedSize], conOneValue, 4)
	if v, _ := c.Current(); v != 1 {
		t.Errorf("ICAP_SUPPORTEDSIZES non ripristinato: %v", v)
	}
	c, _ = parseContainer(f.caps[icapFrames], conOneValue, 4)
	if fr, _ := c.CurrentFrame(); fr.Top != 0 {
		t.Errorf("riquadro non ripristinato: %+v", fr)
	}
	if f.enableN != 0 || f.xferN != 0 {
		t.Error("Probe ha avviato lo scanner")
	}
	// DAT_IMAGELAYOUT (DG_IMAGE) non usato senza il flag
	if f.has(fmt.Sprintf("%d/%04x/", dgImage, datImageLayout)) != 0 {
		t.Error("DG_IMAGE usato senza ProbeOptions.Layout")
	}
	// con TryAll prova anche le altre ricette
	f = newFake(4)
	f.a4Limit = true
	s = newSession(f)
	out = nil
	_ = s.probe("fi-800R", ProbeOptions{TrySetFrame: true, TryAll: true}, &out)
	if !strings.Contains(strings.Join(out, "\n"), "prova 3:") {
		t.Error("TryAll non ha continuato")
	}
}

func TestProbeTrySetFrameDetectsIgnoredTop(t *testing.T) {
	f := newFake(4)
	f.ignoreTop = true
	s := newSession(f)
	var out []string
	if err := s.probe("fi-800R", ProbeOptions{TrySetFrame: true}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(out, "\n"), "RISULTATO: nessuna ricetta") {
		t.Errorf("esito atteso 'NON ha accettato':\n%s", strings.Join(out, "\n"))
	}
	if f.enableN != 0 {
		t.Error("Probe ha avviato lo scanner")
	}
}

func TestErrorMessages(t *testing.T) {
	e := &Error{Op: "prova", RC: rcFailure, CC: ccNoMedia}
	if !strings.Contains(e.Error(), "carta assente") || !IsPaperEmpty(e) || IsPaperJam(e) {
		t.Errorf("%v", e)
	}
	e = &Error{Op: "prova", RC: rcFailure, CC: ccPaperDoubleFeed}
	if !IsPaperJam(e) || !strings.Contains(e.Error(), "alimentazione multipla") {
		t.Errorf("%v", e)
	}
	e = &Error{Op: "prova", RC: rcBusy}
	if !strings.Contains(e.Error(), "occupato") {
		t.Errorf("%v", e)
	}
	for cc, m := range ccMessages {
		if m == "" {
			t.Errorf("messaggio vuoto per CC %d", cc)
		}
	}
}
