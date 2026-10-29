package libreta

// Rifinitura dei campi stampati dopo la lettura (finalizeVIZ): nuovo voto con i pesi calibrati sull'MRZ, normalizzazione,
// riconciliazione con l'MRZ e avvisi. Regola di fondo: l'MRZ prevale perche' i suoi check digit lo rendono piu' affidabile
// dell'OCR della zona visiva. Sei campi stanno anche nell'MRZ (tipo, codice paese, nazionalita' tramite il codice ISO,
// data di nascita, data di scadenza, numero libretto) e, sul modello 2026, il campo opzionale e' il nome dell'ufficio
// regionale: con l'MRZ valido quei campi si riempiono se la lettura stampata manca e si correggono se e' diversa (con
// un avviso senza valori). Misurato sulle verita': la zona visiva e l'MRZ coincidono sempre dove l'MRZ e' stato letto.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"scanlibretto/internal/mrz"
)

var reLibrettoMRZ = regexp.MustCompile(`^PA?[0-9]{7}$`)

// vizIsMRZName e' vero se il testo e' il nome o il cognome dell'MRZ (ogni parola somiglia a una parola del nome).
func vizIsMRZName(v string, m *mrz.Data) bool {
	words := strings.Fields(strings.ToLower(norm2(m.Surname + " " + m.GivenNames)))
	toks := strings.Fields(strings.ToLower(norm2(v)))
	if len(toks) == 0 || len(words) == 0 {
		return false
	}
	for _, t := range toks {
		ok := false
		for _, w := range words {
			if vizSim(t, w) >= 0.6 {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// vizNatClean: l'aggettivo di nazionalita' ha solo lettere; l'apostrofo e la barra letti al posto della I sono errori OCR.
func vizNatClean(v string) string {
	return strings.NewReplacer("'", "I", "|", "I", "!", "I").Replace(v)
}

// vizFinalize normalizza date e numeri e confronta zona visiva e MRZ.
func vizFinalize(r *Result) {
	if r.Fields == nil {
		return
	}
	st := &vizFinState{r: r}
	st.revote()
	st.normalize()
	st.reconcileNationality()
	st.reconcileMRZ()
	st.missing()
	st.compareMRZ()
	vizCoherence(r) // dopo le sostituzioni con l'MRZ: una data letta male e gia' corretta non e' un'incoerenza
	vizWarnUncertain(r, st.uncertain, st.fixed)
}

// vizFinState e' lo stato di vizFinalize: i campi incerti (da avvisare) e quelli sostituiti dall'MRZ.
type vizFinState struct {
	r               *Result
	uncertain       []string
	fixed           []string
	noRefrendoLabel bool
	natReads        []string
	sexViz          string
}

func (st *vizFinState) markUncertain(k string) { st.uncertain = append(st.uncertain, k) }

func (st *vizFinState) clearUncertain(k string) {
	st.uncertain = slices.DeleteFunc(st.uncertain, func(x string) bool { return x == k })
}

// revote: nuovo voto tra le varianti con i pesi calibrati sull'MRZ (se le letture sono di questo Result).
func (st *vizFinState) revote() {
	r := st.r
	rd := vizTakeReadings(r)
	if rd == nil {
		return
	}
	st.noRefrendoLabel = rd[vizRefrendoLabel] == nil && rd["refrendo"] != nil
	tmp := &Result{Fields: map[string]string{}}
	wts := vizMRZWeights(rd, r.MRZ)
	if vizTraceWeights != nil {
		vizTraceWeights(wts)
	}
	st.uncertain = vizVoteAll(rd, wts, tmp)
	vizTakeName(r, tmp.Fields)
	vizStripInternal(tmp.Fields)
	for k, v := range tmp.Fields {
		r.Fields[k] = v
	}
	for _, c := range rd["nacionalidad"] {
		if v := c.f["nacionalidad"]; v != "" {
			st.natReads = append(st.natReads, vizNatClean(v))
		}
	}
}

// normalize: date nella forma stampata DD-MM-YYYY e ISO coerenti, maiuscole, sesso stampato messo da parte.
func (st *vizFinState) normalize() {
	r := st.r
	for _, key := range []string{"data_emissione", "data_scadenza", "data_nascita"} {
		v := r.Fields[key]
		if v == "" {
			continue
		}
		if d, ok := vizPickDate(vizDates(v), key); ok {
			vizSetDate(r, key, d)
		} else {
			delete(r.Fields, key)
			delete(r.Fields, key+"_iso")
			r.Warnings = append(r.Warnings, vizFieldName(key)+" stampata non interpretabile come data")
		}
	}
	for _, key := range []string{"numero_libretto", "identificazione", "livello"} {
		if v := r.Fields[key]; v != "" {
			r.Fields[key] = strings.ToUpper(strings.TrimSpace(v))
		}
	}
	st.sexViz = r.Fields["sesso_viz"]
	delete(r.Fields, "sesso_viz")
}

// reconcileNationality: l'aggettivo di nazionalita' si controlla contro il codice ISO dell'MRZ (tabella completa). Una
// lettura uguale al nome dell'MRZ e' la riga del nome (il ritaglio la comprende), non l'aggettivo. Se una lettura
// somiglia a un aggettivo atteso per il codice si usa quello; altrimenti, con l'MRZ valido (5 check digit su 5), prevale
// l'aggettivo del codice; con l'MRZ non verificato resta il testo letto e il campo e' incerto.
func (st *vizFinState) reconcileNationality() {
	r, m := st.r, st.r.MRZ
	if m == nil || len(vizNationalities[m.Nationality]) == 0 {
		return
	}
	var reads []string
	for _, v := range append([]string{vizNatClean(r.Fields["nacionalidad"])}, st.natReads...) {
		if v != "" && !vizIsMRZName(v, m) {
			reads = append(reads, v)
		}
	}
	cur := r.Fields["nacionalidad"]
	if cur != "" && vizIsMRZName(vizNatClean(cur), m) {
		cur = ""
		delete(r.Fields, "nacionalidad")
	}
	adj, ok := vizExpectedNationality(m.Nationality, reads)
	switch {
	case ok:
		r.Fields["nacionalidad"] = adj
		st.clearUncertain("nacionalidad") // ora e' verificata
	case m.Checks.AllOK():
		// nessuna lettura utilizzabile: l'aggettivo del codice MRZ prevale; incerto solo se il paese ha piu' aggettivi
		r.Fields["nacionalidad"] = vizNationalities[m.Nationality][0]
		st.clearUncertain("nacionalidad")
		if len(vizNationalities[m.Nationality]) > 1 {
			st.markUncertain("nacionalidad")
		}
	case cur != "":
		st.markUncertain("nacionalidad")
	default:
		r.Fields["nacionalidad"] = vizNationalities[m.Nationality][0]
		st.markUncertain("nacionalidad")
	}
}

// reconcileMRZ riempie o corregge dall'MRZ i campi che l'MRZ ha: date, numero libretto, tipo, codice paese, regionale.
// Date e numero si fidano del proprio check digit; tipo, paese e regionale non hanno un check proprio sulla riga 1: il
// controllo composito della riga 2 (tutti e cinque i check validi) e' la miglior garanzia che l'MRZ sia stato letto bene.
func (st *vizFinState) reconcileMRZ() {
	r, m := st.r, st.r.MRZ
	if m == nil {
		return
	}
	all := m.Checks.AllOK()
	date := func(key, mrzISO string, valid bool) {
		if len(mrzISO) != 10 {
			return
		}
		cur := r.Fields[key+"_iso"]
		if !valid { // check digit non valido: l'MRZ non e' una verita', si segnala soltanto la differenza
			if len(cur) >= 8 && cur[len(cur)-8:] != mrzISO[len(mrzISO)-8:] {
				r.Warnings = append(r.Warnings, vizFieldName(key)+" stampata diversa dall'MRZ")
			}
			return
		}
		var y, mo, d int
		if n, _ := fmt.Sscanf(mrzISO, "%d-%d-%d", &y, &mo, &d); n != 3 || !vizValidDate(d, mo, y) {
			return
		}
		switch {
		case len(cur) >= 8 && cur[len(cur)-8:] == mrzISO[len(mrzISO)-8:]:
			// stessa data (l'MRZ ha solo due cifre per l'anno: si tiene il secolo stampato)
		case len(cur) >= 8:
			r.Warnings = append(r.Warnings, vizFieldName(key)+" stampata diversa dall'MRZ")
			vizSetDate(r, key, vizDate{d, mo, y})
			st.fixed = append(st.fixed, key)
		default: // non letta: la data dell'MRZ e' quella stampata (la verita' coincide su tutte le immagini con l'MRZ letto)
			vizSetDate(r, key, vizDate{d, mo, y})
			st.fixed = append(st.fixed, key)
		}
	}
	date("data_nascita", m.BirthDate, m.Checks.Birth)
	date("data_scadenza", m.ExpiryDate, m.Checks.Expiry)

	doc := strings.TrimRight(m.DocNumber, "<")
	if n := r.Fields["numero_libretto"]; !m.Checks.DocNumber && n != "" && doc != "" && n != doc && !vizHasWarning(r, "numero libretto stampato") {
		r.Warnings = append(r.Warnings, "numero libretto stampato diverso dall'MRZ")
	}
	if m.Checks.DocNumber && reLibrettoMRZ.MatchString(doc) {
		switch cur := r.Fields["numero_libretto"]; {
		case cur == doc:
		case cur != "":
			if !vizHasWarning(r, "numero libretto stampato") {
				r.Warnings = append(r.Warnings, "numero libretto stampato diverso dall'MRZ")
			}
			r.Fields["numero_libretto"] = doc
			st.fixed = append(st.fixed, "numero_libretto")
		default:
			r.Fields["numero_libretto"] = doc
			st.fixed = append(st.fixed, "numero_libretto")
		}
	}

	// il codice paese (PAN) sta accanto al tipo: l'emittente dell'MRZ
	if iss := strings.TrimRight(m.Issuer, "<"); len(iss) == 3 {
		switch cur := r.Fields["paese"]; {
		case cur == "" && all:
			r.Fields["paese"] = iss
			st.fixed = append(st.fixed, "paese")
		case cur == "":
			r.Fields["paese"] = iss
			st.markUncertain("paese")
		case all && cur != iss:
			r.Fields["paese"] = iss
			st.fixed = append(st.fixed, "paese")
		}
	}
	// il tipo ("SI") sta su un ologramma e spesso non si legge: il codice documento dell'MRZ e' lo stesso a lettere invertite
	if dc := strings.TrimRight(m.DocCode, "<"); len(dc) == 2 {
		rev := string([]byte{dc[1], dc[0]})
		switch cur := r.Fields["tipo"]; {
		case cur == "" && all:
			r.Fields["tipo"] = rev
			st.fixed = append(st.fixed, "tipo")
		case cur == "":
			r.Fields["tipo"] = rev
			st.markUncertain("tipo")
		case all && cur != rev:
			r.Fields["tipo"] = rev
			st.fixed = append(st.fixed, "tipo")
		}
	}

	// ufficio regionale: sul modello 2026 il campo opzionale dell'MRZ e' il nome dell'ufficio; sui facsimili e'
	// il solo prefisso ("PANAMA" per "PANAMA DR"), quindi una lettura stampata che comincia con l'MRZ resta
	if opt := strings.TrimSpace(strings.ReplaceAll(m.Optional, "<", " ")); opt != "" && m.Checks.Optional {
		cur := r.Fields["regionale"]
		switch {
		case cur == "":
			r.Fields["regionale"] = opt
			st.markUncertain("regionale") // potrebbe essere il solo prefisso del nome stampato
		case strings.HasPrefix(norm(cur), norm(opt)):
		case vizSim(norm(cur), norm(opt)) >= 0.7:
			r.Fields["regionale"] = opt
			st.fixed = append(st.fixed, "regionale")
			st.clearUncertain("regionale")
		}
	}
}

// missing: avviso sui campi stampati non letti (il refrendo esiste solo su alcuni modelli: serve il numero "PA"+cifre e
// che almeno una variante ne abbia visto l'etichetta).
func (st *vizFinState) missing() {
	r := st.r
	var missing []string
	for _, key := range []string{"paese", "tipo", "nacionalidad", "luogo_nascita", "data_emissione", "data_scadenza",
		"data_nascita", "livello", "cargo", "numero_libretto", "identificazione", "id_marino", "regionale"} {
		if r.Fields[key] == "" {
			missing = append(missing, key)
		}
	}
	if strings.HasPrefix(r.Fields["numero_libretto"], "PA") && r.Fields["refrendo"] == "" && !st.noRefrendoLabel {
		missing = append(missing, "refrendo")
	}
	if len(missing) > 0 {
		r.Warnings = append(r.Warnings, "campi stampati non letti: "+strings.Join(missing, ", "))
	}
}

// compareMRZ: confronti che producono solo avvisi (sesso, nazionalita', regionale).
func (st *vizFinState) compareMRZ() {
	r, m := st.r, st.r.MRZ
	if m == nil {
		return
	}
	if st.sexViz != "" && m.Sex != "" && st.sexViz != m.Sex {
		r.Warnings = append(r.Warnings, "sesso stampato diverso dall'MRZ")
	}
	if nat := strings.ToUpper(r.Fields["nacionalidad"]); nat != "" && m.Nationality != "" {
		bestCode, bestS := "", 0.0
		for dem, code := range vizDemonyms {
			if s := vizSim(norm(nat), norm(dem)); s > bestS {
				bestCode, bestS = code, s
			}
		}
		if bestS >= 0.75 && bestCode != m.Nationality {
			r.Warnings = append(r.Warnings, "nazionalita' stampata diversa dall'MRZ")
		}
	}
	if reg, opt := norm(r.Fields["regionale"]), norm(strings.ReplaceAll(m.Optional, "<", " ")); reg != "" && opt != "" {
		if vizSim(reg, opt) < 0.7 && !strings.HasPrefix(reg, opt) {
			r.Warnings = append(r.Warnings, "regionale stampato diverso dall'MRZ")
		}
	}
}

// vizCoherence controlla l'ordine delle date stampate (nascita < emissione < scadenza).
func vizCoherence(r *Result) {
	iss, exp, birth := r.Fields["data_emissione_iso"], r.Fields["data_scadenza_iso"], r.Fields["data_nascita_iso"]
	if iss != "" && exp != "" && exp <= iss {
		r.Warnings = append(r.Warnings, "data di scadenza stampata non successiva alla data di emissione")
	}
	if birth != "" && iss != "" && birth >= iss {
		r.Warnings = append(r.Warnings, "data di nascita stampata non precedente alla data di emissione")
	}
}

func vizHasWarning(r *Result, prefix string) bool {
	for _, w := range r.Warnings {
		if strings.HasPrefix(w, prefix) {
			return true
		}
	}
	return false
}

func vizFieldName(key string) string {
	switch key {
	case "data_nascita":
		return "data di nascita"
	case "data_scadenza":
		return "data di scadenza"
	case "data_emissione":
		return "data di emissione"
	}
	return key
}

// vizWarnUncertain aggiunge un avviso "campo incerto" per ogni campo (numerico o di testo libero) con letture scarse
// o discordanti tra le varianti, salvo quelli che l'MRZ ha verificato (check digit validi: date e numero libretto) o
// sostituito (fixed). Senza valori.
func vizWarnUncertain(r *Result, uncertain, fixed []string) {
	skip := map[string]bool{}
	for _, k := range fixed {
		skip[k] = true
	}
	if m := r.MRZ; m != nil {
		skip["data_nascita"] = skip["data_nascita"] || m.Checks.Birth
		skip["data_scadenza"] = skip["data_scadenza"] || m.Checks.Expiry
		skip["numero_libretto"] = skip["numero_libretto"] || m.Checks.DocNumber
	}
	seen := map[string]bool{}
	for _, k := range uncertain {
		switch k {
		case "id_marino", "refrendo", "identificazione", "numero_libretto", "data_emissione", "data_scadenza", "data_nascita",
			"luogo_nascita", "cargo", "regionale", "nacionalidad", "paese", "tipo":
			if !skip[k] && !seen[k] && r.Fields[k] != "" {
				seen[k] = true
				r.Warnings = append(r.Warnings, "campo incerto: "+k+" (letture scarse o discordanti tra le varianti)")
			}
		}
	}
}
