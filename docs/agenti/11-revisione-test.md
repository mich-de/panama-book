# 11 · Revisione della strategia di test (`architect-test-strategist`)

**Lente:** lacune di copertura che contano e fragilita'. Sola lettura (ha eseguito `go test -cover`).

## Copertura reale (`go test -cover ./internal/...`)
| Pacchetto | Copertura |
|---|---|
| `mrz` | 89,2% |
| `libreta` | 66,9% |
| `imgproc` | 46,7% |
| `wia` | 29,4% |
| `ocr` | 0% (nessun test) |

Nessuna soglia configurata. Nessun test unitario copre `Extract` e `cmd/libretto/main.go`.

## Lacune alte
- **`extract.go` e `ocr.go`**: l'OCR e' una funzione di pacchetto (`ocr.Recognize`, `ocr.RecognizeMany`, chiamate da `mrzband.go`, `viz.go`, `extract.go`) senza interfaccia sostituibile. Non sono provati: orientamento 0/180/90/270, OCR vuoto, JSON non valido, timeout di 90 s, `crossCheck`.
- **`wia.go` (`scanItem`, `explain`)**: lo scanner e' solo COM, senza interfaccia. Il test copre la mappatura degli HRESULT ma non il flusso (simplex, `Pages`, `Transfer`, BMP/PNG). L'errore "carta inceppata" (`IsPaperJam`) non e' mai gestito nella CLI, solo `IsPaperEmpty` ha un suggerimento.
- **`mrz/fix.go`**: manca il caso in cui la correzione non e' unica (>=2 candidati, quindi nessuna correzione) e `Parse` con il secolo della data di nascita; `date()` usa `time.Now()`, quindi l'esito cambia con la data.

## Lacune medie
- Privacy e riproducibilita': le integrazioni usano `scans/prova1.png` e `facsimili/` (dati reali), esclusi dal `.gitignore` ma il risultato dipende da file che altri non hanno, e i test saltano (`Skip`) in silenzio. Soglie fisse (>=12 campi su 14, durata sotto 6 s: dipende dalla macchina).
- `viz_test.go` contiene numeri di libretto che sembrano presi da documenti reali: sostituirli con valori inventati.
- I formati dei numeri 2017/2020/2026 sono provati solo in `vizLibretto`, senza un controllo esplicito per modello.
- `internal/libreta/zz_test.go`: esplorativo, senza asserzioni, scrive in `_out/` e usa dati reali: spostare o eliminare.

## Bassa
Mancano file di riferimento ("golden") per il JSON di `Result`.

## Test da aggiungere, in ordine di valore
1. `internal/mrz/fix_test.go`: correzione ambigua non applicata; riga 2 con lunghezza errata non toccata; campo opzionale con `<` o `0`; anno di nascita con una funzione di data iniettabile.
2. Interfaccia `Recognizer` in `internal/ocr` e un OCR finto in `internal/libreta/extract_test.go`: risposte per le 4 rotazioni, OCR vuoto, errore.
3. Interfaccia `Transferer` in `internal/wia/scan_test.go`, con uno scanner finto (HRESULT occupato, carta assente, inceppamento, BMP); verifica anche la rimozione dei file temporanei.
4. `cmd/libretto/main_test.go` su `scanAndPost` con lo scanner finto: scansione vuota e messaggio per carta inceppata.
5. Generatore di immagini sintetiche (MRZ e campi stampati, rumore a seme fisso) in `internal/testdata`, al posto di `facsimili/` e `scans/` nei test.
6. File golden `testdata/*.golden.json` per `Result`, aggiornabili con il flag `-update`.

## Piramide consigliata
Unitari e finti (1-4) a ogni modifica; integrazione con OCR reale solo con il tag `integration`, non bloccante e con soglie relative; prova manuale con scanner e carta con una checklist in `docs/`.
