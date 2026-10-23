package wia

import (
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"time"
)

// Valori predefiniti.
const (
	defaultDevice  = "fi-800R"
	defaultDPI     = 300
	defaultTimeout = 2 * time.Minute
	// limiti di sicurezza generici; il vero intervallo e' quello dichiarato dal driver (75-600 per l'fi-800R)
	minSaneDPI = 50
	maxSaneDPI = 1200
)

// ErrTimeout: l'operazione COM non e' terminata nel tempo concesso.
// Una chiamata WIA bloccata non si puo' annullare: dopo il timeout conviene terminare il processo.
var ErrTimeout = errors.New("wia: tempo scaduto")

// Options controlla la scansione.
type Options struct {
	DeviceMatch string        // sottostringa del nome del dispositivo, default "fi-800R"
	DPI         int           // default 300; deve rientrare nell'intervallo del driver (75..600)
	Gray        bool          // true: scala di grigi, default colore
	OutPath     string        // file PNG di destinazione
	Timeout     time.Duration // tempo massimo dell'intera operazione, default 2 minuti
	Force       bool          // true: puo' sostituire un OutPath esistente; false (default): lo rifiuta prima di toccare lo scanner
	// Area di scansione limitata (0 = area massima). Origine in alto: StartMM e' l'offset dal bordo
	// d'ingresso, HeightMM si conta a partire da li; la larghezza e' centrata rispetto al campo
	// (documento centrato).
	StartMM  float64 // offset iniziale in mm dal bordo d'ingresso (0 = dal bordo)
	WidthMM  float64 // larghezza in mm (max 216)
	HeightMM float64 // lunghezza in mm da StartMM (StartMM+HeightMM max 356); 0 = fino al fondo del campo
}

// Campo di scansione massimo del driver (8,5x14 pollici), in mm, per validare le opzioni prima di
// toccare lo scanner. Il limite effettivo in pixel resta quello letto dal driver.
const (
	maxFieldWidthMM  = 216.0
	maxFieldHeightMM = 356.0
	mmPerInch        = 25.4
)

// checkAreaMM valida le dimensioni dell'area in mm (funzione pura): 0 = area massima / dal bordo.
func checkAreaMM(startMM, widthMM, heightMM float64) error {
	for _, d := range []struct {
		nome string
		v    float64
		max  float64
	}{{"offset iniziale", startMM, maxFieldHeightMM}, {"larghezza", widthMM, maxFieldWidthMM}, {"altezza", heightMM, maxFieldHeightMM}} {
		if math.IsNaN(d.v) || math.IsInf(d.v, 0) || d.v < 0 {
			return fmt.Errorf("wia: %s dell'area non valida (%v mm): deve essere 0 (area massima) o positiva", d.nome, d.v)
		}
		if d.v > d.max {
			return fmt.Errorf("wia: %s dell'area %.1f mm oltre il campo dello scanner (massimo %.0f mm)", d.nome, d.v, d.max)
		}
	}
	if startMM+heightMM > maxFieldHeightMM {
		return fmt.Errorf("wia: offset %.1f mm + altezza %.1f mm oltre il campo dello scanner (massimo %.0f mm)", startMM, heightMM, maxFieldHeightMM)
	}
	return nil
}

// areaPixels converte l'area in mm in pixel (funzione pura). maxW e maxH sono i massimi letti dal driver
// per Horizontal/Vertical Extent al dpi corrente. Una misura a 0 vuol dire "area massima" su quell'asse
// (extX=maxW, startX=0, extY=maxH). startMM>0 e' l'offset verticale dal bordo d'ingresso: startY in
// pixel, e l'altezza (o il massimo, se HeightMM=0) si conta da li, quindi startY+extY <= maxH. Misure oltre il massimo del driver (arrotondamenti al bordo del
// campo) sono limitate al massimo. La larghezza e' centrata: startX = (maxW-extX)/2.
// Con una misura richiesta e il relativo massimo sconosciuto (<=0) restituisce errore.
func areaPixels(startMM, widthMM, heightMM float64, dpi, maxW, maxH int) (startX, startY, extX, extY int, err error) {
	if err := checkAreaMM(startMM, widthMM, heightMM); err != nil {
		return 0, 0, 0, 0, err
	}
	if dpi <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("wia: risoluzione %d dpi non valida per calcolare l'area", dpi)
	}
	if startMM > 0 {
		if maxH <= 0 {
			return 0, 0, 0, 0, errors.New("wia: limite altezza del driver non leggibile: offset non applicabile")
		}
		startY = int(math.Round(startMM / mmPerInch * float64(dpi)))
		if startY >= maxH {
			return 0, 0, 0, 0, fmt.Errorf("wia: offset %.1f mm (%d px a %d dpi) fuori dal campo (%d px)", startMM, startY, dpi, maxH)
		}
		maxH -= startY // l'altezza disponibile parte dall'offset
	}
	conv := func(mm float64, max int, nome string) (int, error) {
		if mm == 0 {
			return max, nil
		}
		if max <= 0 {
			return 0, fmt.Errorf("wia: limite %s del driver non leggibile: area limitata non applicabile", nome)
		}
		px := int(math.Round(mm / mmPerInch * float64(dpi)))
		if px < 1 {
			return 0, fmt.Errorf("wia: %s dell'area %.2f mm troppo piccola a %d dpi (meno di 1 pixel)", nome, mm, dpi)
		}
		if px > max {
			px = max
		}
		return px, nil
	}
	if extX, err = conv(widthMM, maxW, "larghezza"); err != nil {
		return 0, 0, 0, 0, err
	}
	if extY, err = conv(heightMM, maxH, "altezza"); err != nil {
		return 0, 0, 0, 0, err
	}
	if widthMM > 0 {
		startX = (maxW - extX) / 2
	}
	return startX, startY, extX, extY, nil
}

// normalized applica i default e valida le opzioni (funzione pura).
func (o Options) normalized() (Options, error) {
	if o.OutPath == "" {
		return o, errors.New("wia: OutPath obbligatorio")
	}
	if o.DeviceMatch == "" {
		o.DeviceMatch = defaultDevice
	}
	if o.DPI == 0 {
		o.DPI = defaultDPI
	}
	if o.DPI < minSaneDPI || o.DPI > maxSaneDPI {
		return o, fmt.Errorf("wia: risoluzione %d dpi non plausibile (%d-%d)", o.DPI, minSaneDPI, maxSaneDPI)
	}
	if err := checkAreaMM(o.StartMM, o.WidthMM, o.HeightMM); err != nil {
		return o, err
	}
	if o.Timeout < 0 {
		return o, errors.New("wia: Timeout negativo")
	}
	if o.Timeout == 0 {
		o.Timeout = defaultTimeout
	}
	return o, nil
}

// checkDPI verifica la risoluzione contro l'intervallo del driver; max<=0 vuol dire intervallo sconosciuto.
func checkDPI(dpi, lo, hi int) error {
	if hi > 0 && (dpi < lo || dpi > hi) {
		return fmt.Errorf("risoluzione %d dpi fuori dall'intervallo del driver (%d-%d)", dpi, lo, hi)
	}
	return nil
}

// checkSimplex verifica che la selezione del gestore documenti sia solo-alimentatore senza duplex.
func checkSimplex(sel int, readable bool) error {
	if !readable {
		return errors.New("Document Handling Select non leggibile: non posso garantire una sola facciata, scansione annullata")
	}
	if sel&dpsDocumentHandlingDuplex != 0 {
		return fmt.Errorf("il duplex risulta attivo (Document Handling Select=%d): scansione annullata", sel)
	}
	if sel&dpsDocumentHandlingFeeder == 0 {
		return fmt.Errorf("l'alimentatore non risulta selezionato (Document Handling Select=%d): scansione annullata", sel)
	}
	return nil
}

const (
	dpsDocumentHandlingFeeder = 1 // WIA_DPS_DOCUMENT_HANDLING_SELECT: FEEDER
	dpsDocumentHandlingDuplex = 4 // DUPLEX: non deve mai essere attivo
	statusFeedReady           = 1 // WIA_DPS_DOCUMENT_HANDLING_STATUS: FEED_READY (NON vuol dire "carta presente")

	dataTypeColor = 3
	dataTypeGray  = 2
)

// runWithTimeout esegue f in un goroutine e restituisce ErrTimeout se non finisce entro d.
// Se d<=0 esegue f direttamente. Un panic in f diventa un errore.
func runWithTimeout(d time.Duration, f func() error) error {
	if d <= 0 {
		return f()
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("wia: panic interno: %v\n%s", r, debug.Stack())
			}
		}()
		done <- f()
	}()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		return err
	case <-t.C:
		return fmt.Errorf("%w dopo %v (la chiamata al driver potrebbe essere ancora in corso)", ErrTimeout, d)
	}
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// num converte i tipi numerici restituiti dai VARIANT in int; 0 se il tipo non e' numerico.
func num(v interface{}) int {
	switch n := v.(type) {
	case int8:
		return int(n)
	case uint8:
		return int(n)
	case int16:
		return int(n)
	case uint16:
		return int(n)
	case int32:
		return int(n)
	case uint32:
		return int(n)
	case int64:
		return int(n)
	case uint64:
		return int(n)
	case int:
		return n
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// isNum dice se v e' un valore numerico riconosciuto da num.
func isNum(v interface{}) bool {
	switch v.(type) {
	case int8, uint8, int16, uint16, int32, uint32, int64, uint64, int, float32, float64:
		return true
	}
	return false
}
