package main

import (
	"errors"
	"net/http"
	"strings"

	"scanlibretto/internal/autocorr"
)

// Autocorrezioni globali (scheda «Autocorrezioni»): vedi internal/autocorr. Qui: apprendimento al salvataggio e rotte della scheda.

// updateRules aggiorna le autocorrezioni dopo un salvataggio, confrontando il valore finale di ogni campo ammesso con quello letto:
//   - l'operatore ha corretto un valore letto  -> si impara (vale da ora per tutti i libretti);
//   - ha rimesso il valore letto su un campo autocorretto -> la regola si spegne (aveva torto);
//   - ha lasciato il valore autocorretto e ha confermato -> la regola conta un uso.
//
// Un guasto nel file non blocca il salvataggio: si scrive nel registro e si avvisa.
func (a *App) updateRules(d *Doc, final map[string]string, mode string, msgs *[]string) {
	if a.rules == nil {
		return
	}
	for _, f := range fieldDefs {
		if !autocorr.Campi[f.Key] {
			continue
		}
		read, fin := d.Read[f.Key], final[f.Key]
		ai, wasAuto := d.Auto[f.Key]
		switch {
		case wasAuto && fin == read:
			if err := a.rules.Attiva(f.Key, read, false); err != nil {
				a.logf("autocorrezioni: non riesco a spegnere %s %q: %v", f.Key, read, err)
				continue
			}
			*msgs = append(*msgs, "Autocorrezione spenta per «"+f.ES+" / "+f.EN+"»: «"+read+"» non diventa più «"+ai.Fixed+"» (la riattivi dalla scheda Autocorrezioni).")
		case wasAuto && fin == ai.Fixed:
			if mode == "confirm" {
				if err := a.rules.Usata(f.Key, read); err != nil {
					a.logf("autocorrezioni: uso non registrato %s %q: %v", f.Key, read, err)
				}
			}
		case read != "" && fin != "" && fin != read:
			if ap, ok := d.Applied[f.Key]; ok && ap.Corretto == fin {
				continue // valore messo dalla memoria di questo libretto: non e' una correzione nuova
			}
			if cur, ok := a.rules.Cerca(f.Key, read); ok && cur.Corretto == fin {
				continue
			}
			learned, err := a.rules.Impara(f.Key, read, fin)
			if err != nil {
				a.logf("autocorrezioni: non riesco a salvare %s %q: %v", f.Key, read, err)
				*msgs = append(*msgs, "Autocorrezione NON salvata: "+err.Error())
			} else if learned {
				*msgs = append(*msgs, "Autocorrezione imparata per «"+f.ES+" / "+f.EN+"»: «"+autocorr.Normalizza(read)+"» diventa «"+fin+"». Vale per tutti i libretti (scheda Autocorrezioni).")
			}
		}
	}
}

var errAction = errors.New("azione sconosciuta")

type autoFieldView struct {
	Key string `json:"key"`
	ES  string `json:"es"`
	EN  string `json:"en"`
}

// hAutocorrList: GET /api/autocorr, l'elenco delle regole e i campi ammessi.
func (a *App) hAutocorrList(w http.ResponseWriter, r *http.Request) { a.writeAutocorr(w, nil) }

// writeAutocorr risponde con l'elenco; dopo un'importazione aggiunge anche il conteggio.
func (a *App) writeAutocorr(w http.ResponseWriter, imp *importResult) {
	var fields []autoFieldView
	for _, f := range fieldDefs {
		if autocorr.Campi[f.Key] {
			fields = append(fields, autoFieldView{Key: f.Key, ES: f.ES, EN: f.EN})
		}
	}
	out := map[string]any{"rules": a.rules.Elenco(), "fields": fields, "max": autocorr.MaxRegole, "max_len": autocorr.MaxLen}
	if imp != nil {
		out["import"] = imp
	}
	writeJSON(w, http.StatusOK, out)
}

// importResult dice com'e' andata l'importazione dalla memoria dei libretti.
type importResult struct {
	Added     int `json:"added"`
	Skipped   int `json:"skipped"`   // gia' presenti o non valide
	Ambiguous int `json:"ambiguous"` // stesso valore letto corretto in modi diversi, senza un vincitore: non si importano
}

// importFromMemory crea le autocorrezioni dalle correzioni gia' ricordate per i singoli libretti (file dati\memoria.json): per ogni
// campo ammesso e valore letto sceglie la correzione piu' frequente tra i libretti; se due correzioni sono in pari non importa
// nessuna (e lo conta come ambigua). Non cambia le regole che esistono gia'.
func (a *App) importFromMemory() (importResult, error) {
	type k struct{ campo, letto string }
	count := map[k]map[string]int{}
	for _, v := range a.mem.Elenco() {
		for campo, c := range v.Campi {
			if !autocorr.Campi[campo] {
				continue
			}
			key := k{campo, autocorr.Normalizza(c.Letto)}
			if count[key] == nil {
				count[key] = map[string]int{}
			}
			count[key][autocorr.Normalizza(c.Corretto)]++
		}
	}
	var res importResult
	var ps []autocorr.Proposta
	for key, m := range count {
		best, second, val := 0, 0, ""
		for c, n := range m {
			switch {
			case n > best:
				second, best, val = best, n, c
			case n > second:
				second = n
			}
		}
		if best == second {
			res.Ambiguous++
			continue
		}
		ps = append(ps, autocorr.Proposta{Campo: key.campo, Letto: key.letto, Corretto: val})
	}
	added, skipped, err := a.rules.Importa(ps)
	res.Added, res.Skipped = added, skipped
	return res, err
}

// hAutocorrSet: POST /api/autocorr {action: add | toggle | delete, field, read, fixed?, on?}. Risponde con l'elenco aggiornato.
func (a *App) hAutocorrSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
		Field  string `json:"field"`
		Read   string `json:"read"`
		Fixed  string `json:"fixed"`
		On     bool   `json:"on"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var err error
	switch req.Action {
	case "add":
		err = a.rules.Aggiungi(req.Field, req.Read, req.Fixed)
	case "toggle":
		err = a.rules.Attiva(req.Field, req.Read, req.On)
	case "delete":
		err = a.rules.Cancella(req.Field, req.Read)
	case "import":
		var res importResult
		if res, err = a.importFromMemory(); err == nil {
			a.writeAutocorr(w, &res)
			return
		}
	default:
		err = errAction
	}
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "autocorrezioni:") {
			code = http.StatusInternalServerError
		}
		writeErr(w, code, "autocorrezioni", err.Error())
		return
	}
	a.writeAutocorr(w, nil)
}
