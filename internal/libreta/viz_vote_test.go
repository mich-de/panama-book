package libreta

// Test unitari del voto tra le varianti (viz_vote.go): letture sintetiche, nessun OCR. I valori usati sono
// inventati (non sono dati di persone reali).

import (
	"image/color"
	"strings"
	"testing"

	"scanlibretto/internal/mrz"
)

func ones(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 1
	}
	return w
}

func TestVIZPosVoteCorrectsIndependentErrors(t *testing.T) {
	// tre letture con un errore ciascuna in un punto diverso: il voto sulla stringa intera non ha vincitori,
	// quello per posizione ricostruisce il valore
	got, contested := vizPosVote([]string{"123456789", "123956789", "123456780", "523456789"}, ones(4))
	if got != "123456789" {
		t.Errorf("voto per posizione = %q", got)
	}
	if contested {
		t.Error("un solo voto contrario su quattro non e' un contrasto")
	}
}

func TestVIZPosVoteTieAndContested(t *testing.T) {
	// parita' in una posizione: vince il carattere della lettura con priorita' (la prima); il contrasto e' segnalato
	got, contested := vizPosVote([]string{"1234", "1294", "1294", "1234"}, ones(4))
	if got != "1234" || !contested {
		t.Errorf("parita': %q contested=%v", got, contested)
	}
	// il peso rompe la parita'
	got, _ = vizPosVote([]string{"1234", "1294"}, []float64{1, 1.5})
	if got != "1294" {
		t.Errorf("peso: %q", got)
	}
}

func TestVIZPosVoteLengthGroups(t *testing.T) {
	// letture di lunghezza diversa non si mescolano: vince il gruppo piu' pesato
	got, _ := vizPosVote([]string{"12345", "1234", "12345", "9999"}, ones(4))
	if got != "12345" {
		t.Errorf("gruppo di lunghezza: %q", got)
	}
	if got, _ := vizPosVote(nil, nil); got != "" {
		t.Errorf("senza letture: %q", got)
	}
}

func TestVIZTextVotePerCharAndWords(t *testing.T) {
	// "VERAGUAS PEDRO": molte letture hanno solo la prima parola, altre hanno una lettera sbagliata in punti diversi
	vals := []string{"VERAGUAS", "VERAGUAS", "VERAGUAS", "VERAGUAS PEDRO", "VERAGUAS PEDRD", "VERACUAS PEDRO", "VERAGUAS OEDRO"}
	got, _, ok := vizTextVote(vals, ones(len(vals)))
	if !ok || got != "VERAGUAS PEDRO" {
		t.Errorf("voto del testo = %q (ok=%v)", got, ok)
	}
	// una sola lettura a due parole (rumore) non fa salire N
	vals = []string{"PANAMA", "PANAMA", "PANAMA", "PANAMA", "PANAMA", "ROBERT ANTONIO"}
	got, _, ok = vizTextVote(vals, ones(len(vals)))
	if !ok || got != "PANAMA" {
		t.Errorf("rumore a piu' parole: %q", got)
	}
	// nessun sostegno (una sola lettura): ripiego sul voto intero
	if _, _, ok := vizTextVote([]string{"ORDINARY SEAMAN"}, []float64{1}); ok {
		t.Error("una sola lettura non ha sostegno sufficiente")
	}
}

func TestVIZTextVoteMissingFirstWord(t *testing.T) {
	// l'OCR perde spesso la prima parola: "SEAMAN" molto votato non deve battere "ORDINARY SEAMAN"
	vals := []string{"SEAMAN", "SEAMAN", "SEAMAN", "SEAMAN", "ORDINARY SEAMAN", "ORDINARY SEAMAN", "ORDINARY SEAMAN", "OROINARY SEAMAN"}
	got, _, ok := vizTextVote(vals, ones(len(vals)))
	if !ok || got != "ORDINARY SEAMAN" {
		t.Errorf("prima parola persa: %q", got)
	}
}

func TestVIZPickLongestSuffix(t *testing.T) {
	counts := map[string]float64{"SEAMAN": 5, "ORDINARY SEAMAN": 3}
	if got := vizPickLongest([]string{"SEAMAN", "ORDINARY SEAMAN"}, counts); got != "ORDINARY SEAMAN" {
		t.Errorf("la fine della frase deve sostenere la frase intera: %q", got)
	}
}

func TestVIZSnapRegional(t *testing.T) {
	for in, want := range map[string]string{
		"PANAMA OR": "PANAMA DR", "PANAMA DR": "PANAMA DR", "PANAMA 0R": "PANAMA DR",
		"PANAMA": "PANAMA", "OR": "OR", "COLON": "COLON", "BOCAS DEL TORO": "BOCAS DEL TORO",
	} {
		if got := vizSnapRegional(in); got != want {
			t.Errorf("vizSnapRegional(%q) = %q, atteso %q", in, got, want)
		}
	}
}

func TestVIZCanonIdent(t *testing.T) {
	for s, want := range map[string]bool{
		"8-123-45": true, "4-123-4567": true, "PE0123456": true, "812345": false, "8-12345": false, "012345": false,
		"-123-45": false, "8-123-": false, "12-1-1": true,
		"C1ABC2DEF": true, "N123456": true, "2051234": true,
	} {
		if got := vizCanonIdent(s); got != want {
			t.Errorf("vizCanonIdent(%q) = %v", s, got)
		}
	}
}

func cand(id int, kv ...string) vizCand {
	f := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		f[kv[i]] = kv[i+1]
	}
	return vizCand{id, f}
}

func TestVIZVoteAllNumericAndUncertain(t *testing.T) {
	rd := vizReadings{
		// l'ultima cifra: 6 contro 8 con pesi quasi uguali (campo incerto); le altre posizioni concordano
		"id_marino": {cand(0, "id_marino", "510123456"), cand(1, "id_marino", "510133458"), cand(2, "id_marino", "610123458"), cand(3, "id_marino", "510123456")},
		// date: errori in punti diversi, nessuna in contrasto
		"data_scadenza": {
			cand(0, "data_scadenza", "04-03-2031", "data_scadenza_iso", "2031-03-04"),
			cand(1, "data_scadenza", "04-09-2031", "data_scadenza_iso", "2031-09-04"),
			cand(2, "data_scadenza", "04-03-2037", "data_scadenza_iso", "2037-03-04"),
			cand(3, "data_scadenza", "04-03-2031", "data_scadenza_iso", "2031-03-04"),
		},
	}
	out := &Result{Fields: map[string]string{}}
	unc := vizVoteAll(rd, nil, out)
	if out.Fields["id_marino"] != "510123456" && out.Fields["id_marino"] != "510123458" {
		t.Errorf("id_marino = %q", out.Fields["id_marino"])
	}
	if out.Fields["data_scadenza"] != "04-03-2031" || out.Fields["data_scadenza_iso"] != "2031-03-04" {
		t.Errorf("data_scadenza = %q / %q", out.Fields["data_scadenza"], out.Fields["data_scadenza_iso"])
	}
	found := map[string]bool{}
	for _, k := range unc {
		found[k] = true
	}
	if !found["id_marino"] {
		t.Errorf("id_marino con ultima cifra in parita' (2 contro 2) deve essere incerto: %v", unc)
	}
	if found["data_scadenza"] {
		t.Errorf("data_scadenza con un solo voto contrario per posizione non e' incerta: %v", unc)
	}
}

func TestVIZMRZWeightsRewardAccurateVariants(t *testing.T) {
	m := &mrz.Data{DocNumber: "PA0123401", BirthDate: "2000-01-01", ExpiryDate: "2031-03-04",
		Checks: mrz.Checks{DocNumber: true, Birth: true, Expiry: true}}
	rd := vizReadings{
		"data_nascita": {
			cand(0, "data_nascita_iso", "2000-01-01"), cand(1, "data_nascita_iso", "2009-09-01"),
		},
		"data_scadenza": {
			cand(0, "data_scadenza_iso", "2031-03-04"), cand(1, "data_scadenza_iso", "2031-09-01"),
		},
		"numero_libretto": {cand(0, "numero_libretto", "PA0123401"), cand(1, "numero_libretto", "PA0823407")},
		"id_marino":       {cand(0, "id_marino", "510123456"), cand(1, "id_marino", "510133458"), cand(2, "id_marino", "510123456")},
	}
	w := vizMRZWeights(rd, m)
	if w == nil || w[0] <= w[1] {
		t.Fatalf("la variante che legge giusto i campi noti deve pesare di piu': %v", w)
	}
	// media 1 sulle varianti con letture di campi noti; senza letture (id 2) il peso e' neutro: assente dalla mappa
	if mean := (w[0] + w[1]) / 2; mean < 0.99 || mean > 1.01 {
		t.Errorf("pesi non normalizzati: media %.3f", mean)
	}
	if _, ok := w[2]; ok {
		t.Errorf("una variante senza letture di campi noti non ha peso calibrato: %v", w)
	}
	// senza MRZ o con check digit non validi nessuna calibrazione utile (pesi uguali)
	if vizMRZWeights(rd, nil) != nil {
		t.Error("senza MRZ i pesi devono essere uguali (nil)")
	}
	w2 := vizMRZWeights(rd, &mrz.Data{})
	if w2[0] != w2[1] {
		t.Errorf("check digit non validi: pesi uguali attesi, trovati %v", w2)
	}
}

func TestVIZWeightsDecideNumericVote(t *testing.T) {
	// due varianti affidabili contro tre inaffidabili: la calibrazione sull'MRZ rovescia la maggioranza semplice
	m := &mrz.Data{DocNumber: "PA0123401", BirthDate: "2000-01-01", ExpiryDate: "2031-03-04",
		Checks: mrz.Checks{DocNumber: true, Birth: true, Expiry: true}}
	good := func(id int) vizCand {
		return cand(id, "data_nascita", "01-01-2000", "data_nascita_iso", "2000-01-01")
	}
	bad := func(id int) vizCand {
		return cand(id, "data_nascita", "01-09-2009", "data_nascita_iso", "2009-09-01")
	}
	rd := vizReadings{
		"data_nascita": {good(0), good(1), bad(2), bad(3), bad(4)},
		"id_marino": {
			cand(0, "id_marino", "510123456"), cand(1, "id_marino", "510123456"),
			cand(2, "id_marino", "510133458"), cand(3, "id_marino", "510133458"), cand(4, "id_marino", "510133458"),
		},
	}
	plain := &Result{Fields: map[string]string{}}
	vizVoteAll(rd, nil, plain)
	if plain.Fields["id_marino"] != "510133458" {
		t.Fatalf("senza pesi vince la maggioranza semplice: %q", plain.Fields["id_marino"])
	}
	weighted := &Result{Fields: map[string]string{}}
	vizVoteAll(rd, vizMRZWeights(rd, m), weighted)
	if weighted.Fields["id_marino"] != "510123456" {
		t.Errorf("con i pesi calibrati sull'MRZ devono vincere le varianti affidabili: %q", weighted.Fields["id_marino"])
	}
}

func vizAllValid() mrz.Checks {
	return mrz.Checks{DocNumber: true, Birth: true, Expiry: true, Optional: true, Composite: true}
}

func TestVIZReconcileMRZOverridesAndFills(t *testing.T) {
	m := &mrz.Data{DocCode: "IS", Issuer: "PAN", DocNumber: "PA0123401", Nationality: "ITA", BirthDate: "2000-01-01",
		ExpiryDate: "2031-03-04", Optional: "NAPOLES", Checks: vizAllValid()}
	// stampato letto male: l'MRZ prevale e la differenza e' segnalata (senza valori)
	r := &Result{MRZ: m, Fields: map[string]string{
		"data_nascita": "01-09-2000", "data_nascita_iso": "2000-09-01",
		"data_scadenza": "04-03-2031", "data_scadenza_iso": "2031-03-04",
		"numero_libretto": "PA0123407", "tipo": "5I", "paese": "PAM", "regionale": "NAPOLFS", "nacionalidad": "ITALIAN",
	}}
	vizFinalize(r)
	want := map[string]string{"data_nascita": "01-01-2000", "data_nascita_iso": "2000-01-01", "numero_libretto": "PA0123401",
		"tipo": "SI", "paese": "PAN", "regionale": "NAPOLES", "nacionalidad": "ITALIAN"}
	for k, v := range want {
		if r.Fields[k] != v {
			t.Errorf("campo %s = %q, atteso %q", k, r.Fields[k], v)
		}
	}
	joined := strings.Join(r.Warnings, " | ")
	for _, w := range []string{"data di nascita stampata diversa dall'MRZ", "numero libretto stampato diverso dall'MRZ"} {
		if !strings.Contains(joined, w) {
			t.Errorf("avviso mancante %q in %q", w, joined)
		}
	}
	if strings.Contains(joined, "2000") || strings.Contains(joined, "PA012") {
		t.Errorf("l'avviso riporta valori personali: %q", joined)
	}
	// stampato non letto: con l'MRZ valido si riempiono i sei campi che l'MRZ ha (e il regionale), senza avvisi di incertezza
	r2 := &Result{MRZ: m, Fields: map[string]string{}}
	vizFinalize(r2)
	want2 := map[string]string{"data_nascita": "01-01-2000", "data_scadenza": "04-03-2031", "data_scadenza_iso": "2031-03-04",
		"numero_libretto": "PA0123401", "tipo": "SI", "paese": "PAN", "nacionalidad": "ITALIAN", "regionale": "NAPOLES"}
	for k, v := range want2 {
		if r2.Fields[k] != v {
			t.Errorf("campo vuoto %s = %q, atteso %q", k, r2.Fields[k], v)
		}
	}
	for _, k := range []string{"tipo", "paese", "nacionalidad", "numero_libretto", "data_nascita", "data_scadenza"} {
		if strings.Contains(strings.Join(r2.Warnings, "|"), "campo incerto: "+k) {
			t.Errorf("campo %s ricavato da un MRZ valido non deve risultare incerto: %v", k, r2.Warnings)
		}
	}
}

func TestVIZReconcileMRZPartialChecks(t *testing.T) {
	// solo il check del numero valido: si fida solo di quello; tipo e paese ricavati restano segnalati come incerti
	m := &mrz.Data{DocCode: "IS", Issuer: "PAN", DocNumber: "PA0123401", BirthDate: "2000-01-01", Nationality: "ITA",
		Checks: mrz.Checks{DocNumber: true}}
	r := &Result{MRZ: m, Fields: map[string]string{"data_nascita": "01-09-2000", "data_nascita_iso": "2000-09-01", "numero_libretto": "PA0123407"}}
	vizFinalize(r)
	if r.Fields["numero_libretto"] != "PA0123401" {
		t.Errorf("numero non allineato: %q", r.Fields["numero_libretto"])
	}
	if r.Fields["data_nascita"] != "01-09-2000" {
		t.Errorf("senza il check della nascita la data stampata non si tocca: %q", r.Fields["data_nascita"])
	}
	joined := strings.Join(r.Warnings, "|")
	for _, k := range []string{"tipo", "paese", "nacionalidad"} {
		if !strings.Contains(joined, "campo incerto: "+k) {
			t.Errorf("campo %s ricavato da un MRZ non verificato deve essere incerto: %v", k, r.Warnings)
		}
	}
	// senza MRZ non si inventa nulla
	r3 := &Result{Fields: map[string]string{}}
	vizFinalize(r3)
	for k := range r3.Fields {
		t.Errorf("campo inventato senza MRZ: %s", k)
	}
	// stessa data con secolo stampato diverso da quello dell'MRZ: si tiene la stampata
	r4 := &Result{MRZ: &mrz.Data{BirthDate: "2080-05-01", Checks: mrz.Checks{Birth: true}},
		Fields: map[string]string{"data_nascita": "01-05-1980", "data_nascita_iso": "1980-05-01"}}
	vizFinalize(r4)
	if r4.Fields["data_nascita"] != "01-05-1980" || strings.Contains(strings.Join(r4.Warnings, "|"), "diversa dall'MRZ") {
		t.Errorf("stessa data (anno a due cifre) non va sostituita: %v %v", r4.Fields["data_nascita"], r4.Warnings)
	}
}

func TestVIZReconcileRegional(t *testing.T) {
	mk := func(opt, printed string) *Result {
		m := &mrz.Data{Issuer: "PAN", DocCode: "IS", Nationality: "ITA", Optional: opt, Checks: vizAllValid(), DocNumber: "PA0123401"}
		f := map[string]string{}
		if printed != "" {
			f["regionale"] = printed
		}
		r := &Result{MRZ: m, Fields: f}
		vizFinalize(r)
		return r
	}
	// il nome stampato e' piu' completo del campo opzionale (facsimile): resta
	if r := mk("PANAMA", "PANAMA DR"); r.Fields["regionale"] != "PANAMA DR" {
		t.Errorf("regionale piu' completo sostituito: %q", r.Fields["regionale"])
	}
	// una lettera storpiata: prevale l'MRZ
	if r := mk("NAPOLES", "NAPOLE5"); r.Fields["regionale"] != "NAPOLES" {
		t.Errorf("regionale storpiato non corretto: %q", r.Fields["regionale"])
	}
	// non letto: si riempie dal campo opzionale ma e' incerto (potrebbe essere solo il prefisso)
	r := mk("NAPOLES", "")
	if r.Fields["regionale"] != "NAPOLES" || !strings.Contains(strings.Join(r.Warnings, "|"), "campo incerto: regionale") {
		t.Errorf("regionale non letto: %q %v", r.Fields["regionale"], r.Warnings)
	}
}

func TestVIZWarnUncertain(t *testing.T) {
	r := &Result{Fields: map[string]string{"id_marino": "510123456", "identificazione": "8-123-45", "data_nascita": "01-01-2000"},
		MRZ: &mrz.Data{Checks: mrz.Checks{Birth: true}}}
	r.Fields["luogo_nascita"] = "VERAGUAS PEDRO"
	vizWarnUncertain(r, []string{"id_marino", "identificazione", "data_nascita", "luogo_nascita", "livello"}, nil)
	joined := strings.Join(r.Warnings, " | ")
	for _, want := range []string{"campo incerto: id_marino", "campo incerto: identificazione", "campo incerto: luogo_nascita"} {
		if !strings.Contains(joined, want) {
			t.Errorf("avviso mancante %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "data_nascita") {
		t.Errorf("la data verificata dall'MRZ non e' incerta: %q", joined)
	}
	if strings.Contains(joined, "livello") {
		t.Errorf("campi fuori elenco non vanno segnalati: %q", joined)
	}
	for _, secret := range []string{"510123456", "8-123-45", "01-01-2000", "VERAGUAS"} {
		if strings.Contains(joined, secret) {
			t.Errorf("l'avviso riporta un valore: %q", joined)
		}
	}
}

func TestVIZFinalizeRevotesWithMRZ(t *testing.T) {
	m := &mrz.Data{DocNumber: "PA0123401", BirthDate: "2000-01-01", ExpiryDate: "2031-03-04", Sex: "M", Nationality: "PAN",
		Optional: "PANAMA", Checks: mrz.Checks{DocNumber: true, Birth: true, Expiry: true, Optional: true, Composite: true}}
	good := func(id int) vizCand {
		return cand(id, "data_nascita", "01-01-2000", "data_nascita_iso", "2000-01-01", "data_scadenza", "04-03-2031", "data_scadenza_iso", "2031-03-04",
			"id_marino", "510123456")
	}
	bad := func(id int) vizCand {
		return cand(id, "data_nascita", "01-09-2009", "data_nascita_iso", "2009-09-01", "data_scadenza", "04-09-2037", "data_scadenza_iso", "2037-09-04",
			"id_marino", "510133458")
	}
	cs := []vizCand{good(0), good(1), bad(2), bad(3), bad(4)}
	rd := vizReadings{"data_nascita": cs, "data_scadenza": cs, "id_marino": cs}
	r := &Result{MRZ: m, Fields: map[string]string{"id_marino": "510133458", "data_nascita": "01-09-2009"}}
	vizSetReadings(r, rd)
	finalizeVIZ(r)
	if r.Fields["id_marino"] != "510123456" {
		t.Errorf("il nuovo voto pesato sull'MRZ non ha corretto id_marino: %q", r.Fields["id_marino"])
	}
	if r.Fields["data_nascita"] != "01-01-2000" || r.Fields["data_nascita_iso"] != "2000-01-01" {
		t.Errorf("data di nascita non coerente con l'MRZ: %q", r.Fields["data_nascita"])
	}
	// le letture sono consumate: una seconda chiamata non rivota
	if vizTakeReadings(r) != nil {
		t.Error("le letture devono essere consumate da finalizeVIZ")
	}
	// letture di un altro Result non si applicano
	vizSetReadings(&Result{}, rd)
	r2 := &Result{Fields: map[string]string{"id_marino": "111111111"}}
	finalizeVIZ(r2)
	if r2.Fields["id_marino"] != "111111111" {
		t.Errorf("letture di un altro risultato applicate: %q", r2.Fields["id_marino"])
	}
	_ = vizTakeReadings(nil)
}

// vizDrawEdges aggiunge due linee verticali scure lunghe (bordi della tessera / plastificazione) ai lati delle righe MRZ.
func vizDrawEdges(g interface {
	SetGray(x, y int, c color.Gray)
}, xs []int, y0, y1 int) {
	for _, x := range xs {
		for y := y0; y < y1; y++ {
			for dx := 0; dx < 3; dx++ {
				g.SetGray(x+dx, y, color.Gray{60})
			}
		}
	}
}

func TestVIZLocateMRZIgnoresCardEdges(t *testing.T) {
	// i bordi lunghi della tessera passano accanto alle righe MRZ: non devono allargarne la geometria
	g := vizDrawMRZ(1000, 1400, 120, 1250, 16, 0)
	vizDrawEdges(g, []int{40, 940}, 100, 1390)
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatal("MRZ non trovata")
	}
	if absf(geo.x0-120) > 10 || absf(geo.w-(43*16+8)) > 25 {
		t.Errorf("geometria sporcata dai bordi: x0=%.0f w=%.0f (attesi 120, %d)", geo.x0, geo.w, 43*16+8)
	}
}

func TestVIZSnapRank(t *testing.T) {
	for in, want := range map[string]string{
		"OROINARY SEAMAN": "ORDINARY SEAMAN", "ORDINARY SEAMAN": "ORDINARY SEAMAN", "ORDINARY SEAHAN": "ORDINARY SEAMAN",
		"CHIEF ENGINEER": "CHIEF ENGINEER", "SEAMAN": "SEAMAN", "HEAD CHEF": "HEAD CHEF", "ABLE SEAMAN": "ABLE SEAMAN",
		"THIRD ENGINEER": "THIRD ENGINEER", "SECOND OFFICER": "SECOND OFFICER",
	} {
		if got := vizSnapRank(in); got != want {
			t.Errorf("vizSnapRank(%q) = %q, atteso %q", in, got, want)
		}
	}
}

func TestVIZVoteThinSupportIsUncertain(t *testing.T) {
	b := vizBands[vizBandIndex("cargo")]
	w := func(int) float64 { return 1 }
	var cs []vizCand
	for i := 0; i < 12; i++ {
		cs = append(cs, cand(i, "cargo", ""))
	}
	cs[3] = cand(3, "cargo", "MASTER")
	out := &Result{Fields: map[string]string{}}
	if !vizVote(b, cs, w, out) {
		t.Error("un campo letto da 1 variante su 12 deve risultare incerto")
	}
	for i := 0; i < 6; i++ {
		cs[i] = cand(i, "cargo", "MASTER")
	}
	if vizVote(b, cs, w, out) {
		t.Error("un campo letto da 6 varianti su 12, concordi, non e' incerto")
	}
	if out.Fields["cargo"] != "MASTER" {
		t.Errorf("cargo = %q", out.Fields["cargo"])
	}
}

func TestVIZPreparePageStash(t *testing.T) {
	g := vizDrawMRZ(400, 300, 20, 100, 8, 0)
	out := vizPreparePage(g, 2)
	b := out.Bounds()
	if b.Dx() != 800 || b.Dy() != 600 {
		t.Fatalf("pagina ingrandita %dx%d, attesa 800x600", b.Dx(), b.Dy())
	}
	if vizTakePage(10, 10) != nil {
		t.Error("le varianti non devono essere restituite per dimensioni diverse")
	}
	out = vizPreparePage(g, 2)
	set := vizTakePage(b.Dx(), b.Dy())
	if set == nil || len(set.imgs) != 2 {
		t.Fatalf("varianti della pagina assenti: %+v", set)
	}
	if vizTakePage(b.Dx(), b.Dy()) != nil {
		t.Error("le varianti si consumano una volta")
	}
	// la variante a rampa e' in grigi sfumati (non binaria): contiene toni intermedi
	mid := 0
	for _, v := range set.imgs[1].Pix {
		if v > 20 && v < 235 {
			mid++
		}
	}
	if mid == 0 {
		t.Error("la rampa morbida non ha toni intermedi")
	}
	_ = out
}

func TestVIZSoftRamp(t *testing.T) {
	// rampa e non taglio netto: scura sotto thr, bianca oltre thr+w/2, toni intermedi in mezzo, monotona (con thr
	// bassa il nero resta un grigio scuro: e' il contrasto che il motore OCR legge meglio)
	v := []uint8{0, 20, 45, 90, 135, 200, 255}
	vizSoftRamp(v, 45)
	if v[0] > 70 || v[len(v)-1] != 255 {
		t.Errorf("estremi: %v", v)
	}
	if v[2] < 100 || v[2] > 150 {
		t.Errorf("alla soglia deve esserci un grigio medio: %d", v[2])
	}
	for i := 1; i < len(v); i++ {
		if v[i] < v[i-1] {
			t.Errorf("non monotona: %v", v)
		}
	}
	if v[1] == 0 || v[1] == 255 {
		t.Errorf("il bordo deve restare sfumato: %v", v)
	}
}

func TestVIZLabelFragment(t *testing.T) {
	for s, want := range map[string]bool{
		"DE": true, "LUGAR DE": true, "PLACE OF": true, "Lugar de nacimiento": true, "CASAL": false, "DE LA CRUZ": false,
		"HOTEL STAFF": false, "": false, "SEAMAN S BOOK": true,
	} {
		if got := vizLabelFragment(s); got != want {
			t.Errorf("vizLabelFragment(%q) = %v", s, got)
		}
	}
}

func TestVIZTextClusterDropsNoise(t *testing.T) {
	// cinque letture buone del valore (una con un carattere storpiato) e quattro resti di etichetta tutti diversi
	vals := []string{"CASAL", "CASAL", "GASAL", "CASAL", "RECE OF", "OF H", "LVGAR DE", "PECE", "CASAQ"}
	got, _, ok := vizTextVote(vals, ones(len(vals)))
	if !ok || got != "CASAL" {
		t.Errorf("voto del testo con rumore = %q (ok=%v)", got, ok)
	}
}

func TestVIZSnapRankKeepsUnknownRanks(t *testing.T) {
	// l'elenco delle qualifiche NON e' chiuso: reparti hotel, ristorazione, intrattenimento restano come letti
	for _, s := range []string{"HOTEL STAFF", "ENTERTAINER", "RESTAURANT STAFF", "MASTER II/2", "CASINO DEALER"} {
		if got := vizSnapRank(s); got != s {
			t.Errorf("vizSnapRank(%q) = %q: una qualifica non riconosciuta deve restare com'e'", s, got)
		}
	}
}

func TestVIZTipoVocabularyDropsNoise(t *testing.T) {
	b := vizBands[vizBandIndex("tipo")]
	w := func(int) float64 { return 1 }
	cs := []vizCand{cand(0, "tipo", "C6O"), cand(1, "tipo", "C6O"), cand(2, "tipo", "S1"), cand(3, "tipo", "")}
	out := &Result{Fields: map[string]string{}}
	vizVote(b, cs, w, out)
	if out.Fields["tipo"] != "SI" {
		t.Errorf("tipo = %q: il rumore fuori vocabolario non deve vincere", out.Fields["tipo"])
	}
	out = &Result{Fields: map[string]string{}}
	vizVote(b, []vizCand{cand(0, "tipo", "C6O"), cand(1, "tipo", "DEL")}, w, out)
	if out.Fields["tipo"] != "" {
		t.Errorf("solo rumore: il tipo deve restare vuoto, trovato %q", out.Fields["tipo"])
	}
}

func TestVIZFinalizeTipoFromMRZ(t *testing.T) {
	r := &Result{MRZ: &mrz.Data{DocCode: "IS"}, Fields: vizFullFields()}
	delete(r.Fields, "tipo")
	finalizeVIZ(r)
	if r.Fields["tipo"] != "SI" {
		t.Errorf("tipo non ricavato dal codice documento MRZ: %q", r.Fields["tipo"])
	}
	if !strings.Contains(strings.Join(r.Warnings, "|"), "campo incerto: tipo") {
		t.Errorf("il tipo ricavato dall'MRZ deve essere segnalato: %v", r.Warnings)
	}
	// il tipo letto non si tocca
	r2 := &Result{MRZ: &mrz.Data{DocCode: "XY"}, Fields: vizFullFields()}
	finalizeVIZ(r2)
	if r2.Fields["tipo"] != "SI" {
		t.Errorf("tipo letto sostituito: %q", r2.Fields["tipo"])
	}
}

func TestVIZFinalizeRefrendoExpectedOnlyWithLabel(t *testing.T) {
	mk := func(withLabel bool) *Result {
		r := &Result{Fields: vizFullFields()}
		delete(r.Fields, "refrendo")
		rd := vizReadings{"refrendo": {cand(0, "refrendo", "")}}
		if withLabel {
			rd[vizRefrendoLabel] = []vizCand{{0, nil}}
		}
		vizSetReadings(r, rd)
		return r
	}
	r := mk(false)
	finalizeVIZ(r)
	if strings.Contains(strings.Join(r.Warnings, "|"), "refrendo") {
		t.Errorf("senza etichetta del refrendo il campo non e' atteso: %v", r.Warnings)
	}
	r = mk(true)
	finalizeVIZ(r)
	if !strings.Contains(strings.Join(r.Warnings, "|"), "refrendo") {
		t.Errorf("con l'etichetta visibile il refrendo mancante va segnalato: %v", r.Warnings)
	}
	if !vizHasRefrendoLabel([]word{{"N° Titulo/Certificate", box{}}}) || vizHasRefrendoLabel([]word{{"PANAMA", box{}}}) {
		t.Error("riconoscimento dell'etichetta del refrendo")
	}
}

func TestVIZLocateMRZOneDirtyRow(t *testing.T) {
	// una riga sola e' sporcata a destra (bordo del libro vicino alla MRZ): la geometria usa l'intersezione
	g := vizDrawMRZ(1100, 1400, 120, 1250, 16, 0)
	for c := 44; c < 47; c++ {
		x := 120 + c*16
		for y := 1250 + 48; y < 1250+48+22; y++ { // seconda riga
			for _, ax := range []int{0, 3, 6} {
				g.SetGray(x+ax, y, color.Gray{40})
				g.SetGray(x+ax+1, y, color.Gray{40})
			}
		}
	}
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatal("MRZ non trovata")
	}
	if absf(geo.w-(43*16+8)) > 25 {
		t.Errorf("larghezza sporcata dalla riga sporca: %.0f (attesa %d)", geo.w, 43*16+8)
	}
}

func TestVIZTextClusterPrefersTallerLines(t *testing.T) {
	// i resti dell'etichetta sono piu' numerosi del valore ma piu' bassi (carattere piu' piccolo): vince il valore
	hc := func(id int, text, hw string) vizCand { return cand(id, "luogo_nascita", text, "_hw", hw) }
	cs := []vizCand{
		hc(0, "CASAL", "0.0200"), hc(1, "CASAL", "0.0192"), hc(2, "GASAL", "0.0200"), hc(3, "CASAL", "0.0195"),
		hc(4, "RECE", "0.0133"), hc(5, "RECE", "0.0138"), hc(6, "RECE OF", "0.0133"), hc(7, "RECE", "0.0131"),
		hc(8, "RECE", "0.0138"), hc(9, "RACE", "0.0140"), hc(10, "RECE", "0.0133"), hc(11, "HECE", "0.0138"),
	}
	out := &Result{Fields: map[string]string{}}
	vizVote(vizBands[vizBandIndex("luogo_nascita")], cs, func(int) float64 { return 1 }, out)
	if out.Fields["luogo_nascita"] != "CASAL" {
		t.Errorf("luogo = %q: i resti dell'etichetta non devono battere il valore", out.Fields["luogo_nascita"])
	}
	// altezze simili (campo senza resti, anche con maiuscole accentate piu' alte): conta solo il peso
	hr := func(id int, text, hw string) vizCand { return cand(id, "regionale", text, "_hw", hw) }
	rs := []vizCand{
		hr(0, "PANAMA DR", "0.0200"), hr(1, "PANAMA DR", "0.0194"), hr(2, "PANAMA DR", "0.0190"), hr(3, "PANAMA DR", "0.0200"),
		hr(4, "PANAMA DR", "0.0188"), hr(5, "PANAMA DR", "0.0215"), hr(6, "PÂNAMÂ DR", "0.0277"), hr(7, "PÂNNA DR", "0.0269"),
	}
	out = &Result{Fields: map[string]string{}}
	vizVote(vizBands[vizBandIndex("regionale")], rs, func(int) float64 { return 1 }, out)
	if out.Fields["regionale"] != "PANAMA DR" {
		t.Errorf("regionale = %q", out.Fields["regionale"])
	}
}

func TestVIZLibrettoFusedFours(t *testing.T) {
	for in, want := range map[string]string{
		"PA012U17": "PA0124417", "PA0124417": "PA0124417", "P12U456": "P1244456", "PA0U0817": "PA0440817",
	} {
		if got, ok := vizLibretto(in); !ok || got != want {
			t.Errorf("vizLibretto(%q) = %q %v, atteso %q", in, got, ok, want)
		}
	}
	if _, ok := vizLibretto("PA07UU17"); ok {
		t.Error("due U non si espandono")
	}
	if got, ok := vizLibretto("PA0456917"); !ok || got != "PA0456917" {
		t.Errorf("un numero corretto non cambia: %q", got)
	}
}

func TestVIZLocateMRZWidthFromPitch(t *testing.T) {
	// inchiostro sporco a destra su ENTRAMBE le righe (bordo del libro + ombra): gli estremi misurati sono troppo larghi,
	// il passo dei caratteri da' la larghezza giusta
	g := vizDrawMRZ(1200, 1400, 120, 1250, 16, 0)
	for _, line := range []int{0, 1} {
		for c := 44; c < 49; c++ {
			x := 120 + c*16
			top := 1250 + line*48
			for y := top; y < top+22; y++ {
				g.SetGray(x, y, color.Gray{40})
				g.SetGray(x+1, y, color.Gray{40})
			}
		}
	}
	geo, ok := vizLocateMRZ(g)
	if !ok {
		t.Fatal("MRZ non trovata")
	}
	if absf(geo.w-(43*16+8)) > 30 {
		t.Errorf("larghezza %.0f, attesa circa %d (misurata dagli estremi: oltre %d)", geo.w, 43*16+8, 47*16)
	}
}

// Fuori dalla qualifica il voto e' quello per lunghezza (le letture con apostrofo al posto della I si sommano a quelle
// con la I); il consenso per allineamento e' riservato alla qualifica.
func TestVIZTextVoteFullByLengthOutsideCargo(t *testing.T) {
	vals := []string{"PORTO MARIO", "PORT'O MARIO", "PORTO MAR1O", "PORTO MARIO", "POR"}
	ws := []float64{1, 1, 1, 1, 1}
	got, _, ok, conf := vizTextVoteFull(vals, ws, nil, false)
	if !ok || got != "PORTO MARIO" {
		t.Fatalf("voto per lunghezza: %q ok=%v", got, ok)
	}
	if conf <= 0 || conf > 1 {
		t.Errorf("confidenza fuori intervallo: %v", conf)
	}
	if got, _, ok, _ := vizTextVoteFull(vals, ws, nil, true); !ok || got == "" {
		t.Errorf("il consenso deve dare comunque un testo: %q ok=%v", got, ok)
	}
}
