package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"scanlibretto/internal/seafarer"
)

// Ricerca automatica per Marine ID sul sito di verifica dell'AMP (internal/seafarer.SearchMarine): parte solo quando l'operatore
// preme «Cerca per Marine ID (automatico)» nel dialogo «Cerca sul sito AMP». Serve da controprova quando il QR non si legge.
// Vale per certificati e libretti: confronta i dati del foglio con quelli della pagina del marittimo e, se si conosce il numero
// del documento (dal QR o dal «Nº Título»), cerca quel documento nell'elenco.

type ampSearchReq struct {
	Name     string            `json:"name"`
	Birth    string            `json:"birth"` // come mostrato nel dialogo: GG-MES-AAAA
	MarineID string            `json:"marine_id"`
	Kind     string            `json:"kind"` // "cert" | "book"
	Fields   map[string]string `json:"fields"`
	DocNo    string            `json:"doc_number"`
}

// dmyOfMon riporta «24-OCT-1992» a «24-10-1992» (il dialogo mostra la data nella forma del sito).
func dmyOfMon(s string) string { return seafarer.DMY(s) }

// hAmpSearch: POST /api/amp/search.
func (a *App) hAmpSearch(w http.ResponseWriter, r *http.Request) {
	var req ampSearchReq
	if !readJSON(w, r, &req) {
		return
	}
	for _, s := range []string{req.Name, req.Birth, req.MarineID, req.DocNo} {
		if len([]rune(s)) > 120 || strings.ContainsFunc(s, unicode.IsControl) {
			writeErr(w, http.StatusBadRequest, "campi", "dati non validi")
			return
		}
	}
	search := a.certs.search
	if search == nil {
		search = seafarer.SearchMarine
	}
	ctx, cancel := context.WithTimeout(r.Context(), seafarer.ApexTimeout+2*1e9)
	defer cancel()
	found, err := search(ctx, req.Name, dmyOfMon(req.Birth), req.MarineID)
	if err != nil {
		a.logf("ricerca AMP per Marine ID non riuscita: %v", err)
		code := http.StatusBadGateway
		if errors.Is(err, seafarer.ErrNotFound) {
			code = http.StatusNotFound
		}
		writeErr(w, code, "amp", err.Error())
		return
	}
	rec := seafarer.Record{"marine_id": found.MarineID, "name": found.Name, "nationality": found.Nationality, "date_of_birth": found.Birth, "id": found.ID, "sex": found.Sex}
	var checks []seafarer.Check
	if req.Kind == "book" {
		checks = seafarer.CompareBook(req.Fields, rec)
	} else {
		checks = seafarer.Compare(req.Fields, rec)
	}
	var match *seafarer.DocRow
	for i := range found.Docs {
		if req.DocNo != "" && found.Docs[i].DocNumber == strings.TrimSpace(req.DocNo) {
			match = &found.Docs[i]
		}
	}
	a.logf("ricerca AMP per Marine ID: trovato, %d documenti, %d campi confrontati", len(found.Docs), len(checks))
	writeJSON(w, http.StatusOK, map[string]any{"person": found.Person, "docs": found.Docs, "more": found.More, "checks": checks, "match": match})
}
