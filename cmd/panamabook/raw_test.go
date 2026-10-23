package main

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func imgSize(t *testing.T, p string) (int, int) {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, _, err := image.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	return c.Width, c.Height
}

func hasHint(hs []any, sub string) bool {
	for _, h := range hs {
		if strings.Contains(h.(string), sub) {
			return true
		}
	}
	return false
}

// ---- parte pura della diagnosi ------------------------------------------------------------------------------------

func TestAnalyzeBoxCasi(t *testing.T) {
	const dpi = 300
	ppm := 300 / 25.4
	W, H := int(216*ppm), int(190*ppm)
	mm := func(v float64) int { return int(v * ppm) }
	box := func(x0, x1, y0, y1 float64) image.Rectangle { return image.Rect(mm(x0), mm(y0), mm(x1), mm(y1)) }
	// centrato, alto 176: nessun suggerimento
	d := analyzeBox(W, H, box(45.5, 170.5, 7, 183), 0, false, dpi, 190)
	if !d.OK || len(d.Hints) != 0 {
		t.Errorf("centrato: %+v", d)
	}
	// centro spostato di 30 mm a destra della grezza = SINISTRA nell'anteprima (la grezza e' capovolta)
	d = analyzeBox(W, H, box(75.5, 200.5, 7, 183), 0, false, dpi, 190)
	if d.OK || !strings.Contains(d.Hints[0], "decentrato verso SINISTRA") || !strings.Contains(d.Hints[0], "3.0 cm verso DESTRA") || d.OffsetMM > -29 || d.OffsetMM < -31 {
		t.Errorf("spostato: %+v", d)
	}
	d = analyzeBox(W, H, box(15.5, 140.5, 7, 183), 0, false, dpi, 190)
	if !strings.Contains(d.Hints[0], "decentrato verso DESTRA") {
		t.Errorf("spostato sull'altro lato: %+v", d.Hints)
	}
	// tocca il bordo della grezza (tagliato): si stima quanto manca. Bordo destro della grezza = sinistro nell'anteprima
	d = analyzeBox(W, H, box(150, 216, 7, 183), 0, false, dpi, 190)
	if len(d.Hints) == 0 || !strings.Contains(d.Hints[0], "esce dal campo di lettura") || !strings.Contains(d.Hints[0], "bordo sinistro") || !strings.Contains(d.Hints[0], "decentrato verso SINISTRA") {
		t.Errorf("tagliato (bordo destro della grezza): %+v", d.Hints)
	}
	d = analyzeBox(W, H, box(0, 70, 7, 183), 0, false, dpi, 190)
	if len(d.Hints) == 0 || !strings.Contains(d.Hints[0], "bordo destro") || !strings.Contains(d.Hints[0], "decentrato verso DESTRA") {
		t.Errorf("tagliato (bordo sinistro della grezza): %+v", d.Hints)
	}
	// entrato troppo poco (50 mm con 190 mm letti) e con 97 mm (soglia diversa)
	d = analyzeBox(W, H, box(45.5, 170.5, 4, 54), 0, false, dpi, 190)
	if len(d.Hints) != 1 || !strings.Contains(d.Hints[0], "troppo poco") || !strings.Contains(d.Hints[0], "alto solo 50 mm invece di circa 176") {
		t.Errorf("troppo poco: %+v", d.Hints)
	}
	if d = analyzeBox(W, int(97*ppm), box(45.5, 170.5, 4, 94), 0, false, dpi, 97); !d.OK {
		t.Errorf("97 mm letti e 90 mm di documento vanno bene: %+v", d.Hints)
	}
	if d = analyzeBox(W, int(97*ppm), box(45.5, 170.5, 4, 54), 0, false, dpi, 97); len(d.Hints) != 1 {
		t.Errorf("50 mm su 97: %+v", d.Hints)
	}
	// inclinato
	d = analyzeBox(W, H, box(45.5, 170.5, 7, 183), 3.4, false, dpi, 190)
	if len(d.Hints) != 1 || !strings.Contains(d.Hints[0], "inclinato di circa 3.4 gradi") {
		t.Errorf("inclinato: %+v", d.Hints)
	}
	if d = analyzeBox(W, H, box(45.5, 170.5, 7, 183), -0.8, false, dpi, 190); !d.OK {
		t.Errorf("0,8 gradi sono tollerati: %+v", d.Hints)
	}
	// nessun documento
	if d = analyzeBox(W, H, image.Rectangle{}, 0, true, dpi, 190); !d.Blank || len(d.Hints) != 1 || !strings.Contains(d.Hints[0], "non si vede nessun documento") {
		t.Errorf("vuoto: %+v", d)
	}
	// somma di problemi
	d = analyzeBox(W, H, box(75.5, 200.5, 4, 54), 2.5, false, dpi, 190)
	if len(d.Hints) != 3 {
		t.Errorf("tre problemi: %+v", d.Hints)
	}
}

// ---- con immagini sintetiche (AutoCropInfo vero) --------------------------------------------------------------------

func TestDiagnosiSuGrezzeSintetiche(t *testing.T) {
	cases := []struct {
		mode string
		want string // sottostringa attesa nei suggerimenti; "" = nessun suggerimento
	}{
		{"centered", ""},
		{"right", "decentrato verso SINISTRA"}, // documento verso il bordo destro della grezza = sinistra nell'anteprima
		{"left", "decentrato verso DESTRA"},
		{"short", "troppo poco"},
		{"tilt", "inclinato"},
		{"blank", "non si vede nessun documento"},
	}
	for _, c := range cases {
		e := newEnv(t, nil, "FAKE_RAW="+c.mode)
		pv := e.previewDone()["preview"].(map[string]any)
		dg, _ := pv["diag"].(map[string]any)
		if dg == nil || dg["done"] != true {
			t.Fatalf("%s: nessuna diagnosi: %v", c.mode, pv)
		}
		hints, _ := dg["hints"].([]any)
		if c.want == "" {
			if dg["ok"] != true || len(hints) != 0 {
				t.Errorf("%s: attesa posizione ok, trovato %v (%v)", c.mode, hints, dg["summary"])
			}
		} else if !hasHint(hints, c.want) {
			t.Errorf("%s: manca %q nei suggerimenti %v (%v)", c.mode, c.want, hints, dg["summary"])
		}
		if dg["summary"] == "" {
			t.Errorf("%s: riepilogo mancante", c.mode)
		}
	}
}

// ---- grezza accanto all'anteprima -------------------------------------------------------------------------------------

func TestGrezzaSalvataConLAnteprima(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	e := newEnv(t, nil, "FAKE_ARGS="+args)
	pv := e.previewDone()["preview"].(map[string]any)
	if pv["has_raw"] != true || !strings.Contains(pv["raw_url"].(string), "anteprima-001-grezza.png") {
		t.Fatalf("anteprima: %v", pv)
	}
	b, _ := os.ReadFile(args)
	if !strings.Contains(string(b), "-raw "+filepath.Join("dati", "anteprima", "anteprima-001-grezza.png")) {
		t.Errorf("manca -raw: %s", b)
	}
	if !e.exists("dati", "anteprima", "anteprima-001-grezza.png") || !e.exists("dati", "anteprima", "anteprima-001-diagnosi.json") || e.exists("scans") {
		t.Error("grezza e diagnosi in dati\\anteprima, niente in scans")
	}
	if w, h := imgSize(t, filepath.Join(e.base, "dati", "anteprima", "anteprima-001-grezza.png")); w != 2551 || h != 1145 {
		t.Errorf("grezza sintetica: %dx%d", w, h)
	}
	// la grezza si vede (dir=preview), ma non come immagine di scans, e non si legge come anteprima
	if c, _ := e.do("GET", "/api/image?dir=preview&file=anteprima-001-grezza.png", nil); c != 200 {
		t.Errorf("grezza: %d", c)
	}
	if c, _ := e.do("GET", "/api/image?file=anteprima-001-grezza.png", nil); c != 404 {
		t.Errorf("grezza come file di scans: %d", c)
	}
	if c, _ := e.do("POST", "/api/open", map[string]string{"file": "anteprima-001-grezza.png", "dir": "preview"}); c != 404 {
		t.Errorf("lettura della grezza: %d", c)
	}
	// la striscia riporta grezza e diagnosi (dopo un riavvio la diagnosi si rilegge dal file)
	_, l := e.do("GET", "/api/previews", nil)
	it := l["items"].([]any)[0].(map[string]any)
	if it["has_raw"] != true || it["diag"] == nil {
		t.Errorf("striscia: %v", it)
	}
	// la grezza non e' una voce della striscia
	if len(l["items"].([]any)) != 1 {
		t.Errorf("voci: %d", len(l["items"].([]any)))
	}
}

func TestScartoEPuliziaTolgonoGrezzaPaginaEDiagnosi(t *testing.T) {
	e := newEnv(t, nil)
	e.previewDone()
	e.readPreview("anteprima-001.png")
	aux := []string{"anteprima-001.png", "anteprima-001-grezza.png", "anteprima-001-pagina.png", "anteprima-001-diagnosi.json"}
	for _, f := range aux {
		if !e.exists("dati", "anteprima", f) {
			t.Fatalf("manca %s", f)
		}
	}
	if c, _ := e.do("POST", "/api/preview/discard", map[string]string{"file": "anteprima-001.png"}); c != 200 {
		t.Fatal(c)
	}
	for _, f := range aux {
		if e.exists("dati", "anteprima", f) {
			t.Errorf("%s doveva essere scartato", f)
		}
	}
	// pulizia per eta': tutti i file di un'anteprima vecchia
	e.previewDone()
	old := time.Now().Add(-8 * 24 * time.Hour)
	for _, f := range []string{"anteprima-001.png", "anteprima-001-grezza.png", "anteprima-001-diagnosi.json"} {
		p := filepath.Join(e.base, "dati", "anteprima", f)
		_ = os.Chtimes(p, old, old)
	}
	if n := e.app.cleanOldPreviews(previewMaxAge); n != 3 {
		t.Errorf("eliminati %d, attesi 3", n)
	}
}

// ---- pagina dati ritagliata: e' l'immagine che si salva -----------------------------------------------------------------

func TestLetturaAutomaticaPaginaDatiESalvataggio(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	e := newEnv(t, nil, "FAKE_ARGS="+args)
	pv := e.previewDone()["preview"].(map[string]any)
	if pv["has_page"] != true || !strings.Contains(pv["page_url"].(string), "anteprima-001-pagina.png") {
		t.Fatalf("pagina dati attesa subito dopo la scansione: %v", pv)
	}
	// "Leggi i dati": istantaneo dalla cache (nessuna seconda estrazione), il margine richiesto non rilancia nulla
	doc := func() map[string]any {
		_, r := e.do("POST", "/api/open", map[string]any{"file": "anteprima-001.png", "dir": "preview", "top_margin_mm": 3.5})
		return e.waitJob(r["job"].(string))["doc"].(map[string]any)
	}()
	b, _ := os.ReadFile(args)
	if strings.Count(string(b), "extract -i") != 1 {
		t.Errorf("una sola estrazione attesa:\n%s", b)
	}
	if !strings.Contains(doc["page_url"].(string), "anteprima-001-pagina.png") || len(doc["fields"].([]any)) != len(fieldDefs) {
		t.Errorf("documento dalla cache: %v", doc["page_url"])
	}
	// PDF dalla pagina dati
	if c, _ := e.do("POST", "/api/pdf", map[string]any{"file": "anteprima-001.png", "dir": "preview"}); c != 200 {
		t.Fatal(c)
	}
	b, _ = os.ReadFile(args)
	if !strings.Contains(string(b), "pdf -i "+filepath.Join("dati", "anteprima", "anteprima-001-pagina.png")+" -o "+filepath.Join("dati", "anteprima", "anteprima-001.pdf")) {
		t.Errorf("il PDF deve partire dalla pagina dati: %s", b)
	}
	_, l := e.do("GET", "/api/previews", nil)
	if it := l["items"].([]any)[0].(map[string]any); it["has_page"] != true {
		t.Errorf("striscia: %v", it)
	}
	// conferma: in scans va la SOLA PAGINA (200x140 del finto); intera e grezza non conservate; la cache e' sparita
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, nil)
	if code != 200 || r["used_page"] != true {
		t.Fatalf("conferma: %d %v", code, r)
	}
	const base = "MARIO - ROSSI - P1234567"
	if w, h := imgSize(t, filepath.Join(e.base, "scans", base+".png")); w != 200 || h != 140 {
		t.Errorf("l'immagine salvata deve essere la pagina dati: %dx%d", w, h)
	}
	if !e.exists("scans", base+".pdf") || e.exists("scans", base+" (intera).png") || e.exists("scans", base+" (grezza).png") {
		t.Errorf("file in scans: %v", e.scansEntries())
	}
	if ents, _ := os.ReadDir(filepath.Join(e.base, "dati", "anteprima")); len(ents) != 0 {
		t.Errorf("file di lavoro rimasti: %d", len(ents))
	}
}

func TestPaginaNonTrovataSiMostraLIntera(t *testing.T) {
	e := newEnv(t, nil, "FAKE_PAGE=none")
	j := e.previewDone()
	pv := j["preview"].(map[string]any)
	if pv["has_page"] != false || !strings.Contains(j["message"].(string), "La pagina dati non è stata trovata") {
		t.Errorf("senza pagina: %v %v", pv["has_page"], j["message"])
	}
	// la lettura in cache c'e' comunque (MRZ dubbia): "Leggi i dati" funziona
	if !e.exists("dati", "anteprima", "anteprima-001-lettura.json") {
		t.Error("lettura in cache attesa")
	}
}

func TestScansioneWIAnonRuota(t *testing.T) {
	args := filepath.Join(t.TempDir(), "args.txt")
	e := newEnv(t, nil, "FAKE_ARGS="+args)
	code, r := e.do("POST", "/api/preview", map[string]any{"driver": "wia", "dpi": 300, "len_mm": 190, "top_margin_mm": 2})
	if code != 202 {
		t.Fatal(code)
	}
	e.waitJob(r["job"].(string))
	b, _ := os.ReadFile(args)
	line := strings.SplitN(string(b), "\n", 2)[0]
	want := "scan -yes -o " + filepath.Join("dati", "anteprima", "anteprima-001.png") + " -driver wia -dpi 300 -len 190 -top-margin 2 -raw "
	if !strings.HasPrefix(line, want) || strings.Contains(line, "-rotate") || strings.Contains(line, "-from") {
		t.Errorf("WIA senza -rotate e -from (ricetta di scan-wia.cmd):\n%s", line)
	}
}

func TestConservaScansioneInteraEGrezzaSoloSeRichiesto(t *testing.T) {
	e := newEnv(t, nil)
	e.previewDone()
	doc := e.readPreview("anteprima-001.png")
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"keep_full": true, "keep_raw": true})
	if code != 200 {
		t.Fatalf("%d %v", code, r)
	}
	const base = "MARIO - ROSSI - P1234567"
	if w, h := imgSize(t, filepath.Join(e.base, "scans", base+".png")); w != 200 || h != 140 {
		t.Errorf("principale: %dx%d", w, h)
	}
	if w, _ := imgSize(t, filepath.Join(e.base, "scans", base+" (intera).png")); w != 120 {
		t.Errorf("intera: %d", w)
	}
	if w, _ := imgSize(t, filepath.Join(e.base, "scans", base+" (grezza).png")); w != 2551 {
		t.Errorf("grezza: %d", w)
	}
	// un secondo libretto uguale: "(2)" e le copie "(intera)/(grezza)" non si sovrappongono
	e.previewDone()
	doc2 := e.readPreview("anteprima-001.png")
	code, r = e.save(doc2["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"keep_full": true, "keep_raw": true})
	if code != 200 || r["doc"].(map[string]any)["file"] != base+" (2).png" || !e.exists("scans", base+" (2) (intera).png") || !e.exists("scans", base+" (2) (grezza).png") {
		t.Errorf("seconda copia: %d %v %v", code, r["doc"], e.scansEntries())
	}
}

func TestSenzaPaginaDatiSiSalvaLAnteprimaECosiELoDice(t *testing.T) {
	e := newEnv(t, nil, "FAKE_PAGE=none")
	e.previewDone()
	doc := e.readPreview("anteprima-001.png")
	if doc["page_url"] != nil {
		t.Errorf("senza pagina: %v", doc["page_url"])
	}
	code, r := e.save(doc["doc_id"].(string), "confirm", nil, []string{"capacity"}, map[string]any{"keep_full": true})
	if code != 200 || r["used_page"] != false {
		t.Fatalf("%d %v", code, r)
	}
	const base = "MARIO - ROSSI - P1234567"
	if w, h := imgSize(t, filepath.Join(e.base, "scans", base+".png")); w != 120 || h != 80 {
		t.Errorf("deve restare l'anteprima intera: %dx%d", w, h)
	}
	if e.exists("scans", base+" (intera).png") {
		t.Error("senza pagina non c'e' una copia 'intera' separata")
	}
	said := false
	for _, m := range r["messages"].([]any) {
		if strings.Contains(m.(string), "La pagina dati non è stata trovata") {
			said = true
		}
	}
	if !said {
		t.Errorf("va detto: %v", r["messages"])
	}
}

func TestImpostazioniPredefiniteLunghezza190(t *testing.T) {
	e := newEnv(t, nil)
	_, st := e.do("GET", "/api/state", nil)
	d := st["defaults"].(map[string]any)
	if d["driver"] != "wia" || d["dpi"].(float64) != 300 || d["len_mm"].(float64) != 190 || d["top_margin_mm"].(float64) != 2 {
		t.Errorf("predefiniti: %v", d)
	}
}
