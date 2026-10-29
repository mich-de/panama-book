package libreta

// Lettura dei testi per PAROLA. Il motore OCR di Windows legge male una riga corta in grassetto con parole di tipo
// diverso (es. "MASTER II/2": restituisce "MAS" e perde il resto, o nulla), ma legge bene la stessa parola isolata in un
// ritaglio proprio ("MASTER") e riconosce a pezzi la sigla di grado ("II/2" esce come "11/2", "1112", "Itn"...). Per i
// campi di testo (qualifica, luogo di nascita, regionale, nazionalita') si individua la riga del valore nel ritaglio
// (la banda con piu' inchiostro scuro: il valore e' in grassetto, piu' grande dell'etichetta), la si divide in parole con
// la proiezione verticale dell'inchiostro e ogni parola va all'OCR come ritaglio a se' (nella stessa chiamata dei
// ritagli degli altri campi: nessun avvio in piu' di PowerShell). Le parole lette si riuniscono in un testo candidato che
// entra nel voto insieme alle letture di riga intera. L'ultima parola della qualifica, se somiglia a una sigla di grado
// (numerale romano / cifra), si decodifica con una piccola grammatica (viz_rank.go).

import (
	"image"
	"sort"
	"strconv"
	"strings"
)

// vizWordBands: campi di testo letti anche per parola.
var vizWordBands = map[string]bool{"cargo": true, "luogo_nascita": true, "regionale": true, "nacionalidad": true, "nome": true}

// vizWordVariants: elaborazioni dei ritagli da cui si estraggono le parole (rampa morbida a due soglie, scala 1,5 volte; finestra
// larga perche' nelle scansioni reali il valore sta qualche pixel piu' in basso del previsto e la finestra stretta
// finirebbe centrata sull'etichetta). Le soglie sono nel dominio normalizzato di vizStretchCrop (inchiostro 0, fondo 255).
var vizWordVariants = []vizVariant{
	{1950, 0.7, 0.9, false, 40},
	{1950, 0.7, 0.9, false, 70},
}

const (
	vizWordBase   = 8000 // primo slot dei ritagli di parola
	vizWordMaxPer = 12   // parole al massimo per riga
	vizWordIDBase = 100  // primo identificativo di variante dei candidati di parola (vedi vizVariantID)
	vizWordDark   = 110  // valore sotto il quale un pixel del ritaglio normalizzato e' inchiostro del testo
)

// vizWordWeight: peso nel voto di una lettura parola per parola rispetto a una lettura di riga intera.
var vizWordWeight = 1.5

// vizWordJob descrive le parole estratte da un ritaglio: fascia, variante e, per ogni riga trovata, uno slot per parola.
type vizWordJob struct {
	band    int // indice in vizBands
	variant int // indice in vizWordVariants
	lines   []vizWordLine
}

// vizWordLine e' una riga del ritaglio divisa in parole: altezza in pixel del ritaglio, uno slot per parola e lo slot della riga intera.
type vizWordLine struct {
	h     float64
	slots []int
	full  int // slot del ritaglio dell'intera riga (stretto sull'inchiostro), 0 se assente
}

// vizLineBand e' una banda di righe del ritaglio con inchiostro scuro.
type vizLineBand struct {
	y0, y1 int // y1 inclusa
	mass   int // pixel scuri nella banda
}

// vizLineBands trova le bande di righe con inchiostro scuro (piu' di un sesto del massimo per riga) e ne da la massa.
func vizLineBands(vals []uint8, w, h int) []vizLineBand {
	rowSum := make([]int, h)
	maxv := 0
	for y := 0; y < h; y++ {
		n := 0
		for x := 0; x < w; x++ {
			if vals[y*w+x] < vizWordDark {
				n++
			}
		}
		rowSum[y] = n
		if n > maxv {
			maxv = n
		}
	}
	if maxv < 4 {
		return nil
	}
	var bands []vizLineBand
	for y := 0; y < h; y++ {
		if rowSum[y] > maxv/6 {
			if k := len(bands); k > 0 && bands[k-1].y1 == y-1 {
				bands[k-1].y1 = y
				bands[k-1].mass += rowSum[y]
			} else {
				bands = append(bands, vizLineBand{y, y, rowSum[y]})
			}
		}
	}
	return bands
}

// vizSplitWords divide una banda in parole: colonne con inchiostro scuro raggruppate finche' il vuoto e' minore di
// 0,45 volte l'altezza della banda (le lettere di un grassetto stanno a meno di 0,2, lo spazio tra parole a piu' di 0,5).
// Restituisce i rettangoli delle parole (coordinate del ritaglio), in ordine da sinistra.
func vizSplitWords(vals []uint8, w, h int, band vizLineBand) []image.Rectangle {
	lineH := band.y1 - band.y0 + 1
	gapMin := max(3, int(0.45*float64(lineH)))
	minW := max(3, lineH/4)
	var out []image.Rectangle
	start, last := -1, -1
	flush := func() {
		if start >= 0 && last-start+1 >= minW {
			out = append(out, image.Rect(max(0, start-3), max(0, band.y0-3), min(w, last+4), min(h, band.y1+4)))
		}
		start = -1
	}
	for x := 0; x < w; x++ {
		dark := false
		for y := band.y0; y <= band.y1; y++ {
			if vals[y*w+x] < vizWordDark {
				dark = true
				break
			}
		}
		if !dark {
			continue
		}
		if start >= 0 && x-last > gapMin {
			flush()
		}
		if start < 0 {
			start = x
		}
		last = x
	}
	flush()
	// una parola corta (1-2 lettere, es. "DR") isolata non si legge: si unisce alla precedente
	var merged []image.Rectangle
	for _, r := range out {
		if k := len(merged); k > 0 && float64(r.Dx()) < vizShortWord*float64(lineH) {
			merged[k-1] = merged[k-1].Union(r)
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

// vizShortWord: una parola piu' stretta di questa frazione dell'altezza della riga (2 lettere) si unisce alla precedente.
const vizShortWord = 1.5

// vizWordRects sceglie le bande da leggere (la piu' pesante e, se pesa almeno la meta', la seconda: il valore e'
// quasi sempre la piu' pesante, ma un'etichetta lunga puo' superarlo; l'etichetta letta si scarta dopo l'OCR) e ne
// restituisce, per ogni banda in ordine dall'alto, i rettangoli delle parole e l'altezza.
func vizWordRects(vals []uint8, w, h int) (lines [][]image.Rectangle, heights []float64) {
	bands := vizLineBands(vals, w, h)
	if len(bands) == 0 {
		return nil, nil
	}
	sort.SliceStable(bands, func(i, j int) bool { return bands[i].mass > bands[j].mass })
	pick := []vizLineBand{bands[0]}
	if len(bands) > 1 && 2*bands[1].mass >= bands[0].mass {
		pick = append(pick, bands[1])
	}
	sort.Slice(pick, func(i, j int) bool { return pick[i].y0 < pick[j].y0 })
	for _, bd := range pick {
		rs := vizSplitWords(vals, w, h, bd)
		if len(rs) == 0 {
			continue
		}
		if len(rs) > vizWordMaxPer {
			rs = rs[:vizWordMaxPer]
		}
		lines = append(lines, rs)
		heights = append(heights, float64(bd.y1-bd.y0+1))
	}
	return lines, heights
}

// vizSubGray ritaglia r dai valori (w x h) e lo mette in un'immagine con margine bianco (vizCropMargin).
func vizSubGray(vals []uint8, w int, r image.Rectangle) *image.Gray {
	sub := make([]uint8, r.Dx()*r.Dy())
	for y := 0; y < r.Dy(); y++ {
		copy(sub[y*r.Dx():(y+1)*r.Dx()], vals[(r.Min.Y+y)*w+r.Min.X:(r.Min.Y+y)*w+r.Max.X])
	}
	return vizPadGray(sub, r.Dx(), r.Dy())
}

// vizReadWordLine riunisce le parole lette di una riga in un testo (parole separate da uno spazio; un ritaglio puo'
// aver dato piu' parole OCR, che si riuniscono con lo spazio). Vuoto se nessuna parola e' stata letta.
func vizReadWordLine(line vizWordLine, words map[int][]word) string {
	var parts []string
	for _, slot := range line.slots {
		ws := append([]word(nil), words[slot]...)
		sort.Slice(ws, func(i, j int) bool { return ws[i].x < ws[j].x })
		var p []string
		for _, w := range ws {
			if t := strings.TrimSpace(w.text); t != "" {
				p = append(p, t)
			}
		}
		if len(p) > 0 {
			parts = append(parts, strings.Join(p, " "))
		}
	}
	return strings.Join(parts, " ")
}

// vizBuildWordJobs prepara i ritagli di parola dei campi di testo e li registra negli slot di crops.
func vizBuildWordJobs(g *image.Gray, geo vizGeom, crops map[int]*image.Gray) []vizWordJob {
	var jobs []vizWordJob
	slot := vizWordBase
	for i, b := range vizBands {
		if !vizWordBands[b.key] {
			continue
		}
		for vi, wv := range vizWordVariants {
			vals, w, h := vizPrepCrop(g, geo, vizBandRect(geo, b, wv, 0, 0), wv, 0, 0)
			if vals == nil {
				continue
			}
			lines, hs := vizWordRects(vals, w, h)
			job := vizWordJob{band: i, variant: vi}
			for li, rects := range lines {
				wl := vizWordLine{h: hs[li]}
				all := rects[0]
				for _, r := range rects {
					crops[slot] = vizSubGray(vals, w, r)
					wl.slots = append(wl.slots, slot)
					slot++
					all = all.Union(r)
				}
				if len(rects) > 1 { // con una sola parola il ritaglio della riga e' quello della parola
					crops[slot] = vizSubGray(vals, w, all)
					wl.full = slot
					slot++
				}
				job.lines = append(job.lines, wl)
			}
			if len(job.lines) > 0 {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs
}

// vizReadWordJobs trasforma le parole lette in letture candidate dei campi di testo (una per riga trovata, piu' una per la riga
// intera) e le aggiunge a rd. Le righe che sono etichette (lette con le parole dell'etichetta) si scartano; la qualifica con
// una sigla di grado in coda la ottiene decodificata. Una lettura che concorda (somiglianza >= vizWordAgree) con una di un'altra
// elaborazione e' sostenuta da due letture indipendenti e conta il doppio (vizWordWeight); una isolata conta uno: dove la parola
// per parola fallisce (ritagli di lettere sfocate) le sue letture sono tutte diverse tra loro.
func vizReadWordJobs(rd vizReadings, crops vizCropWords) {
	type wc struct {
		key     string
		text    string
		variant int
		src     int // 0 parole, 1 riga intera: stessa elaborazione, letture correlate
	}
	var cands []wc
	for _, job := range crops.jobs {
		b := vizBands[job.band]
		for _, line := range job.lines {
			texts := []string{vizReadWordLine(line, crops.words)}
			if line.full > 0 { // la riga intera, ritagliata stretta sull'inchiostro, e' una seconda lettura
				texts = append(texts, vizReadWordLine(vizWordLine{slots: []int{line.full}}, crops.words))
			}
			for k, text := range texts {
				if text == "" || vizIsLabelish(text) || vizLabelFragment(vizCleanText(text)) {
					continue
				}
				t := vizCleanText(text)
				if b.key == "cargo" {
					t, _ = vizDecodeRankTail(t)
				}
				if vizLetters(t) < 2 {
					continue
				}
				cands = append(cands, wc{b.key, t, job.variant, k})
			}
		}
	}
	for i, c := range cands {
		id := vizWordIDBase + c.variant
		for j, d := range cands {
			if i != j && c.key == d.key && c.variant != d.variant && vizSim(norm(c.text), norm(d.text)) >= vizWordAgree {
				id += vizWordIDAgree
				break
			}
		}
		f := map[string]string{c.key: c.text} // senza "_hw": l'altezza dei ritagli di parola non e' confrontabile con quella delle righe OCR
		rd[c.key] = append(rd[c.key], vizCand{id, f})
		if vizTrace != nil {
			vizTrace(c.key, "w"+strconv.Itoa(c.variant)+[]string{"", "L"}[c.src], f)
		}
	}
}

// vizWordAgree: somiglianza minima tra due letture parola per parola di elaborazioni diverse perche' si sostengano.
const vizWordAgree = 0.9

// vizWordIDAgree si somma all'identificativo di una lettura parola per parola sostenuta da un'altra elaborazione.
const vizWordIDAgree = 50
