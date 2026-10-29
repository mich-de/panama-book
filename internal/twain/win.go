//go:build windows

package twain

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// ptrSize e' 4 su GOARCH=386 e 8 su amd64.
const ptrSize = int(unsafe.Sizeof(uintptr(0)))

// dsmCandidates elenca i percorsi in cui cercare il DSM (TWAINDSM.dll) dello stesso numero di bit del processo.
// 32 bit: SysWOW64 (Windows a 64 bit) o System32 (Windows a 32 bit), poi il vecchio DSM 1.x twain_32.dll.
// 64 bit: solo System32 (installato da 'PaperStream IP (TWAIN x64)' o da un DSM 2.x a 64 bit).
func dsmCandidates(winDir string, ptr int) []string {
	if ptr == 4 {
		return []string{
			filepath.Join(winDir, "SysWOW64", "TWAINDSM.dll"),
			filepath.Join(winDir, "System32", "TWAINDSM.dll"),
			filepath.Join(winDir, "twain_32.dll"),
		}
	}
	return []string{filepath.Join(winDir, "System32", "TWAINDSM.dll")}
}

// dsDir e' la cartella dei data source dello stesso numero di bit.
func dsDir(winDir string, ptr int) string {
	if ptr == 4 {
		return filepath.Join(winDir, "twain_32")
	}
	return filepath.Join(winDir, "twain_64")
}

// dsmMissingMessage e' il messaggio (italiano) quando il DSM del numero di bit del processo non si trova.
func dsmMissingMessage(winDir string, ptr int, cands []string, lastErr error) string {
	msg := fmt.Sprintf("DSM TWAIN a %d bit (TWAINDSM.dll) non trovato; cercato in: %v", ptr*8, cands)
	if ptr == 8 {
		msg += "; il driver installato e' a 32 bit: usare twainprobe32.exe oppure installare 'PaperStream IP (TWAIN x64)' (crea " +
			dsDir(winDir, 8) + " e il DSM a 64 bit)"
	}
	if lastErr != nil {
		msg += fmt.Sprintf(" (ultimo errore di caricamento: %v)", lastErr)
	}
	return msg
}

// noDSMessage e' l'indicazione di dove si e' cercato il data source (aggiunta all'errore "nessun data source").
func noDSMessage(winDir string, ptr int) string {
	m := fmt.Sprintf("cercato in %s; DSM %s", dsDir(winDir, ptr), dsmCandidates(winDir, ptr)[0])
	if ptr == 8 {
		m += "; il driver PaperStream IP installato e' a 32 bit: serve 'PaperStream IP (TWAIN x64)' oppure la versione a 32 bit di questo programma"
	}
	return m
}

func windowsDir() string {
	if d := os.Getenv("SystemRoot"); d != "" {
		return d
	}
	if d := os.Getenv("windir"); d != "" {
		return d
	}
	return `C:\Windows`
}

// ArchitectureHint descrive il numero di bit del processo e dove servono DSM e data source (messaggio italiano).
func ArchitectureHint() string {
	w := windowsDir()
	return fmt.Sprintf("processo a %d bit: serve il DSM %s e un data source in %s", ptrSize*8,
		dsmCandidates(w, ptrSize)[0], dsDir(w, ptrSize))
}

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	pGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	pGlobalLock   = kernel32.NewProc("GlobalLock")
	pGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	pGlobalFree   = kernel32.NewProc("GlobalFree")
	pGlobalSize   = kernel32.NewProc("GlobalSize")

	pCreateWindowEx  = user32.NewProc("CreateWindowExW")
	pDestroyWindow   = user32.NewProc("DestroyWindow")
	pPeekMessage     = user32.NewProc("PeekMessageW")
	pTranslateMsg    = user32.NewProc("TranslateMessage")
	pDispatchMessage = user32.NewProc("DispatchMessageW")
	pMsgWait         = user32.NewProc("MsgWaitForMultipleObjects")
)

const (
	gmemMoveableZero = 0x0002 | 0x0040 // GMEM_MOVEABLE | GMEM_ZEROINIT
	wsPopup          = 0x80000000
	pmRemove         = 1
	qsAllInput       = 0x04FF
)

// winMSG ha lo stesso layout di MSG di Windows (allineamento naturale, uguale a 32 e 64 bit).
type winMSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
}

// realTransport lega il pacchetto a TWAINDSM.dll e a una finestra nascosta; va usato su un solo thread del sistema.
type realTransport struct {
	dll  *syscall.DLL
	proc *syscall.Proc
	hwnd uintptr
	size map[uintptr]int
}

// openTransport carica il DSM dello stesso numero di bit del processo e crea la finestra nascosta.
// Deve essere chiamata dopo runtime.LockOSThread, sul thread che farà tutte le chiamate TWAIN.
func openTransport() (*realTransport, error) {
	w := windowsDir()
	cands := dsmCandidates(w, ptrSize)
	var lastErr error
	var dll *syscall.DLL
	for _, c := range cands {
		if _, err := os.Stat(c); err != nil {
			continue
		}
		d, err := syscall.LoadDLL(c)
		if err != nil {
			lastErr = err
			continue
		}
		dll = d
		break
	}
	if dll == nil {
		return nil, errors.New("twain: " + dsmMissingMessage(w, ptrSize, cands, lastErr))
	}
	proc, err := dll.FindProc("DSM_Entry")
	if err != nil {
		return nil, fmt.Errorf("twain: DSM_Entry non trovato nel DSM: %w", err)
	}
	t := &realTransport{dll: dll, proc: proc, size: map[uintptr]int{}}
	cls, _ := syscall.UTF16PtrFromString("STATIC")
	name, _ := syscall.UTF16PtrFromString("libretto-twain")
	h, _, cerr := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)),
		wsPopup, 0, 0, 0, 0, 0, 0, 0, 0)
	if h == 0 {
		return nil, fmt.Errorf("twain: impossibile creare la finestra di appoggio: %v", cerr)
	}
	t.hwnd = h
	return t, nil
}

func (t *realTransport) Close() {
	if t.hwnd != 0 {
		pDestroyWindow.Call(t.hwnd)
		t.hwnd = 0
	}
}

func (t *realTransport) PtrSize() int    { return ptrSize }
func (t *realTransport) Parent() uintptr { return t.hwnd }

func bytesPtr(b []byte) uintptr {
	if len(b) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&b[0]))
}

// Entry chiama DSM_Entry. DSM_Entry usa la convenzione stdcall (32 bit) o la convenzione unica (64 bit).
func (t *realTransport) Entry(origin, dest []byte, dg uint32, dat, msg uint16, data []byte) uint16 {
	r, _, _ := t.proc.Call(bytesPtr(origin), bytesPtr(dest), uintptr(dg), uintptr(dat), uintptr(msg), bytesPtr(data))
	runtime.KeepAlive(origin)
	runtime.KeepAlive(dest)
	runtime.KeepAlive(data)
	return uint16(r)
}

func (t *realTransport) Alloc(size int) (uintptr, error) {
	if size < 1 {
		size = 1
	}
	h, _, err := pGlobalAlloc.Call(gmemMoveableZero, uintptr(size))
	if h == 0 {
		return 0, fmt.Errorf("twain: memoria insufficiente (GlobalAlloc %d byte): %v", size, err)
	}
	t.size[h] = size
	return h, nil
}

func (t *realTransport) Lock(h uintptr) []byte {
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return nil
	}
	sz, _, _ := pGlobalSize.Call(h)
	// uintptr -> puntatore di memoria globale (non gestita da Go): conversione indiretta per evitare l'avviso di vet
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	return unsafe.Slice((*byte)(ptr), int(sz))
}

func (t *realTransport) Unlock(h uintptr) { pGlobalUnlock.Call(h) }

func (t *realTransport) Free(h uintptr) {
	pGlobalFree.Call(h)
	delete(t.size, h)
}

// Next attende un messaggio di Windows (o la scadenza) e lo restituisce come byte di una struttura MSG.
func (t *realTransport) Next(deadline time.Time) ([]byte, bool) {
	for {
		var m winMSG
		r, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		if r != 0 {
			b := make([]byte, unsafe.Sizeof(m))
			copy(b, unsafe.Slice((*byte)(unsafe.Pointer(&m)), len(b)))
			return b, true
		}
		left := time.Until(deadline)
		if left <= 0 {
			return nil, false
		}
		ms := int64(left / time.Millisecond)
		if ms > 100 {
			ms = 100
		}
		pMsgWait.Call(0, 0, 0, uintptr(ms), qsAllInput)
	}
}

func (t *realTransport) Dispatch(raw []byte) {
	pTranslateMsg.Call(bytesPtr(raw))
	pDispatchMessage.Call(bytesPtr(raw))
}

// Info riassume come il driver e' stato configurato per l'area (vuoto se non e' stata chiesta un'area).
type Info struct {
	Recipe string // configurazione di ICAP_FRAMES accettata dal driver
}

// ---- API pubblica ----

func newRealSession() (*session, *realTransport, error) {
	t, err := openTransport()
	if err != nil {
		return nil, nil, err
	}
	s := newSession(t)
	w := windowsDir()
	s.noDSHint = noDSMessage(w, ptrSize)
	return s, t, nil
}

// Scan acquisisce UNA pagina (una sola facciata) dal data source TWAIN e la restituisce come immagine.
// L'area (StartMM/HeightMM/WidthMM) si imposta con ICAP_FRAMES e viene riletta: se il driver non la accetta
// la scansione e' annullata prima di avviare lo scanner. Il transfer non viene mai ritentato.
// NON VERIFICATO con carta: che il motore rispetti l'alto del riquadro (vedi docs/agenti/03-wia.md, sezione TWAIN).
func Scan(opts Options) (image.Image, error) {
	img, _, err := ScanInfo(opts)
	return img, err
}

// ScanInfo come Scan ma restituisce anche come e' stata impostata l'area.
func ScanInfo(opts Options) (image.Image, Info, error) {
	o, err := opts.normalized()
	if err != nil {
		return nil, Info{}, err
	}
	var img image.Image
	var info Info
	err = runWithTimeout(o.Timeout+10*time.Second, func() error {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		s, t, err := newRealSession()
		if err != nil {
			return err
		}
		defer t.Close()
		img, err = s.scan(o, time.Now().Add(o.Timeout))
		info = Info{Recipe: s.recipeUsed}
		return err
	})
	return img, info, err
}

// Probe apre DSM e data source, legge le capability e chiude. Non avvia mai la scansione.
// Restituisce le righe da stampare (anche in caso di errore, quelle raccolte fino a quel punto).
func Probe(match string, po ProbeOptions) ([]string, error) {
	if match == "" {
		match = defaultDevice
	}
	var out []string
	err := runWithTimeout(90*time.Second, func() error {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		s, t, err := newRealSession()
		if err != nil {
			return err
		}
		defer t.Close()
		return s.probe(match, po, &out)
	})
	return out, err
}
