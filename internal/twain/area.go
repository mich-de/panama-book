package twain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	mmPerInch = 25.4
	// campo di scansione predefinito dello scanner (8,5x14 pollici) se il driver non lo dichiara
	defaultFieldWmm = 8.5 * mmPerInch
	defaultFieldHmm = 14.0 * mmPerInch
	// tolleranza sul campo: i valori arrotondati (216x356 mm) possono superare di poco quello reale (215,9x355,6)
	fieldToleranceMM = 0.5

	defaultDevice  = "fi-800R"
	defaultDPI     = 300
	defaultTimeout = 3 * time.Minute
)

// Options controlla la scansione TWAIN. Area: origine in alto a sinistra; StartMM e' l'offset dal bordo d'ingresso,
// HeightMM si conta da li (0 = fino al fondo del campo); la larghezza e' centrata sul campo (0 = tutta).
type Options struct {
	DeviceMatch string        // sottostringa del nome del data source, default "fi-800R"
	DPI         int           // default 300
	Gray        bool          // true: scala di grigi, default colore
	StartMM     float64       // offset verticale iniziale in mm (0 = dal bordo d'ingresso)
	HeightMM    float64       // lunghezza in mm da StartMM (0 = fino al fondo)
	WidthMM     float64       // larghezza in mm, centrata (0 = tutto il campo)
	Timeout     time.Duration // tempo massimo dell'intera operazione, default 3 minuti
	// IgnoreFeederLoaded: non rifiutare la scansione se CAP_FEEDERLOADED dice "alimentatore vuoto" (il sensore
	// dell'alimentatore potrebbe non vedere un libretto inserito dal percorso frontale).
	IgnoreFeederLoaded bool
}

// normalized applica i default e valida le opzioni (funzione pura).
func (o Options) normalized() (Options, error) {
	if o.DeviceMatch == "" {
		o.DeviceMatch = defaultDevice
	}
	if o.DPI == 0 {
		o.DPI = defaultDPI
	}
	if o.DPI < 50 || o.DPI > 1200 {
		return o, fmt.Errorf("twain: risoluzione %d dpi non plausibile (50-1200)", o.DPI)
	}
	if o.Timeout < 0 {
		return o, errors.New("twain: Timeout negativo")
	}
	if o.Timeout == 0 {
		o.Timeout = defaultTimeout
	}
	// controllo grossolano sul campo massimo (216x356 mm) prima di toccare lo scanner
	if _, err := frameForArea(o.StartMM, o.WidthMM, o.HeightMM, defaultFieldWmm, defaultFieldHmm); err != nil {
		return o, err
	}
	return o, nil
}

// frameForArea converte l'area in mm nel riquadro TWAIN in pollici (funzione pura). maxWmm e maxHmm sono le
// dimensioni del campo (da ICAP_PHYSICALWIDTH/HEIGHT o predefinite). 0 = area massima sull'asse.
// Valori negativi, NaN, oltre il campo (con tolleranza) o con offset+altezza oltre il campo sono errori.
func frameForArea(startMM, widthMM, heightMM, maxWmm, maxHmm float64) (Frame, error) {
	for _, d := range []struct {
		nome string
		v    float64
		max  float64
	}{{"offset iniziale", startMM, maxHmm}, {"larghezza", widthMM, maxWmm}, {"altezza", heightMM, maxHmm}} {
		if math.IsNaN(d.v) || math.IsInf(d.v, 0) || d.v < 0 {
			return Frame{}, fmt.Errorf("twain: %s dell'area non valida (%v mm): deve essere 0 (massimo) o positiva", d.nome, d.v)
		}
		if d.v > d.max+fieldToleranceMM {
			return Frame{}, fmt.Errorf("twain: %s dell'area %.1f mm oltre il campo dello scanner (massimo %.1f mm)", d.nome, d.v, d.max)
		}
	}
	if startMM+heightMM > maxHmm+fieldToleranceMM {
		return Frame{}, fmt.Errorf("twain: offset %.1f mm + altezza %.1f mm oltre il campo dello scanner (massimo %.1f mm)", startMM, heightMM, maxHmm)
	}
	if maxWmm <= 0 || maxHmm <= 0 {
		return Frame{}, errors.New("twain: dimensioni del campo di scansione non valide")
	}
	width := math.Min(widthMM, maxWmm)
	if widthMM == 0 {
		width = maxWmm
	}
	start := math.Min(startMM, maxHmm)
	bottom := maxHmm
	if heightMM > 0 {
		bottom = math.Min(start+heightMM, maxHmm)
	}
	if bottom-start <= 0 {
		return Frame{}, fmt.Errorf("twain: offset %.1f mm fuori dal campo (%.1f mm)", startMM, maxHmm)
	}
	left := (maxWmm - width) / 2 // centrato: il documento e' centrato sul campo
	return Frame{
		Left:   left / mmPerInch,
		Top:    start / mmPerInch,
		Right:  (left + width) / mmPerInch,
		Bottom: bottom / mmPerInch,
	}, nil
}

// areaRequested dice se Options chiede un'area limitata (altrimenti si lascia il riquadro predefinito).
func (o Options) areaRequested() bool { return o.StartMM > 0 || o.WidthMM > 0 || o.HeightMM > 0 }

// runWithTimeout esegue f in un goroutine; ErrTimeout se non finisce entro d. Un panic in f diventa un errore.
// (Stessa idea di wia.runWithTimeout: il pacchetto e' indipendente da internal/wia.)
func runWithTimeout(d time.Duration, f func() error) error {
	if d <= 0 {
		return f()
	}
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("twain: panic interno: %v", r)
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
