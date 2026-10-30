// Package autocorr e' l'elenco GLOBALE delle autocorrezioni: «se in questo campo il programma legge X, il valore giusto e' Y».
//
// Si ricava dalle correzioni fatte dall'operatore e si puo' modificare a mano dalla scheda «Autocorrezioni». Sta in un solo file
// JSON leggibile (dati\autocorrezioni.json, nessun dato personale) e vale per TUTTI i libretti, non per uno solo.
//
// Prudenza: valgono solo per i campi con pochi valori ricorrenti (Campi: tipo, paese, nazionalita', luogo di nascita, livello,
// qualifica, regional), mai per nomi, numeri, date o sesso; si confronta il valore INTERO del campo (mai una parte), senza
// distinguere maiuscole e spazi; una regola si puo' spegnere o cancellare; quando l'operatore rimette il valore letto la regola
// si spegne da sola. Il programma segna sempre un campo autocorretto, non lo nasconde.
package autocorr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"scanlibretto/internal/memoria"
)

// Versione e' la versione del formato del file.
const Versione = 1

// Limiti: il file resta piccolo e leggibile.
const (
	MaxRegole = 500
	MaxLen    = 60
)

// Campi sono i campi per cui esistono autocorrezioni (valori ricorrenti, mai dati personali).
var Campi = map[string]bool{"type": true, "country_code": true, "nationality": true, "place_of_birth": true, "level": true, "capacity": true, "regional": true}

// Origine di una regola.
const (
	Appresa = "appresa" // nata da una correzione dell'operatore
	Manuale = "manuale" // scritta nella scheda Autocorrezioni
)

// Regola e' una autocorrezione.
type Regola struct {
	Campo     string     `json:"campo"`
	Letto     string     `json:"letto"`
	Corretto  string     `json:"corretto"`
	Attiva    bool       `json:"attiva"`
	Origine   string     `json:"origine"`
	Usi       int        `json:"usi"`                  // volte in cui e' stata applicata e confermata
	Creata    time.Time  `json:"creata"`               //
	UltimoUso *time.Time `json:"ultimo_uso,omitempty"` // assente se la regola non e' mai stata usata
}

type file struct {
	Versione int       `json:"versione"`
	Regole   []*Regola `json:"regole"`
}

// Archivio e' l'elenco su disco. Sicuro per l'uso da piu' goroutine.
type Archivio struct {
	path string
	mu   sync.Mutex
	d    file
}

// Normalizza porta un valore alla forma di confronto e di salvataggio: spazi ripuliti e maiuscole (come i campi del programma).
func Normalizza(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

func chiave(campo, letto string) string { return campo + "\x00" + Normalizza(letto) }

func (r *Regola) chiave() string { return chiave(r.Campo, r.Letto) }

// valida controlla una regola (gia' normalizzata).
func valida(campo, letto, corretto string) error {
	if !Campi[campo] {
		return fmt.Errorf("campo %q non ammesso per le autocorrezioni", campo)
	}
	if letto == "" || corretto == "" {
		return errors.New("servono il valore letto e quello corretto")
	}
	if len([]rune(letto)) > MaxLen || len([]rune(corretto)) > MaxLen {
		return fmt.Errorf("valori troppo lunghi (massimo %d caratteri)", MaxLen)
	}
	for _, s := range []string{letto, corretto} {
		for _, r := range s {
			if unicode.IsControl(r) {
				return errors.New("caratteri non validi")
			}
		}
	}
	if letto == corretto {
		return errors.New("il valore letto e quello corretto sono uguali")
	}
	return nil
}

// Apri legge il file; se manca o e' vuoto l'elenco parte vuoto (il file si crea al primo salvataggio).
func Apri(path string) (*Archivio, error) {
	a := &Archivio{path: path, d: file{Versione: Versione}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(strings.TrimSpace(string(b))) == 0) {
		return a, nil
	}
	if err != nil {
		return nil, fmt.Errorf("autocorrezioni: %w", err)
	}
	var d file
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("autocorrezioni: file %s non leggibile: %w", "autocorrezioni.json", err)
	}
	if d.Versione > Versione {
		return nil, fmt.Errorf("autocorrezioni: formato versione %d piu' recente di quello conosciuto (%d)", d.Versione, Versione)
	}
	seen := map[string]bool{}
	for _, r := range d.Regole { // un file modificato a mano: si tengono solo le regole valide e non doppie
		if r == nil {
			continue
		}
		r.Campo, r.Letto, r.Corretto = strings.TrimSpace(r.Campo), Normalizza(r.Letto), Normalizza(r.Corretto)
		if valida(r.Campo, r.Letto, r.Corretto) != nil || seen[r.chiave()] || len(a.d.Regole) >= MaxRegole {
			continue
		}
		if r.Origine != Manuale {
			r.Origine = Appresa
		}
		seen[r.chiave()] = true
		a.d.Regole = append(a.d.Regole, r)
	}
	return a, nil
}

func (a *Archivio) trova(campo, letto string) *Regola {
	k := chiave(campo, letto)
	for _, r := range a.d.Regole {
		if r.chiave() == k {
			return r
		}
	}
	return nil
}

// scrivi salva su disco in modo atomico. Va chiamata con il lucchetto preso; se fallisce lo stato in memoria e' quello di prima
// (chi chiama lavora su una copia con `prima`).
func (a *Archivio) scrivi() error {
	sort.SliceStable(a.d.Regole, func(i, j int) bool {
		if a.d.Regole[i].Campo != a.d.Regole[j].Campo {
			return a.d.Regole[i].Campo < a.d.Regole[j].Campo
		}
		return a.d.Regole[i].Letto < a.d.Regole[j].Letto
	})
	b, err := json.MarshalIndent(a.d, "", "  ")
	if err != nil {
		return err
	}
	if err := memoria.ScriviFile(a.path, append(b, '\n')); err != nil {
		return fmt.Errorf("autocorrezioni: %w", err)
	}
	return nil
}

// modifica applica f e scrive; se la scrittura fallisce rimette l'elenco com'era.
func (a *Archivio) modifica(f func() error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	prima := make([]*Regola, len(a.d.Regole))
	for i, r := range a.d.Regole {
		c := *r
		prima[i] = &c
	}
	if err := f(); err != nil {
		a.d.Regole = prima
		return err
	}
	if err := a.scrivi(); err != nil {
		a.d.Regole = prima
		return err
	}
	return nil
}

// Cerca restituisce la regola ATTIVA per il valore letto del campo.
func (a *Archivio) Cerca(campo, letto string) (Regola, bool) {
	if !Campi[campo] {
		return Regola{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.trova(campo, letto); r != nil && r.Attiva {
		return *r, true
	}
	return Regola{}, false
}

// Impara registra una correzione dell'operatore («letto» e' diventato «corretto»). Una regola nuova nasce attiva; se esiste gia'
// per lo stesso valore letto, il valore corretto si aggiorna e la regola si riattiva. Un campo non ammesso o un valore non valido
// non e' un errore: semplicemente non si impara (false).
func (a *Archivio) Impara(campo, letto, corretto string) (bool, error) {
	l, c := Normalizza(letto), Normalizza(corretto)
	if valida(campo, l, c) != nil {
		return false, nil
	}
	err := a.modifica(func() error {
		now := time.Now().UTC().Truncate(time.Second)
		if r := a.trova(campo, l); r != nil {
			if r.Corretto != c {
				r.Corretto, r.Usi = c, 0
			}
			r.Attiva = true
			return nil
		}
		if len(a.d.Regole) >= MaxRegole {
			return fmt.Errorf("troppe autocorrezioni (massimo %d): cancellane qualcuna", MaxRegole)
		}
		a.d.Regole = append(a.d.Regole, &Regola{Campo: campo, Letto: l, Corretto: c, Attiva: true, Origine: Appresa, Creata: now})
		return nil
	})
	return err == nil, err
}

// Aggiungi crea o sostituisce una regola scritta a mano (scheda Autocorrezioni). Restituisce un errore chiaro se non e' valida.
func (a *Archivio) Aggiungi(campo, letto, corretto string) error {
	l, c := Normalizza(letto), Normalizza(corretto)
	if err := valida(campo, l, c); err != nil {
		return err
	}
	return a.modifica(func() error {
		now := time.Now().UTC().Truncate(time.Second)
		if r := a.trova(campo, l); r != nil {
			r.Corretto, r.Attiva, r.Origine = c, true, Manuale
			return nil
		}
		if len(a.d.Regole) >= MaxRegole {
			return fmt.Errorf("troppe autocorrezioni (massimo %d): cancellane qualcuna", MaxRegole)
		}
		a.d.Regole = append(a.d.Regole, &Regola{Campo: campo, Letto: l, Corretto: c, Attiva: true, Origine: Manuale, Creata: now})
		return nil
	})
}

// Proposta e' una correzione candidata (da importare, per esempio dalla memoria dei libretti).
type Proposta struct{ Campo, Letto, Corretto string }

// Importa aggiunge in un colpo solo le proposte valide che NON hanno gia' una regola (le regole esistenti, anche spente o scritte a
// mano, non si toccano). Restituisce quante ne ha aggiunte; le non valide e le gia' presenti si contano come saltate.
func (a *Archivio) Importa(ps []Proposta) (aggiunte, saltate int, err error) {
	err = a.modifica(func() error {
		aggiunte, saltate = 0, 0
		now := time.Now().UTC().Truncate(time.Second)
		for _, p := range ps {
			l, c := Normalizza(p.Letto), Normalizza(p.Corretto)
			if valida(p.Campo, l, c) != nil || a.trova(p.Campo, l) != nil {
				saltate++
				continue
			}
			if len(a.d.Regole) >= MaxRegole {
				return fmt.Errorf("troppe autocorrezioni (massimo %d): cancellane qualcuna", MaxRegole)
			}
			a.d.Regole = append(a.d.Regole, &Regola{Campo: p.Campo, Letto: l, Corretto: c, Attiva: true, Origine: Appresa, Creata: now})
			aggiunte++
		}
		return nil
	})
	if err != nil {
		aggiunte = 0
	}
	return aggiunte, saltate, err
}

// Usata conta un'applicazione confermata dall'operatore.
func (a *Archivio) Usata(campo, letto string) error {
	return a.modifica(func() error {
		if r := a.trova(campo, letto); r != nil {
			r.Usi++
			t := time.Now().UTC().Truncate(time.Second)
			r.UltimoUso = &t
		}
		return nil
	})
}

// Attiva accende o spegne una regola. Errore se non esiste.
func (a *Archivio) Attiva(campo, letto string, on bool) error {
	return a.modifica(func() error {
		r := a.trova(campo, letto)
		if r == nil {
			return errors.New("autocorrezione non trovata")
		}
		r.Attiva = on
		return nil
	})
}

// Cancella toglie una regola. Errore se non esiste.
func (a *Archivio) Cancella(campo, letto string) error {
	return a.modifica(func() error {
		k := chiave(campo, letto)
		for i, r := range a.d.Regole {
			if r.chiave() == k {
				a.d.Regole = append(a.d.Regole[:i], a.d.Regole[i+1:]...)
				return nil
			}
		}
		return errors.New("autocorrezione non trovata")
	})
}

// Elenco restituisce una copia di tutte le regole (ordinate per campo e valore letto).
func (a *Archivio) Elenco() []Regola {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Regola, 0, len(a.d.Regole))
	for _, r := range a.d.Regole {
		out = append(out, *r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Campo != out[j].Campo {
			return out[i].Campo < out[j].Campo
		}
		return out[i].Letto < out[j].Letto
	})
	return out
}
