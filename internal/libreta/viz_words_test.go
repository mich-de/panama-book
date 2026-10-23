package libreta

import (
	"image"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

// vizDrawWordLine disegna in un ritaglio sintetico (grigi, fondo 255) una riga di "parole": blocchi scuri alti h separati da
// vuoti piccoli (lettere) e grandi (spazi). Restituisce i valori e le dimensioni.
func vizDrawWordLine(w, h, y0, lineH int, words []int, letterGap, wordGap int) []uint8 {
	vals := make([]uint8, w*h)
	for i := range vals {
		vals[i] = 255
	}
	x := 20
	for _, n := range words {
		for c := 0; c < n; c++ { // n lettere larghe 14 px
			for yy := y0; yy < y0+lineH; yy++ {
				for xx := x; xx < x+14 && xx < w; xx++ {
					vals[yy*w+xx] = 20
				}
			}
			x += 14 + letterGap
		}
		x += wordGap - letterGap
	}
	return vals
}

func TestVIZSplitWordsAndPickBand(t *testing.T) {
	// "MASTER" (6 lettere) e "II/2" (4 simboli): vuoto tra lettere 3 px, tra parole 30 px, riga alta 30 px
	w, h := 400, 100
	vals := vizDrawWordLine(w, h, 45, 30, []int{6, 4}, 3, 30)
	lines, hs := vizWordRects(vals, w, h)
	if len(lines) != 1 || len(lines[0]) != 2 {
		t.Fatalf("righe %d, parole %v: attese 1 riga di 2 parole", len(lines), lines)
	}
	if hs[0] < 28 || hs[0] > 32 {
		t.Errorf("altezza della riga %.0f, attesa 30", hs[0])
	}
	if lines[0][0].Min.X >= lines[0][1].Min.X || lines[0][0].Dx() < 80 {
		t.Errorf("parole fuori ordine o troppo strette: %v", lines[0])
	}
	// un'etichetta piu' leggera sopra il valore: la banda con piu' inchiostro e' il valore, l'etichetta (>= meta') si legge a parte
	for y := 10; y < 30; y++ {
		for x := 20; x < 135; x++ {
			vals[y*w+x] = 30
		}
	}
	lines, _ = vizWordRects(vals, w, h)
	if len(lines) != 2 {
		t.Fatalf("con l'etichetta ci si aspettano 2 righe, trovate %d", len(lines))
	}
	if lines[1][0].Min.Y < lines[0][0].Min.Y {
		t.Errorf("le righe devono uscire dall'alto in basso: %v", lines)
	}
	// ritaglio vuoto: nessuna riga
	empty := make([]uint8, w*h)
	for i := range empty {
		empty[i] = 255
	}
	if l, _ := vizWordRects(empty, w, h); l != nil {
		t.Error("un ritaglio senza inchiostro non ha righe")
	}
}

func TestVIZReadWordLineAndJobs(t *testing.T) {
	words := map[int][]word{
		8000: {{"MASTER", box{}}},
		8001: {{"11/2", box{}}},
		8002: {{"Cargo/", box{x: 0}}, {"Capacity", box{x: 60}}},
	}
	line := vizWordLine{h: 30, slots: []int{8000, 8001}}
	if got := vizReadWordLine(line, words); got != "MASTER 11/2" {
		t.Errorf("riga = %q", got)
	}
	// dai lavori: la sigla si decodifica e l'etichetta letta si scarta
	cargo := vizBandIndex("cargo")
	jobs := []vizWordJob{{band: cargo, variant: 0, lines: []vizWordLine{{h: 25, slots: []int{8002}}, line}}}
	rd := vizReadings{}
	vizReadWordJobs(rd, vizCropWords{words: words, jobs: jobs})
	if len(rd["cargo"]) != 1 || rd["cargo"][0].f["cargo"] != "MASTER II/2" {
		t.Fatalf("candidati della qualifica: %+v", rd["cargo"])
	}
	if rd["cargo"][0].id < vizWordIDBase {
		t.Errorf("identificativo di variante %d dentro l'intervallo delle altre varianti", rd["cargo"][0].id)
	}
}

func TestVIZWordCandidatesWinTheVote(t *testing.T) {
	// la riga intera esce come "MAS" in 3 varianti e la parola per parola da' la qualifica completa in 4 candidati
	b := vizBands[vizBandIndex("cargo")]
	w := func(int) float64 { return 1 }
	var cs []vizCand
	for i := 0; i < 3; i++ {
		cs = append(cs, cand(i, "cargo", "MAS"))
	}
	for i := 0; i < 4; i++ {
		cs = append(cs, cand(vizWordIDBase+i, "cargo", "MASTER II/2"))
	}
	cs = append(cs, cand(9, "cargo", "UASTZ"), cand(10, "cargo", "MASTER II/2"))
	out := &Result{Fields: map[string]string{}}
	vizVote(b, cs, w, out)
	if out.Fields["cargo"] != "MASTER II/2" {
		t.Errorf("qualifica = %q", out.Fields["cargo"])
	}
}

func TestVIZPrintedName(t *testing.T) {
	// il nome stampato esce dalla fascia "nome" senza entrare in Result.Fields
	var specs []vizWordSpec
	for v := 0; v < 3; v++ {
		specs = append(specs, vizSheetWord("nome", v, "ANGELA LAURA GARCIA"))
	}
	specs = append(specs, vizSheetWord("data_scadenza", 0, "04-03-2031"))
	res := &Result{Fields: map[string]string{}}
	extractVIZ(vizSheetResult(specs), res)
	if _, ok := res.Fields["nome"]; ok {
		t.Error("il nome stampato non deve stare nei campi dell'uscita")
	}
	for k := range res.Fields {
		if strings.HasPrefix(k, "_") {
			t.Errorf("chiave di lavoro %s rimasta nei campi", k)
		}
	}
	name, conf, ok := vizPrintedName(res)
	if !ok || name != "ANGELA LAURA GARCIA" {
		t.Fatalf("nome stampato = %q (ok=%v)", name, ok)
	}
	if conf < 0.9 {
		t.Errorf("tre letture uguali: confidenza attesa alta, trovata %.2f", conf)
	}
	// un altro Result non lo riceve
	if _, _, ok := vizPrintedName(&Result{}); ok {
		t.Error("il nome appartiene a un altro risultato")
	}
	// letture discordanti: confidenza piu' bassa
	specs = nil
	specs = append(specs, vizSheetWord("nome", 0, "ANGELA LAURA GARCIA"), vizSheetWord("nome", 1, "ANGELA LEOM GARCIA"), vizSheetWord("nome", 2, "ANNIXA LAURA GRUNEA"))
	res2 := &Result{Fields: map[string]string{}}
	extractVIZ(vizSheetResult(specs), res2)
	n2, c2, ok2 := vizPrintedName(res2)
	if !ok2 || n2 != "ANGELA LAURA GARCIA" || c2 >= 0.99 {
		t.Errorf("voto per carattere sul nome: %q conf %.2f", n2, c2)
	}
	// il confronto con l'MRZ (riga 1) riconosce il nome letto
	m := &mrz.Data{Surname: "ANGELA LAURA", GivenNames: "GARCIA"}
	if !vizIsMRZName(n2, m) {
		t.Error("il nome stampato letto deve somigliare al nome dell'MRZ")
	}
}

func TestVIZSubGrayPads(t *testing.T) {
	vals := make([]uint8, 10*8)
	for i := range vals {
		vals[i] = 0
	}
	g := vizSubGray(vals, 10, image.Rect(2, 1, 6, 5))
	b := g.Bounds()
	if b.Dx() != 4+2*vizCropMargin || b.Dy() != 4+2*vizCropMargin {
		t.Errorf("dimensioni %v", b)
	}
	if g.GrayAt(0, 0).Y != 255 || g.GrayAt(vizCropMargin, vizCropMargin).Y != 0 {
		t.Error("margine bianco e contenuto")
	}
}
