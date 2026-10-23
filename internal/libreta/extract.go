// Package libreta estrae i dati dalla scansione della pagina dati di una Libreta de embarque
// (Seaman's Book) di Panama: legge l'MRZ (affidabile, con check digit) e i campi stampati
// ancorandosi alle loro etichette bilingui ES/EN.
//
// Organizzazione dei file:
//
//	extract.go   orchestrazione (orientamento, OCR pagina intera, confronto MRZ/zona visiva, punti di aggancio)
//	mrzband.go   lettura delle due righe MRZ con il motore OCR di Windows
//	mrzcells.go  lettura dell'MRZ a template, senza OCR (facoltativa: si aggancia con mrzCellsReader)
//	mrzmerge.go  seconda lettura: confronto OCR / template per le parti dell'MRZ senza check digit
//	viz.go       lettura dei campi stampati
package libreta

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"scanlibretto/internal/imgproc"
	"scanlibretto/internal/mrz"
	"scanlibretto/internal/ocr"
)

// Result e' il dato estratto da una scansione.
type Result struct {
	Rotation       int               `json:"rotation_deg"`
	MRZ            *mrz.Data         `json:"mrz,omitempty"`
	MRZCorrections []string          `json:"mrz_corrections,omitempty"`
	Fields         map[string]string `json:"fields"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// Options controlla l'estrazione.
type Options struct {
	Scale    float64 // ingrandimento prima dell'OCR di pagina; 0 = automatico
	DebugDir string  // se valorizzata, salva le immagini preparate e il testo OCR
}

// mrzCellsReader e' il punto di aggancio del lettore MRZ "a template" (internal/libreta/mrzcells.go): legge le celle
// dei caratteri dell'MRZ confrontandole con i glifi, senza il motore OCR di Windows, che su alcune scansioni pulite
// restituisce testo vuoto sui ritagli delle righe. Il lettore si registra da solo, nel suo file, con
//
//	func init() { mrzCellsReader = readMRZCells }
//
// Senza quel file la variabile resta nil e il flusso e' quello di sempre. Riceve l'immagine gia' orientata (testo
// dritto) e una cartella per le immagini di debug (vuota = nessuna); restituisce i dati, le correzioni applicate e un
// punteggio nella scala di scoreMRZ (0 = MRZ non trovato). Contratto in docs/agenti/COORDINAMENTO.md.
var mrzCellsReader func(img image.Image, debugDir string) (*mrz.Data, []string, int)

// mrzBandFunc e' la firma di readMRZBand: serve a provare il flusso senza avviare il motore OCR.
type mrzBandFunc func(ctx context.Context, img image.Image, debugDir string) (*mrz.Data, []string, int, error)

// mrzRead e' una lettura dell'MRZ con le correzioni applicate, il punteggio (scoreMRZ) e gli avvisi da riportare nel
// risultato (testo interno senza valori: cmd/libretto/output.go li traduce in codici stabili).
type mrzRead struct {
	Data     *mrz.Data // nil = MRZ non trovato
	Notes    []string  // correzioni (Result.MRZCorrections)
	Score    int
	Warnings []string // avvisi (Result.Warnings)
}

// readMRZOriented legge l'MRZ da un'immagine gia' orientata. Prima la lettura a banda (OCR); poi, se c'e' un lettore a
// celle registrato in mrzCellsReader (non usa l'OCR e costa circa 0,15 s), anche quello sulla stessa immagine:
//   - se la banda non trova l'MRZ o non tornano tutti i check digit, si tiene la lettura con il punteggio piu' alto (a
//     parita' resta quella della banda);
//   - se la banda ha gia' tutti i check digit, il lettore a celle fa da seconda lettura per le parti che i check digit
//     non coprono (riga 1, nazionalita', sesso): vedi reconcileMRZ. I check digit non peggiorano mai.
func readMRZOriented(ctx context.Context, img image.Image, debugDir string, band mrzBandFunc) (mrzRead, error) {
	d, notes, score, err := band(ctx, img, debugDir)
	if err != nil {
		return mrzRead{}, err
	}
	r := mrzRead{Data: d, Notes: notes, Score: score}
	if mrzCellsReader == nil {
		return r, nil
	}
	if err := ctx.Err(); err != nil {
		return mrzRead{}, err
	}
	cellsDebug := ""
	if debugDir != "" {
		cellsDebug = filepath.Join(debugDir, "celle") // cartella a parte: non sovrascrive i file della lettura a banda
	}
	cd, cnotes, cscore := callCellsReader(img, cellsDebug)
	switch {
	case d != nil && d.Checks.AllOK():
		if cd != nil && cd.Checks.AllOK() {
			r.Data, r.Notes, r.Warnings = reconcileMRZ(d, notes, cd, cnotes)
		}
	case cd != nil && cscore > score:
		r = mrzRead{Data: cd, Notes: cnotes, Score: cscore}
	}
	return r, nil
}

// callCellsReader chiama il lettore a celle. Un suo guasto (panic) non deve fermare l'estrazione: si avvisa su
// stderr e si prosegue con quello che ha letto la banda.
func callCellsReader(img image.Image, debugDir string) (d *mrz.Data, notes []string, score int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "avviso: lettore MRZ a celle in errore (%v): si usa solo la lettura OCR\n", r)
			d, notes, score = nil, nil, 0
		}
	}()
	return mrzCellsReader(img, debugDir)
}

// findOrientation prova le 4 orientazioni (0, 180, 90, 270) e restituisce il risultato con l'MRZ piu' affidabile
// insieme all'immagine ruotata in cui e' stato letto; si ferma alla prima orientazione in cui tutti i check digit
// tornano. Se l'MRZ non si trova in nessuna restituisce (nil, nil, nil).
func findOrientation(ctx context.Context, base image.Image, debugDir string, band mrzBandFunc) (*Result, image.Image, error) {
	var best *Result
	var bestImg image.Image
	bestScore := 0
	for _, rot := range []int{0, 180, 90, 270} {
		img := imgproc.Rotate(base, rot)
		dbg := ""
		if debugDir != "" {
			dbg = filepath.Join(debugDir, fmt.Sprintf("rot%d", rot))
		}
		r, err := readMRZOriented(ctx, img, dbg, band)
		if err != nil {
			return nil, nil, err
		}
		if r.Score > bestScore {
			bestScore, bestImg = r.Score, img
			best = &Result{Rotation: rot, MRZ: r.Data, MRZCorrections: r.Notes, Fields: map[string]string{}, Warnings: r.Warnings}
		}
		if r.Data != nil && r.Data.Checks.AllOK() {
			break
		}
	}
	return best, bestImg, nil
}

// Extract legge la scansione in imgPath. Prova le 4 orientazioni cercando l'MRZ (lettura a banda e, se registrato,
// lettore a celle, che fa anche da seconda lettura per la riga 1) e tiene la prima in cui tutti i check digit tornano
// (o, in mancanza, quella con il punteggio piu' alto); poi legge i campi stampati.
func Extract(ctx context.Context, imgPath string, opt Options) (*Result, error) {
	src, err := imgproc.Load(imgPath)
	if err != nil {
		return nil, fmt.Errorf("lettura immagine: %w", err)
	}
	base := imgproc.CropContent(src, 10)
	base, _ = imgproc.Deskew(base) // una volta sola, prima delle rotazioni: raddrizza i fogli inseriti storti

	best, bestImg, err := findOrientation(ctx, base, opt.DebugDir, readMRZBand)
	if err != nil {
		return nil, err
	}
	if best == nil { // MRZ non trovato: si prova comunque a leggere i campi stampati cosi' com'e' l'immagine
		best = &Result{Fields: map[string]string{}}
		bestImg = base
	}

	// OCR della pagina intera, per i campi stampati
	scaled := prepareVIZImage(bestImg)
	tmp, err := os.CreateTemp("", "libretto-*.png")
	if err != nil {
		return nil, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := imgproc.SavePNG(scaled, tmp.Name()); err != nil {
		return nil, err
	}
	page, err := ocr.Recognize(ctx, tmp.Name())
	if err != nil {
		return nil, err
	}
	if opt.DebugDir != "" {
		_ = os.MkdirAll(opt.DebugDir, 0o755)
		_ = imgproc.SavePNG(scaled, filepath.Join(opt.DebugDir, "page.png"))
		_ = os.WriteFile(filepath.Join(opt.DebugDir, "page.txt"), []byte(dump(page)), 0o644)
	}
	if best.MRZ == nil { // ripiego: righe MRZ dall'OCR della pagina intera
		findMRZ(page, best)
	}
	extractVIZ(page, best)
	crossCheck(best)
	finalizeVIZ(best)
	checkPrintedName(best)
	return best, nil
}

// DataPage ritaglia la sola pagina dati (125x88 mm) dalla scansione, nell'orientamento trovato da Extract
// (rotation = Result.Rotation). Restituisce ok=false se non trova le due righe MRZ.
func DataPage(imgPath string, rotation int) (image.Image, bool, error) {
	src, err := imgproc.Load(imgPath)
	if err != nil {
		return nil, false, fmt.Errorf("lettura immagine: %w", err)
	}
	base, _ := imgproc.Deskew(imgproc.CropContent(src, 10)) // stesso raddrizzamento di Extract
	img := imgproc.Rotate(base, rotation)
	out, ok := imgproc.CropDataPage(img)
	return out, ok, nil
}

func autoScale(width int) float64 {
	switch {
	case width < 900:
		return 3
	case width < 1800:
		return 2
	}
	return 1
}

func dump(r *ocr.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "lingua=%s %dx%d\n", r.Language, r.Width, r.Height)
	for _, l := range r.Lines {
		if len(l.Words) == 0 {
			continue
		}
		fmt.Fprintf(&b, "y=%4.0f x=%4.0f | %s\n", l.Words[0].Y, l.Words[0].X, l.Text)
	}
	return b.String()
}

// crossCheck confronta i campi stampati con l'MRZ e aggiunge i campi derivati dall'MRZ.
func crossCheck(r *Result) {
	if r.MRZ == nil {
		r.Warnings = append(r.Warnings, "MRZ non trovato: i dati stampati non sono verificabili")
		return
	}
	if !r.MRZ.Checks.AllOK() {
		r.Warnings = append(r.Warnings, fmt.Sprintf("check digit MRZ non tutti validi: %+v", r.MRZ.Checks))
	}
	if n := r.Fields["numero_libretto"]; n != "" && n != strings.TrimRight(r.MRZ.DocNumber, "<") {
		r.Warnings = append(r.Warnings, "numero libretto stampato diverso dall'MRZ")
	}
	// campi derivati dall'MRZ, piu' affidabili dell'OCR della zona visiva
	r.Fields["cognome"] = r.MRZ.Surname
	r.Fields["nomi"] = r.MRZ.GivenNames
	r.Fields["nazionalita_iso"] = r.MRZ.Nationality
	r.Fields["sesso"] = r.MRZ.Sex
	r.Fields["data_nascita_mrz"] = r.MRZ.BirthDate
	r.Fields["data_scadenza_mrz"] = r.MRZ.ExpiryDate
	r.Fields["numero_libretto_mrz"] = r.MRZ.DocNumber
	r.Fields["regionale_mrz"] = r.MRZ.Optional
}

func firstMatch(text, pattern string) string {
	return regexpCache(pattern).FindString(text)
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func absf(a float64) float64 {
	if a < 0 {
		return -a
	}
	return a
}
