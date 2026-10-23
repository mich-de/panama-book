# 04 · Agente Documentazione (`libretto-docs`)

**Ruolo:** mantenere README, tabella dei campi, Registro della Codebase e `.gitignore`; proporre la pulizia senza eliminare nulla. Non scrive codice.
**File suoi:** `README.md`, `docs/campi.md`, `REGISTRO.md`, `.gitignore`.

## Passata 1 (stesura iniziale, codice ancora incompleto)
Creati README, `docs/campi.md` (campi, formati, fonte, validazione), `REGISTRO.md` nel formato YAML richiesto dal tuo CLAUDE.md, `.gitignore` con commenti.
**Incoerenze trovate:** `mrzband.go` e `viz.go` erano ancora stub; chiavi JSON miste ES/IT; la skill consigliava il "vassoio piatto" che il fi-800R non ha
(**corretta**); `drivers/LEGGIMI.md` parlava di un ISO da scaricare mentre c'e' l'`.exe` (**corretto**); `panama.png` non nel LEGGIMI dei facsimili; funzioni duplicate (`itoa`, `mini/maxi`, `mrzClean/cleanOCR`).

## Passata 2 (sul codice finale)
Aggiornati README (con la procedura di prova con il foglio stampato e la regola di non usare il caricatore dall'alto per i libretti), campi, registro, `.gitignore`.
**Incoerenze:**
- `docs/campi.md` diceva che cognome, nomi e paese venivano confrontati con la zona visiva: il codice non lo fa (**documento corretto**);
- per il modello 2017 compare sempre l'avviso "campi stampati non letti";
- `crossCheck` scriveva valori personali negli avvisi mentre `finalizeVIZ` dichiara di non farlo (**corretto dal committente**: ora l'avviso e' generico);
- `Options.Scale` e `regexpCache` ancora presenti;
- `viz_exp2.exe` in `_out` non era nell'elenco da pulire (aggiunto).
Verifiche: `go vet` anche con il tag `integration` e `go test ./...` passano. Nessuna scansione, nessun file eliminato, solo dati inventati nei documenti.

## Pulizia proposta (NON eseguita, serve il tuo ok)
| Gruppo | Dimensione | Note |
|---|---|---|
| `drivers/` (`_work`, `_extract`, installer `.exe`, log di installazione) | circa 1,1 GB | l'installer e l'estratto servono solo per reinstallare |
| `_out/` (prove, eseguibili di sviluppo, debug) | circa 132 MB | **contiene immagini con dati personali** |
| `libretto.exe` | 6 MB | si ricrea con `go build` |
| `_old/` | pochi KB | due pezzi di codice accantonati |
| dead code | poche righe | `Options.Scale`, `regexpCache`, funzioni duplicate |

Da aggiungere dopo i rapporti successivi (agente MRZ e revisioni): `internal/libreta/zz_time_test.go`, `internal/libreta/zz_test.go`, `_out/exp*`, `_out/dbg_*`,
`scans/dbg*`, `_viz_exp*`, `scans/prova1.json` e `scans/prova1.err` (non coperti da `.gitignore`).

**Privacy:** `facsimili/panama/panama.png` e le pagine dati 2020 e 2026 mostrano nomi e foto non oscurati: tenerle solo su questo PC.

## Passata 3 (2026-10-22, sera: JSON inglese, TWAIN, nuove opzioni)
Aggiornati README (opzioni `-driver`, `-from`, `-len`, `-width`, `-raw`, `-page-only`; sezione "Driver: WIA e TWAIN"; script `tools\prova-*.cmd` e istruzione di inserimento del libretto; JSON inglese con `schema_version` e avvisi a codice; nuovi limiti), `docs/campi.md` (chiavi inglesi, tabella dei codici degli avvisi, mappa delle vecchie chiavi, tipo ricavato dall'MRZ, tabella delle nazionalita'), `REGISTRO.md` (moduli TWAIN, `output.go`, `deskew.go`, `viz_*`, conteggio test: 176 passati e 1 saltato, 10 di integrazione), `.gitignore` (solo commenti) e `docs/agenti/README.md` (indice, stato, decisioni aperte, storico).
**Incoerenze trovate (non corrette, il codice non e' mio):**
- `tools/prova-pagina.cmd` dice di inserire un libretto vero e legge 110 mm, ma la pagina dati sta tra 90 e 180 mm;
- `usage()` di `main.go` non elenca tutte le opzioni nelle righe di sintassi; `scan` accetta `-page-only` e `-page-out` e li ignora;
- il testo "data stampata non interpretabile come data" finisce in `UNKNOWN_WARNING` (manca un codice);
- `libretto` passa sempre `-ignora-sensore` a `twainscan32.exe`;
- i test di integrazione di `imgproc` e `mrzband` cercano le scansioni in `scans\`, non in `scans\vecchi test WIA\`;
- `docs/agenti/02-viz.md` non ha il round 4; `_da_eliminare/ELENCO.md` non elenca `round4\` e `round5\`;
- il README diceva che nessun libretto vero era stato provato, ma `03-wia.md` riporta misure su un libretto vero (README corretto);
- `go vet -tags integration` fallisce per `internal/libreta/viz_tmp_integration_test.go` (file di lavoro dell'agente VIZ).
Nessun file eliminato o spostato; nessuno `scan`, `read` o `tools\prova*.cmd` eseguito (solo `go vet` e `go test`, che non toccano lo scanner).
