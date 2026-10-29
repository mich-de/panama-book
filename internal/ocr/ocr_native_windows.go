//go:build windows

package ocr

// Backend OCR nativo: chiama direttamente le API WinRT di Windows (Windows.Media.Ocr) dal processo, senza PowerShell e
// senza cgo. Si parla con gli oggetti WinRT a mano, attraverso le vtable COM:
//
//	combase.dll: RoInitialize, RoGetActivationFactory, WindowsCreateString/DeleteString/GetStringRawBuffer
//	Windows.Media.Ocr.OcrEngine              (factory statica IOcrEngineStatics, oggetto IOcrEngine)
//	Windows.Graphics.Imaging.SoftwareBitmap  (factory statica ISoftwareBitmapStatics)
//	Windows.Storage.Streams.Buffer           (factory IBufferFactory, scrittura dei byte con IBufferByteAccess)
//
// L'immagine non passa da BitmapDecoder: si decodifica con image/png e si consegna al motore un SoftwareBitmap Bgra8
// (alfa Straight) creato dai pixel, cioe' gli stessi byte che BitmapDecoder produce per un PNG (vedi ocr_bitmap.go).
//
// Ogni chiamata gira in una goroutine propria con il thread bloccato (runtime.LockOSThread, mai sbloccato: il thread
// finisce con la goroutine e non lascia stato COM a nessuno). Il thread e' nel MTA. L'attesa di IAsyncOperation e' a
// polling su IAsyncInfo e rispetta il contesto: se scade o viene annullato si chiama IAsyncInfo::Cancel.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	combase                       = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize              = combase.NewProc("RoInitialize")
	procRoUninitialize            = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString       = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
)

// IID delle interfacce usate (specifiche WinRT, Windows SDK).
var (
	iidOcrEngineStatics    = windows.GUID{Data1: 0x5BFFA85A, Data2: 0x3384, Data3: 0x3540, Data4: [8]byte{0x99, 0x40, 0x69, 0x91, 0x20, 0xD4, 0x28, 0xA8}}
	iidSoftwareBitmapStats = windows.GUID{Data1: 0xDF0385DB, Data2: 0x672F, Data3: 0x4A9D, Data4: [8]byte{0x80, 0x6E, 0xC2, 0x44, 0x2F, 0x34, 0x3E, 0x86}}
	iidBufferFactory       = windows.GUID{Data1: 0x71AF914D, Data2: 0xC10F, Data3: 0x484B, Data4: [8]byte{0xBC, 0x50, 0x14, 0xBC, 0x62, 0x3B, 0x3A, 0x27}}
	iidBufferByteAccess    = windows.GUID{Data1: 0x905A0FEF, Data2: 0xBC53, Data3: 0x11DF, Data4: [8]byte{0x8C, 0x49, 0x00, 0x1E, 0x4F, 0xC6, 0x86, 0xDA}}
	iidAsyncInfo           = windows.GUID{Data1: 0x00000036, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

// Indici dei metodi nelle vtable (IInspectable occupa 0..5: QueryInterface, AddRef, Release, GetIids,
// GetRuntimeClassName, GetTrustLevel; i metodi propri cominciano da 6).
const (
	vtQueryInterface = 0
	vtRelease        = 2

	// IOcrEngineStatics
	vtStatMaxImageDimension  = 6
	vtStatTryCreateFromProfL = 10 // TryCreateFromUserProfileLanguages
	// IOcrEngine
	vtEngRecognizeAsync = 6
	vtEngRecognizerLang = 7
	// ILanguage
	vtLangTag = 6
	// IOcrResult
	vtResLines = 6
	// IOcrLine
	vtLineWords = 6
	vtLineText  = 7
	// IOcrWord
	vtWordRect = 6
	vtWordText = 7
	// IVectorView<T>
	vtVecGetAt = 6
	vtVecSize  = 7
	// ISoftwareBitmapStatics
	vtSBCreateCopyWithAlphaFromBuffer = 10
	// IBufferFactory
	vtBufFactoryCreate = 6
	// IBuffer
	vtBufPutLength = 8
	// IBufferByteAccess (derivata da IUnknown)
	vtByteAccessBuffer = 3
	// IAsyncOperation<T>
	vtOpGetResults = 8
	// IAsyncInfo
	vtInfoStatus    = 7
	vtInfoErrorCode = 8
	vtInfoCancel    = 9
	vtInfoClose     = 10
)

const (
	roInitMultithreaded = 1
	rpcEChangedMode     = 0x80010106
	pixelFormatBgra8    = 87 // BitmapPixelFormat.Bgra8
	alphaStraight       = 1  // BitmapAlphaMode.Straight (come BitmapDecoder)

	asyncStarted   = 0
	asyncCompleted = 1
	asyncCanceled  = 2
	asyncError     = 3
)

// com raccoglie il necessario per una serie di chiamate COM su un thread: un'area di lavoro per i parametri di uscita
// (allocata nello heap con dimensione non costante: lo heap di Go non si sposta, quindi l'indirizzo passato a COM
// resta valido durante la chiamata) e gli oggetti da rilasciare alla fine.
type com struct {
	mem   []uintptr
	owned []unsafe.Pointer
}

var scratchWords = 4 // variabile e non costante: cosi' l'array va sempre nello heap

func newCom() *com { return &com{mem: make([]uintptr, scratchWords)} }

func ptrFromUintptr(u uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&u)) }

func hrError(what string, hr uintptr) error {
	return fmt.Errorf("OCR nativo: %s: HRESULT 0x%08X", what, uint32(hr))
}

// call invoca il metodo idx della vtable di obj; restituisce l'HRESULT.
func (c *com) call(obj unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vt := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vt, idx*int(unsafe.Sizeof(uintptr(0)))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// out chiama un metodo con un ultimo parametro di uscita e restituisce i primi 8 byte scritti.
func (c *com) out(what string, obj unsafe.Pointer, idx int, args ...uintptr) (uintptr, error) {
	c.mem[0], c.mem[1] = 0, 0
	args = append(args, uintptr(unsafe.Pointer(&c.mem[0])))
	if hr := c.call(obj, idx, args...); int32(hr) < 0 {
		return 0, hrError(what, hr)
	}
	return c.mem[0], nil
}

func (c *com) outPtr(what string, obj unsafe.Pointer, idx int, args ...uintptr) (unsafe.Pointer, error) {
	v, err := c.out(what, obj, idx, args...)
	if err != nil {
		return nil, err
	}
	p := ptrFromUintptr(v)
	if p != nil {
		c.owned = append(c.owned, p)
	}
	return p, nil
}

func (c *com) outU32(what string, obj unsafe.Pointer, idx int, args ...uintptr) (uint32, error) {
	v, err := c.out(what, obj, idx, args...)
	return uint32(v), err
}

// outString legge una proprieta' HSTRING e la converte in stringa Go.
func (c *com) outString(what string, obj unsafe.Pointer, idx int) (string, error) {
	h, err := c.out(what, obj, idx)
	if err != nil || h == 0 {
		return "", err
	}
	defer procWindowsDeleteString.Call(h)
	var n uint32
	r, _, _ := procWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&n)))
	if r == 0 || n == 0 {
		return "", nil
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(ptrFromUintptr(r)), int(n)))), nil
}

func (c *com) queryInterface(what string, obj unsafe.Pointer, iid *windows.GUID) (unsafe.Pointer, error) {
	return c.outPtr(what, obj, vtQueryInterface, uintptr(unsafe.Pointer(iid)))
}

// factory ottiene la factory statica di una classe runtime.
func (c *com) factory(class string, iid *windows.GUID) (unsafe.Pointer, error) {
	u := utf16.Encode([]rune(class))
	var hs uintptr
	if r, _, _ := procWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&hs))); int32(r) < 0 {
		return nil, hrError("WindowsCreateString", r)
	}
	defer procWindowsDeleteString.Call(hs)
	c.mem[0] = 0
	r, _, _ := procRoGetActivationFactory.Call(hs, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&c.mem[0])))
	if int32(r) < 0 {
		return nil, hrError("RoGetActivationFactory("+class+")", r)
	}
	p := ptrFromUintptr(c.mem[0])
	if p == nil {
		return nil, fmt.Errorf("OCR nativo: factory %s nulla", class)
	}
	c.owned = append(c.owned, p)
	return p, nil
}

// releaseAll rilascia gli oggetti ottenuti, in ordine inverso.
func (c *com) releaseAll() {
	for i := len(c.owned) - 1; i >= 0; i-- {
		c.call(c.owned[i], vtRelease)
	}
	c.owned = nil
}

// release rilascia subito un oggetto (e lo toglie dall'elenco).
func (c *com) release(p unsafe.Pointer) {
	for i := len(c.owned) - 1; i >= 0; i-- {
		if c.owned[i] == p {
			c.owned = append(c.owned[:i], c.owned[i+1:]...)
			break
		}
	}
	c.call(p, vtRelease)
}

// engine e' un OcrEngine creato per le lingue del profilo utente, con le factory che servono.
type engine struct {
	eng     unsafe.Pointer
	maxDim  int
	lang    string
	buffers unsafe.Pointer // IBufferFactory
	bitmaps unsafe.Pointer // ISoftwareBitmapStatics
}

func (c *com) openEngine() (*engine, error) {
	statics, err := c.factory("Windows.Media.Ocr.OcrEngine", &iidOcrEngineStatics)
	if err != nil {
		return nil, err
	}
	e := &engine{}
	md, err := c.outU32("OcrEngine.MaxImageDimension", statics, vtStatMaxImageDimension)
	if err != nil {
		return nil, err
	}
	e.maxDim = int(md)
	if e.eng, err = c.outPtr("OcrEngine.TryCreateFromUserProfileLanguages", statics, vtStatTryCreateFromProfL); err != nil {
		return nil, err
	}
	if e.eng == nil {
		return nil, errors.New("OCR nativo: nessun motore OCR disponibile per le lingue del profilo utente")
	}
	lang, err := c.outPtr("OcrEngine.RecognizerLanguage", e.eng, vtEngRecognizerLang)
	if err != nil {
		return nil, err
	}
	if e.lang, err = c.outString("Language.LanguageTag", lang, vtLangTag); err != nil {
		return nil, err
	}
	if e.buffers, err = c.factory("Windows.Storage.Streams.Buffer", &iidBufferFactory); err != nil {
		return nil, err
	}
	if e.bitmaps, err = c.factory("Windows.Graphics.Imaging.SoftwareBitmap", &iidSoftwareBitmapStats); err != nil {
		return nil, err
	}
	return e, nil
}

// bitmap crea un SoftwareBitmap Bgra8 dai pixel di img.
func (c *com) bitmap(e *engine, img image.Image) (unsafe.Pointer, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	n := w * h * 4
	buf, err := c.outPtr("Buffer.Create", e.buffers, vtBufFactoryCreate, uintptr(n))
	if err != nil {
		return nil, err
	}
	ba, err := c.queryInterface("IBufferByteAccess", buf, &iidBufferByteAccess)
	if err != nil {
		return nil, err
	}
	raw, err := c.out("IBufferByteAccess.Buffer", ba, vtByteAccessBuffer)
	if err != nil {
		return nil, err
	}
	if raw == 0 {
		return nil, errors.New("OCR nativo: buffer nullo")
	}
	fillBGRA(unsafe.Slice((*byte)(ptrFromUintptr(raw)), n), img)
	if hr := c.call(buf, vtBufPutLength, uintptr(n)); int32(hr) < 0 {
		return nil, hrError("IBuffer.Length", hr)
	}
	bmp, err := c.outPtr("SoftwareBitmap.CreateCopyWithAlphaFromBuffer", e.bitmaps, vtSBCreateCopyWithAlphaFromBuffer,
		uintptr(buf), pixelFormatBgra8, uintptr(w), uintptr(h), alphaStraight)
	c.release(ba)
	c.release(buf) // CreateCopy copia i pixel: il buffer (fino a 400 MB) si libera subito
	return bmp, err
}

// await aspetta (polling su IAsyncInfo) un IAsyncOperation<T> e ne restituisce il risultato (puntatore a oggetto).
// Se ctx finisce prima, chiama IAsyncInfo::Cancel.
func (c *com) await(ctx context.Context, op unsafe.Pointer) (unsafe.Pointer, error) {
	info, err := c.queryInterface("IAsyncInfo", op, &iidAsyncInfo)
	if err != nil {
		return nil, err
	}
	defer c.call(info, vtInfoClose)
	delay := 200 * time.Microsecond
	for {
		st, err := c.outU32("IAsyncInfo.Status", info, vtInfoStatus)
		if err != nil {
			return nil, err
		}
		switch st {
		case asyncCompleted:
			return c.outPtr("IAsyncOperation.GetResults", op, vtOpGetResults)
		case asyncError, asyncCanceled:
			ec, _ := c.out("IAsyncInfo.ErrorCode", info, vtInfoErrorCode)
			if st == asyncCanceled {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, errors.New("OCR nativo: operazione annullata")
			}
			return nil, hrError("RecognizeAsync", ec)
		}
		if err := ctx.Err(); err != nil {
			c.call(info, vtInfoCancel)
			// breve attesa che il motore si fermi, per non rilasciare un'operazione ancora in corso
			for i := 0; i < 200; i++ {
				if s, e := c.outU32("IAsyncInfo.Status", info, vtInfoStatus); e != nil || s != asyncStarted {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			return nil, err
		}
		time.Sleep(delay)
		if delay < 4*time.Millisecond {
			delay *= 2
		}
	}
}

// abort annulla un'operazione asincrona ancora in corso e attende (al piu' 1 s) che il motore si fermi.
func (c *com) abort(op unsafe.Pointer) {
	info, err := c.queryInterface("IAsyncInfo", op, &iidAsyncInfo)
	if err != nil {
		return
	}
	c.call(info, vtInfoCancel)
	for i := 0; i < 200; i++ {
		if s, err := c.outU32("IAsyncInfo.Status", info, vtInfoStatus); err != nil || s != asyncStarted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.call(info, vtInfoClose)
}

// readLine legge il testo e le parole (con i riquadri) di una riga.
func (c *com) readLine(line unsafe.Pointer) (Line, error) {
	var l Line
	var err error
	if l.Text, err = c.outString("OcrLine.Text", line, vtLineText); err != nil {
		return l, err
	}
	vec, err := c.outPtr("OcrLine.Words", line, vtLineWords)
	if err != nil {
		return l, err
	}
	n, err := c.outU32("IVectorView.Size", vec, vtVecSize)
	if err != nil {
		return l, err
	}
	l.Words = make([]Word, 0, n)
	for i := uint32(0); i < n; i++ {
		w, err := c.outPtr("IVectorView.GetAt", vec, vtVecGetAt, uintptr(i))
		if err != nil {
			return l, err
		}
		var wd Word
		if wd.Text, err = c.outString("OcrWord.Text", w, vtWordText); err != nil {
			return l, err
		}
		c.mem[0], c.mem[1] = 0, 0
		if hr := c.call(w, vtWordRect, uintptr(unsafe.Pointer(&c.mem[0]))); int32(hr) < 0 {
			return l, hrError("OcrWord.BoundingRect", hr)
		}
		r := (*[4]float32)(unsafe.Pointer(&c.mem[0]))
		wd.X, wd.Y, wd.W, wd.H = floatShortest(r[0]), floatShortest(r[1]), floatShortest(r[2]), floatShortest(r[3])
		l.Words = append(l.Words, wd)
		c.release(w)
	}
	return l, nil
}

// collect aspetta l'operazione avviata e ne legge le righe.
func (c *com) collect(ctx context.Context, e *engine, op unsafe.Pointer, w, h int) (*Result, error) {
	res, err := c.await(ctx, op)
	if err != nil {
		return nil, err
	}
	out := &Result{Language: e.lang, Width: w, Height: h, Lines: []Line{}}
	vec, err := c.outPtr("OcrResult.Lines", res, vtResLines)
	if err != nil {
		return nil, err
	}
	n, err := c.outU32("IVectorView.Size", vec, vtVecSize)
	if err != nil {
		return nil, err
	}
	for i := uint32(0); i < n; i++ {
		ln, err := c.outPtr("IVectorView.GetAt", vec, vtVecGetAt, uintptr(i))
		if err != nil {
			return nil, err
		}
		l, err := c.readLine(ln)
		if err != nil {
			return nil, err
		}
		out.Lines = append(out.Lines, l)
		c.release(ln)
	}
	return out, nil
}

// onThread esegue f in una goroutine con thread bloccato e COM (MTA) inizializzato.
func onThread(f func(c *com) error) (err error) {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread() // mai sbloccato: il thread termina con la goroutine
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("OCR nativo: panic: %v", r)
			}
		}()
		hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
		if int32(hr) < 0 && uint32(hr) != rpcEChangedMode {
			done <- hrError("RoInitialize", hr)
			return
		}
		if int32(hr) >= 0 {
			defer procRoUninitialize.Call()
		}
		c := newCom()
		defer c.releaseAll()
		done <- f(c)
	}()
	return <-done
}

var (
	probeOnce  sync.Once
	probeErr   error
	anchorOnce sync.Once
	anchorErr  error
)

// anchorMTA tiene vivo per tutta la vita del processo il MTA di WinRT: un thread dedicato che fa RoInitialize e non
// esce mai. Senza, quando l'ultimo thread di lavoro fa RoUninitialize l'apartment viene smontato e la chiamata
// successiva fallisce (visto: RoGetActivationFactory 0x80080008, CO_E_SERVER_STOPPING).
func anchorMTA() error {
	anchorOnce.Do(func() {
		ready := make(chan error, 1)
		go func() {
			runtime.LockOSThread()
			hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
			if int32(hr) < 0 {
				ready <- hrError("RoInitialize", hr)
				return
			}
			ready <- nil
			select {} // resta qui: il thread non finisce
		}()
		anchorErr = <-ready
	})
	return anchorErr
}

// nativeProbe verifica una volta sola che il backend nativo parta: combase.dll, le factory, un motore per la lingua.
func nativeProbe() error {
	probeOnce.Do(func() {
		if err := combase.Load(); err != nil {
			probeErr = fmt.Errorf("OCR nativo: %w", err)
			return
		}
		for _, p := range []*windows.LazyProc{procRoInitialize, procRoUninitialize, procRoGetActivationFactory,
			procWindowsCreateString, procWindowsDeleteString, procWindowsGetStringRawBuffer} {
			if err := p.Find(); err != nil {
				probeErr = fmt.Errorf("OCR nativo: %w", err)
				return
			}
		}
		if err := anchorMTA(); err != nil {
			probeErr = err
			return
		}
		probeErr = onThread(func(c *com) error {
			_, err := c.openEngine()
			return err
		})
	})
	return probeErr
}

const (
	// decodeAhead: quante immagini si decodificano in anticipo mentre il motore lavora sulle precedenti (la decodifica
	// PNG di image/png costa circa 80 ms per pagina intera, piu' di un terzo del tempo del motore).
	decodeAhead = 3
	// maxInFlight: quante immagini sono nel motore insieme. Un OcrEngine rifiuta due RecognizeAsync sovrapposte
	// (E_ABORT 0x80004004), quindi ogni operazione in volo ha un motore suo; i risultati sono identici all'esecuzione
	// una per volta (verificato su 72 immagini).
	maxInFlight = 3
)

type loaded struct {
	img  image.Image
	w, h int
	err  error
}

// loadAll avvia in background la lettura delle immagini, al piu' decodeAhead in avanti rispetto a quante ne sono
// state consumate: ogni consumo di un risultato va seguito da una lettura da tokens. Restituisce un canale per
// immagine. Il controllo di MaxImageDimension e' fatto sulle sole dimensioni, prima di decodificare.
func loadAll(ctx context.Context, paths []string, maxDim int) ([]chan loaded, chan struct{}) {
	chs := make([]chan loaded, len(paths))
	for i := range chs {
		chs[i] = make(chan loaded, 1)
	}
	tokens := make(chan struct{}, decodeAhead)
	go func() {
		for i, p := range paths {
			select {
			case tokens <- struct{}{}:
			case <-ctx.Done():
				return
			}
			go func(i int, p string) {
				var l loaded
				if l.w, l.h, l.err = bitmapSize(p); l.err == nil {
					if l.w > maxDim || l.h > maxDim {
						l.err = errTooLarge{l.w, l.h}
					} else {
						l.img, l.err = bitmapLoad(p)
					}
				}
				chs[i] <- l
			}(i, p)
		}
	}()
	return chs, tokens
}

// pending e' un'immagine consegnata al motore e non ancora letta.
type pending struct {
	op, bmp unsafe.Pointer
	w, h    int
}

// recognizeNative e' il backend nativo di RecognizeMany.
func recognizeNative(ctx context.Context, paths []string) ([]*Result, error) {
	if err := nativeProbe(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	results := make([]*Result, 0, len(paths))
	err := onThread(func(c *com) error {
		var engs []*engine
		for len(engs) < maxInFlight && len(engs) < len(paths) {
			e, err := c.openEngine()
			if err != nil {
				return err
			}
			engs = append(engs, e)
		}
		lctx, lcancel := context.WithCancel(ctx)
		defer lcancel()
		chs, tokens := loadAll(lctx, paths, engs[0].maxDim)

		var queue []pending
		defer func() { // uscita con errore: le operazioni ancora in volo si annullano
			for _, f := range queue {
				c.abort(f.op)
			}
		}()
		finish := func() error {
			f := queue[0]
			r, err := c.collect(ctx, engs[0], f.op, f.w, f.h)
			if err != nil {
				return err
			}
			queue = queue[1:]
			c.release(f.bmp)
			results = append(results, r)
			return nil
		}
		for i := range paths {
			var l loaded
			select {
			case l = <-chs[i]:
			case <-ctx.Done():
				return ctx.Err()
			}
			<-tokens
			if l.err != nil {
				return l.err
			}
			bmp, err := c.bitmap(engs[0], l.img)
			if err != nil {
				return err
			}
			op, err := c.outPtr("OcrEngine.RecognizeAsync", engs[i%len(engs)].eng, vtEngRecognizeAsync, uintptr(bmp))
			if err != nil {
				return err
			}
			queue = append(queue, pending{op, bmp, l.w, l.h})
			for len(queue) >= len(engs) { // il motore i%len(engs) si riusa solo dopo che la sua operazione e' finita
				if err := finish(); err != nil {
					return err
				}
			}
		}
		for len(queue) > 0 {
			if err := finish(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
