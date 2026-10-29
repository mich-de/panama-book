package libreta

// Modo pagina (ripiego, quando le righe MRZ non si trovano): una sola lettura OCR della pagina intera e' fragile
// sulle scansioni reali (guilloche, ologrammi, grassetto), mentre nel modo foglio le stesse parole sono lette bene
// perche' il voto tra le varianti corregge gli errori in punti diversi. Qui si prepara lo stesso tipo di insieme
// per la pagina: l'immagine di base (come prima), la stessa con il contrasto stirato e la stessa con la rampa di
// binarizzazione morbida. extractVIZ legge tutte le varianti (un solo avvio di PowerShell), estrae i campi da
// ciascuna con vizExtractPage e vota con lo stesso motore del modo foglio (voto per carattere, avviso di campo
// incerto in finalizeVIZ). Senza MRZ non c'e' la misura dello spessore del tratto: la soglia della rampa e' quella
// misurata sulle scansioni reali (vizPageThr), usata solo qui.

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"
	"time"

	"scanlibretto/internal/imgproc"
)

// vizPageThr: soglia (dopo lo stiramento del contrasto) della variante a rampa del modo pagina. Le soglie adattive
// misurate sulle righe MRZ delle tre scansioni reali (due stampe) sono 45-50.
const vizPageThr = 48

type vizPageSet struct {
	w, h int
	imgs []*image.Gray // varianti oltre all'immagine di base (che e' quella gia' restituita da prepareVIZImage)
}

var (
	vizPageMu    sync.Mutex
	vizPageStash *vizPageSet
)

// vizPreparePage ingrandisce l'immagine come nel ripiego classico e conserva le varianti per extractVIZ.
func vizPreparePage(g *image.Gray, scale float64) image.Image {
	base := imgproc.Scale(g, scale)
	b := base.Bounds()
	bg := vizToGray(base)
	vals := append([]uint8(nil), bg.Pix...)
	if bg.Stride != b.Dx() { // copia compatta
		vals = make([]uint8, b.Dx()*b.Dy())
		for y := 0; y < b.Dy(); y++ {
			copy(vals[y*b.Dx():(y+1)*b.Dx()], bg.Pix[y*bg.Stride:y*bg.Stride+b.Dx()])
		}
	}
	vizStretchCrop(vals)
	stretched := &image.Gray{Pix: vals, Stride: b.Dx(), Rect: image.Rect(0, 0, b.Dx(), b.Dy())}
	soft := image.NewGray(stretched.Rect)
	copy(soft.Pix, vals)
	vizSoftRamp(soft.Pix, vizPageThr)
	vizPageMu.Lock()
	vizPageStash = &vizPageSet{w: b.Dx(), h: b.Dy(), imgs: []*image.Gray{stretched, soft}}
	vizPageMu.Unlock()
	return base
}

func vizTakePage(w, h int) *vizPageSet {
	vizPageMu.Lock()
	defer vizPageMu.Unlock()
	s := vizPageStash
	vizPageStash = nil
	if s == nil || s.w != w || s.h != h {
		return nil
	}
	return s
}

// vizExtractPageSet estrae i campi dalla pagina di base e dalle sue varianti e li vota.
func vizExtractPageSet(words []word, set *vizPageSet, out *Result) {
	read := [][]word{words}
	if set != nil {
		if dir, err := os.MkdirTemp("", "vizpage"); err == nil {
			defer os.RemoveAll(dir)
			var paths []string
			for i, g := range set.imgs {
				p := filepath.Join(dir, fmt.Sprintf("p%d.png", i))
				if imgproc.SavePNG(g, p) == nil {
					paths = append(paths, p)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if rs, err := vizOCRMany(ctx, paths); err == nil {
				for _, r := range rs {
					read = append(read, vizWords(r))
				}
			}
		}
	}
	rd := vizReadings{}
	for id, ws := range read {
		tmp := &Result{Fields: map[string]string{}}
		vizExtractPage(ws, tmp)
		for _, b := range vizBands {
			rd[b.key] = append(rd[b.key], vizCand{id, tmp.Fields})
		}
	}
	vizVoteAll(rd, nil, out)
	vizSetReadings(out, rd)
}
