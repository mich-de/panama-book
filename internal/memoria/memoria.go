// Package memoria e' l'archivio delle correzioni e delle conferme fatte dall'operatore ai campi letti da "libretto extract".
//
// Si memorizza PER LIBRETTO: la chiave d'identita' e' il numero libretto dell'MRZ piu' la data di nascita dell'MRZ, e
// si crea solo da un MRZ con tutti e cinque i check digit validi (Chiave). Una chiave ricavata da una lettura non
// validata non esiste: se l'MRZ non e' valida non si applica ne' si salva nulla.
//
// Due cose si ricordano per ogni campo: una CORREZIONE (valore giusto al posto di quello letto) e una CONFERMA (l'operatore
// ha guardato il valore letto e ha detto che va bene).
//
// L'archivio e' un file JSON locale (dati\memoria.json accanto all'eseguibile). CONTIENE DATI PERSONALI. La scrittura
// e' atomica (file temporaneo nella stessa cartella e poi rename) e il file ha permessi 0600.
package memoria

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"scanlibretto/internal/mrz"
)

// Versione e' la versione del formato del file (le conferme sono un campo in piu' opzionale: i file vecchi si leggono).
const Versione = 1

// ErrMRZNonValida: senza MRZ con i cinque check digit validi non esiste una chiave d'identita' del libretto.
var ErrMRZNonValida = errors.New("MRZ non valida: nessuna chiave d'identita' del libretto")

// Correzione e' un campo corretto dall'operatore.
type Correzione struct {
	Corretto string    `json:"corretto"` // valore giusto
	Letto    string    `json:"letto"`    // valore che il programma aveva letto (sbagliato)
	Data     time.Time `json:"data"`     // quando e' stato corretto
}

// Conferma e' un valore che l'operatore ha guardato e ha segnato "controllato" (senza correggerlo).
type Conferma struct {
	Valore string    `json:"valore"` // valore confermato per questo libretto
	Data   time.Time `json:"data"`   // quando e' stato confermato
}

// Libretto raccoglie le correzioni e le conferme di un libretto.
type Libretto struct {
	Etichetta  string                `json:"etichetta"` // per riconoscerlo nell'elenco (cognome e nomi dell'MRZ)
	Numero     string                `json:"numero"`    // numero libretto (dell'MRZ validata)
	Aggiornato time.Time             `json:"aggiornato"`
	Campi      map[string]Correzione `json:"campi"`
	Confermati map[string]Conferma   `json:"confermati,omitempty"`
}

func (l *Libretto) clone() *Libretto {
	n := *l
	n.Campi = make(map[string]Correzione, len(l.Campi))
	for k, v := range l.Campi {
		n.Campi[k] = v
	}
	n.Confermati = make(map[string]Conferma, len(l.Confermati))
	for k, v := range l.Confermati {
		n.Confermati[k] = v
	}
	return &n
}

func (l *Libretto) vuoto() bool { return len(l.Campi) == 0 && len(l.Confermati) == 0 }

func (l *Libretto) ident(now time.Time, etichetta, numero string) {
	l.Aggiornato = now
	if etichetta != "" {
		l.Etichetta = etichetta
	}
	if numero != "" {
		l.Numero = numero
	}
}

type file struct {
	Versione int                  `json:"versione"`
	Libretti map[string]*Libretto `json:"libretti"`
}

// Archivio e' l'archivio su disco. Sicuro per l'uso da piu' goroutine.
type Archivio struct {
	path string
	mu   sync.Mutex
	d    file
}

// Apri legge l'archivio; un file assente da' un archivio vuoto (non lo crea finche' non si salva qualcosa).
func Apri(path string) (*Archivio, error) {
	a := &Archivio{path: path, d: file{Versione: Versione, Libretti: map[string]*Libretto{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memoria: %w", err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return a, nil
	}
	var d file
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("memoria: file %s non leggibile: %w", filepath.Base(path), err)
	}
	if d.Versione > Versione {
		return nil, fmt.Errorf("memoria: formato versione %d piu' recente di quello conosciuto (%d)", d.Versione, Versione)
	}
	if d.Libretti == nil {
		d.Libretti = map[string]*Libretto{}
	}
	for k, l := range d.Libretti {
		if l == nil {
			delete(d.Libretti, k)
			continue
		}
		if l.Campi == nil {
			l.Campi = map[string]Correzione{}
		}
		if l.Confermati == nil {
			l.Confermati = map[string]Conferma{}
		}
	}
	d.Versione = Versione
	a.d = d
	return a, nil
}

// Chiave ricava la chiave d'identita' dall'MRZ. Errore (ErrMRZNonValida) se manca o se un check digit non torna.
func Chiave(m *mrz.Data) (string, error) {
	if m == nil {
		return "", ErrMRZNonValida
	}
	c := m.Checks
	if !(c.DocNumber && c.Birth && c.Expiry && c.Optional && c.Composite) {
		return "", ErrMRZNonValida
	}
	num := strings.Trim(strings.ToUpper(strings.TrimSpace(m.DocNumber)), "<")
	nascita := strings.TrimSpace(m.BirthDate)
	if num == "" || nascita == "" {
		return "", ErrMRZNonValida
	}
	return num + "|" + nascita, nil
}

// Campi restituisce una copia delle correzioni memorizzate per il libretto (mappa vuota se non c'e').
func (a *Archivio) Campi(chiave string) map[string]Correzione {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]Correzione{}
	if l := a.d.Libretti[chiave]; l != nil {
		for k, v := range l.Campi {
			out[k] = v
		}
	}
	return out
}

// Confermati restituisce una copia delle conferme ("valore X controllato") del libretto.
func (a *Archivio) Confermati(chiave string) map[string]Conferma {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[string]Conferma{}
	if l := a.d.Libretti[chiave]; l != nil {
		for k, v := range l.Confermati {
			out[k] = v
		}
	}
	return out
}

// modifica applica f a una copia del libretto (creato se manca e crea e' vero) e scrive il file; se la scrittura fallisce
// lo stato in memoria resta quello di prima. Un libretto che resta senza correzioni e senza conferme sparisce.
func (a *Archivio) modifica(chiave string, crea bool, f func(l *Libretto, now time.Time)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	vecchio := a.d.Libretti[chiave]
	if vecchio == nil && !crea {
		return nil
	}
	nuovo := &Libretto{Campi: map[string]Correzione{}, Confermati: map[string]Conferma{}}
	if vecchio != nil {
		nuovo = vecchio.clone()
	}
	f(nuovo, time.Now().UTC().Truncate(time.Second))
	if nuovo.vuoto() {
		delete(a.d.Libretti, chiave)
	} else {
		a.d.Libretti[chiave] = nuovo
	}
	if err := a.scrivi(); err != nil {
		if vecchio == nil {
			delete(a.d.Libretti, chiave)
		} else {
			a.d.Libretti[chiave] = vecchio
		}
		return err
	}
	return nil
}

// Salva memorizza le correzioni (un campo gia' presente viene sostituito) e scrive il file. Con corr vuoto non fa nulla.
// Il campo Data vuoto diventa l'ora corrente. etichetta e numero servono solo a riconoscere il libretto nell'elenco.
// Una correzione sostituisce l'eventuale conferma dello stesso campo.
func (a *Archivio) Salva(chiave, etichetta, numero string, corr map[string]Correzione) error {
	if chiave == "" {
		return ErrMRZNonValida
	}
	if len(corr) == 0 {
		return nil
	}
	return a.modifica(chiave, true, func(l *Libretto, now time.Time) {
		for k, c := range corr {
			if c.Data.IsZero() {
				c.Data = now
			}
			l.Campi[k] = c
			delete(l.Confermati, k)
		}
		l.ident(now, etichetta, numero)
	})
}

// SalvaConferme memorizza "valore X controllato per questo libretto" (un campo gia' presente viene sostituito).
func (a *Archivio) SalvaConferme(chiave, etichetta, numero string, conf map[string]string) error {
	if chiave == "" {
		return ErrMRZNonValida
	}
	if len(conf) == 0 {
		return nil
	}
	return a.modifica(chiave, true, func(l *Libretto, now time.Time) {
		for k, v := range conf {
			l.Confermati[k] = Conferma{Valore: v, Data: now}
		}
		l.ident(now, etichetta, numero)
	})
}

// RimuoviConferme toglie le conferme dei campi indicati.
func (a *Archivio) RimuoviConferme(chiave string, campi ...string) error {
	return a.modifica(chiave, false, func(l *Libretto, now time.Time) {
		for _, c := range campi {
			delete(l.Confermati, c)
		}
	})
}

// DimenticaCampo toglie una correzione ("questo era giusto, dimentica"). Un libretto senza piu' nulla sparisce.
func (a *Archivio) DimenticaCampo(chiave, campo string) error {
	return a.modifica(chiave, false, func(l *Libretto, now time.Time) { delete(l.Campi, campo) })
}

// DimenticaLibretto toglie tutto cio' che si ricorda di un libretto.
func (a *Archivio) DimenticaLibretto(chiave string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	l, ok := a.d.Libretti[chiave]
	if !ok {
		return nil
	}
	delete(a.d.Libretti, chiave)
	if err := a.scrivi(); err != nil {
		a.d.Libretti[chiave] = l
		return err
	}
	return nil
}

// Voce e' una riga dell'elenco dei libretti ricordati.
type Voce struct {
	Chiave     string                `json:"chiave"`
	Etichetta  string                `json:"etichetta"`
	Numero     string                `json:"numero"`
	Aggiornato time.Time             `json:"aggiornato"`
	Campi      map[string]Correzione `json:"campi"`
	Confermati map[string]Conferma   `json:"confermati"`
}

// Elenco restituisce i libretti ricordati, i piu' recenti per primi.
func (a *Archivio) Elenco() []Voce {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Voce, 0, len(a.d.Libretti))
	for k, l := range a.d.Libretti {
		c := l.clone()
		out = append(out, Voce{Chiave: k, Etichetta: l.Etichetta, Numero: l.Numero, Aggiornato: l.Aggiornato, Campi: c.Campi, Confermati: c.Confermati})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Aggiornato.Equal(out[j].Aggiornato) {
			return out[i].Aggiornato.After(out[j].Aggiornato)
		}
		return out[i].Chiave < out[j].Chiave
	})
	return out
}

// scrivi salva su disco in modo atomico. Va chiamata con il lucchetto preso.
func (a *Archivio) scrivi() error {
	b, err := json.MarshalIndent(a.d, "", "  ")
	if err != nil {
		return err
	}
	if err := ScriviFile(a.path, b); err != nil {
		return fmt.Errorf("memoria: %w", err)
	}
	return nil
}

// ScriviFile scrive b in path in modo atomico: file temporaneo nella stessa cartella (permessi 0600), sync e rename.
// Crea la cartella (0700) se manca. Un guasto non lascia ne' file a meta' ne' residui temporanei.
func ScriviFile(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".scrittura-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
