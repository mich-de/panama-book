package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"scanlibretto/internal/outsum"
)

// ErrBadTruth e' l'errore di un file di verita' che non si legge (JSON rotto, campo che non e' una stringa).
var ErrBadTruth = errors.New("verita' non valida")

// Truth e' la verita' di una immagine: i 14 valori stampati letti a occhio dalla pagina dati. Contiene dati
// personali (nomi, date, numeri): non va mai stampata ne' scritta in un rapporto.
type Truth struct {
	// Fields ha una voce per ognuna delle chiavi di outsum.FieldKeys. Stringa vuota = campo non stampato su quel
	// modello, fuori dall'immagine o illeggibile: non si confronta.
	Fields map[string]string
	// Notes spiega perche' un campo e' vuoto o difficile (chiave "_note" del file). Solo testo libero per chi legge il file.
	Notes map[string]string
	// Absent elenca le chiavi di outsum.FieldKeys che mancano dal file (trattate come vuote): di solito e' un refuso.
	Absent []string
	// Kind e' il tipo di immagine dichiarato nel file ("_tipo"), se c'e'.
	Kind string
	// NameRow e' il testo della riga del nome stampata (nomi e cognome, "_riga_nome"), se il file lo riporta. Serve
	// solo a riconoscere un campo che ha letto quella riga invece del suo testo; non e' uno dei 14 campi.
	NameRow string
}

// ParseTruth legge un file di verita'. I 14 nomi inglesi dei campi (outsum.FieldKeys) possono stare al livello
// superiore oppure dentro "fields"; il campo `certificate` si legge anche con il vecchio nome `endorsement` (alias.go).
// Le chiavi che cominciano per "_" sono metadati; le altre sconosciute si ignorano. Gli errori citano solo i nomi dei
// campi, mai i valori.
func ParseTruth(b []byte) (Truth, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return Truth{}, fmt.Errorf("%w: %v", ErrBadTruth, err)
	}
	src := top
	if raw, ok := top["fields"]; ok {
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err != nil {
			return Truth{}, fmt.Errorf("%w: \"fields\" non e' un oggetto", ErrBadTruth)
		}
		src = inner
	}
	t := Truth{Fields: make(map[string]string, outsum.NFields), Notes: map[string]string{}}
	for _, k := range outsum.FieldKeys {
		raw, ok := pickRaw(src, k)
		if !ok {
			raw, ok = pickRaw(top, k) // campo al livello superiore anche se c'e' "fields"
		}
		if !ok {
			t.Fields[k] = ""
			t.Absent = append(t.Absent, k)
			continue
		}
		if string(raw) == "null" {
			t.Fields[k] = ""
			continue
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return Truth{}, fmt.Errorf("%w: il campo %s non e' una stringa", ErrBadTruth, k)
		}
		t.Fields[k] = s
	}
	if raw, ok := top["_note"]; ok {
		var notes map[string]string
		if err := json.Unmarshal(raw, &notes); err == nil {
			for k, v := range notes {
				t.Notes[Canonical(k)] = v // anche una nota scritta con il vecchio nome del campo
			}
		}
	}
	if raw, ok := top["_tipo"]; ok {
		_ = json.Unmarshal(raw, &t.Kind)
	}
	if raw, ok := top["_riga_nome"]; ok {
		_ = json.Unmarshal(raw, &t.NameRow)
	}
	return t, nil
}

// pickRaw cerca un campo per nome e, se manca, per i suoi nomi alternativi (vedi alias.go).
func pickRaw(m map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	if raw, ok := m[key]; ok {
		return raw, true
	}
	for _, a := range aliasesOf(key) {
		if raw, ok := m[a]; ok {
			return raw, true
		}
	}
	return nil, false
}

// LoadTruth legge un file di verita' da disco.
func LoadTruth(path string) (Truth, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Truth{}, err
	}
	t, err := ParseTruth(b)
	if err != nil {
		return Truth{}, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}
