package twain

import (
	"errors"
	"fmt"
)

// ErrTimeout: l'operazione non e' terminata nel tempo concesso. Un trasferimento TWAIN in corso non si puo'
// annullare in modo pulito: dopo il timeout conviene terminare il processo. Il transfer non viene mai ritentato.
var ErrTimeout = errors.New("twain: tempo scaduto")

// Error e' un errore restituito da DSM/data source, con codice di ritorno (TWRC_*) e di condizione (TWCC_*).
type Error struct {
	Op string // operazione, in italiano
	RC uint16 // TWRC_*
	CC uint16 // TWCC_* (0 se non letto)
}

func (e *Error) Error() string {
	return fmt.Sprintf("twain: %s: %s (RC=%d, CC=%d)", e.Op, describeCC(e.RC, e.CC), e.RC, e.CC)
}

var ccMessages = map[uint16]string{
	0:  "nessuna condizione (successo)",
	1:  "errore generico del driver",
	2:  "memoria insufficiente",
	3:  "data source non trovato: il driver TWAIN dello stesso numero di bit non e' installato",
	4:  "numero massimo di connessioni raggiunto (scanner gia' aperto da un'altra applicazione)",
	5:  "errore di funzionamento del driver o dello scanner",
	6:  "capability non valida",
	9:  "versione del protocollo TWAIN non compatibile",
	10: "valore non valido",
	11: "sequenza di chiamate errata (stato TWAIN non corretto)",
	12: "destinazione non valida",
	13: "capability non supportata dal driver",
	14: "operazione non consentita su questa capability",
	15: "capability richiesta nello stato sbagliato (alcune si possono impostare solo con lo scanner fermo)",
	16: "operazione negata dal driver",
	20: "inceppamento della carta",
	21: "alimentazione multipla (piu' fogli insieme)",
	23: "scanner non in linea (spento o scollegato): controllare alimentazione e cavo USB",
	24: "interblocco: coperchio o sportello dello scanner aperto",
	25: "angolo del documento danneggiato o piegato",
	26: "errore di messa a fuoco",
	27: "documento troppo chiaro",
	28: "documento troppo scuro",
	29: "nessun documento nell'alimentatore (carta assente)",
}

var rcMessages = map[uint16]string{
	rcFailure:        "operazione fallita",
	rcCancel:         "operazione annullata",
	rcInfoNotSupport: "informazione non supportata",
	rcDataNotAvail:   "dati non disponibili",
	rcBusy:           "scanner occupato: riprovare tra qualche secondo",
	rcScannerLocked:  "scanner bloccato da un'altra applicazione",
}

func describeCC(rc, cc uint16) string {
	if m, ok := ccMessages[cc]; ok && cc != 0 {
		return m
	}
	if m, ok := rcMessages[rc]; ok {
		return m
	}
	return "errore TWAIN non classificato"
}

// IsPaperEmpty dice se l'errore indica che manca la carta (come wia.IsPaperEmpty).
func IsPaperEmpty(err error) bool {
	var te *Error
	return errors.As(err, &te) && te.CC == ccNoMedia
}

// IsPaperJam dice se l'errore indica un inceppamento o un'alimentazione multipla (come wia.IsPaperJam).
func IsPaperJam(err error) bool {
	var te *Error
	return errors.As(err, &te) && (te.CC == ccPaperJam || te.CC == ccPaperDoubleFeed)
}
