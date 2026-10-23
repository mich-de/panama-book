package wia

import (
	"errors"
	"fmt"

	ole "github.com/go-ole/go-ole"
)

// Codici HRESULT usati nella mappatura (facility WIA 0x0021).
const (
	hrDispEException = 0x80020009 // DISP_E_EXCEPTION: il codice vero e' in EXCEPINFO.scode
	hrWIAGeneral     = 0x80210001
	hrWIAPaperJam    = 0x80210002
	hrWIAPaperEmpty  = 0x80210003
	hrWIABusy        = 0x80210006
	hrWIAWarmingUp   = 0x80210007
)

// hresultMessages traduce in italiano i codici WIA piu' comuni (wiadef.h / winerror.h).
// I codici 0x80210001..0x8021000F sono quelli storici di WIA; 0x15..0x1A provengono
// dalle intestazioni piu' recenti e non sono stati verificati su questo driver.
var hresultMessages = map[uint32]string{
	0x80210001: "errore generale del dispositivo",
	0x80210002: "inceppamento della carta",
	0x80210003: "nessun documento nell'alimentatore (carta assente)",
	0x80210004: "problema con la carta (alimentazione errata)",
	0x80210005: "scanner non in linea (spento o scollegato)",
	0x80210006: "scanner occupato: riprovare tra qualche secondo",
	0x80210007: "scanner in riscaldamento: attendere",
	0x80210008: "serve un intervento dell'utente sullo scanner (carta, coperchio o pannello)",
	0x80210009: "elemento di scansione non piu' disponibile",
	0x8021000A: "errore di comunicazione con lo scanner (controllare il cavo USB)",
	0x8021000B: "comando non valido per questo dispositivo",
	0x8021000C: "impostazione hardware non corretta",
	0x8021000D: "scanner bloccato da un'altra applicazione",
	0x8021000E: "eccezione nel driver dello scanner",
	0x8021000F: "risposta non valida dal driver dello scanner",
	0x80210015: "nessun dispositivo di acquisizione disponibile",
	0x80210016: "coperchio dello scanner aperto",
	0x80210017: "lampada dello scanner spenta",
	0x80210018: "destinazione dell'acquisizione non valida",
	0x8021001A: "alimentazione multipla (piu' fogli insieme)",
	0x80070005: "accesso negato",
	0x80004005: "errore non specificato",
	0x80070015: "dispositivo non pronto",
	0x800706BA: "servizio Acquisizione di immagini di Windows non raggiungibile",
	0x80010106: "COM gia' inizializzato in un'altra modalita'",
}

// hresultOf estrae l'HRESULT piu' significativo da un errore go-ole.
// Per DISP_E_EXCEPTION (tipico dei metodi WIA) il codice WIA sta in EXCEPINFO.scode.
func hresultOf(err error) (code uint32, ok bool) {
	var oe *ole.OleError
	if !errors.As(err, &oe) || oe == nil {
		return 0, false
	}
	code = uint32(oe.Code())
	if code == hrDispEException {
		if ex, isEx := oe.SubError().(ole.EXCEPINFO); isEx {
			if sc := ex.SCODE(); sc != 0 {
				return sc, true
			}
			if wc := ex.WCode(); wc != 0 {
				return uint32(wc), true
			}
		}
	}
	return code, true
}

// describeHRESULT restituisce il messaggio italiano per un HRESULT.
func describeHRESULT(code uint32) string {
	if m, ok := hresultMessages[code]; ok {
		return m
	}
	if code>>16 == 0x8021 {
		return "errore WIA non classificato"
	}
	return "errore COM"
}

// explain avvolge un errore COM aggiungendo il contesto e il messaggio italiano del codice WIA.
// L'errore originale resta raggiungibile con errors.Is/As.
func explain(op string, err error) error {
	if err == nil {
		return nil
	}
	if code, ok := hresultOf(err); ok {
		return fmt.Errorf("%s: %s (HRESULT 0x%08X): %w", op, describeHRESULT(code), code, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// isRetryable dice se l'errore indica una condizione transitoria (occupato/riscaldamento).
func isRetryable(err error) bool {
	code, ok := hresultOf(err)
	return ok && (code == hrWIABusy || code == hrWIAWarmingUp)
}

// IsPaperEmpty segnala se l'errore (restituito da Scan) vuol dire "nessun documento nell'alimentatore".
func IsPaperEmpty(err error) bool {
	code, ok := hresultOf(err)
	return ok && code == hrWIAPaperEmpty
}

// IsPaperJam segnala un inceppamento.
func IsPaperJam(err error) bool {
	code, ok := hresultOf(err)
	return ok && code == hrWIAPaperJam
}
