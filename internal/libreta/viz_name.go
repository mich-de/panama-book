package libreta

// Nome stampato. La riga "Nombres/Given Names Apellidos/Surname" della pagina dati ha il nome e il cognome del titolare
// in una sola riga di maiuscole. Non e' uno dei 14 campi e non entra in Result.Fields (e quindi nel JSON): si espone con
// vizPrintedName perche' extract.go lo confronti con il cognome e i nomi dell'MRZ, la cui riga 1 non ha check digit
// (una lettura storpiata passa inosservata con i cinque check della riga 2 validi).

import (
	"strconv"
	"strings"
	"sync"
)

var vizNameStore struct {
	mu    sync.Mutex
	owner *Result
	name  string
	conf  float64
}

func vizSetPrintedName(owner *Result, name string, conf float64) {
	vizNameStore.mu.Lock()
	vizNameStore.owner, vizNameStore.name, vizNameStore.conf = owner, name, conf
	vizNameStore.mu.Unlock()
}

// vizPrintedName restituisce il nome stampato letto da extractVIZ per r: testo in maiuscolo (nome e cognome nell'ordine
// stampato, senza simboli spuri) e una confidenza in [0,1] (accordo tra le letture delle varianti: 1 = tutte uguali, sotto
// 0,5 la lettura e' debole). ok e' falso se il campo non e' stato letto (nessuna MRZ trovata, riga illeggibile). Va chiamata
// dopo extractVIZ e vale fino alla prossima chiamata di extractVIZ; finalizeVIZ la aggiorna con il nuovo voto.
func vizPrintedName(r *Result) (name string, conf float64, ok bool) {
	vizNameStore.mu.Lock()
	defer vizNameStore.mu.Unlock()
	if vizNameStore.owner != r || vizNameStore.name == "" {
		return "", 0, false
	}
	return vizNameStore.name, vizNameStore.conf, true
}

// vizTakeName toglie la riga del nome da fields (dove il voto l'ha scritta) e la registra per r.
func vizTakeName(r *Result, fields map[string]string) {
	name := fields["nome"]
	conf, _ := strconv.ParseFloat(fields["_conf_nome"], 64)
	delete(fields, "nome")
	delete(fields, "_conf_nome")
	vizSetPrintedName(r, name, conf)
}

// vizStripInternal toglie da fields le chiavi di lavoro (prefisso "_conf_") scritte dal voto.
func vizStripInternal(fields map[string]string) {
	for k := range fields {
		if strings.HasPrefix(k, "_") {
			delete(fields, k)
		}
	}
}
