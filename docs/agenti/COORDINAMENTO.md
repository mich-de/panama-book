# Bacheca di coordinamento tra gli agenti

Tenuta dall'integratore (`libretto-exe`). Qui si trovano: chi possiede quali file, le firme che collegano i moduli (contratti),
lo stato di ogni agente, le regole per integrare i candidati in `libretto.exe` e le richieste aperte tra agenti.
Ultimo aggiornamento: **2026-10-23**. Nessun dato personale in questa pagina: solo esiti numerici.

Come si usa: chi ha bisogno di una modifica in un file altrui NON la fa, scrive una riga in "Richieste aperte" (o manda un messaggio
all'agente) e continua. Chi chiude una richiesta la segna "fatta" con la data.

## 1. Proprietari dei file

| Ruolo | Agente (id) | File suoi |
|---|---|---|
| MRZ (lettura con il motore OCR di Windows) | `libretto-mrz` (`a3ab1a62451c268d9`) | `internal/libreta/mrzband.go` e `mrzband_test.go` (e i test `mrzband_*integration_test.go`), `internal/imgproc/rows.go`, `pagecrop.go`, `deskew.go` (con i loro test), `internal/mrz/**` |
| MRZ a template (lettura senza OCR) | `af33114ed9381b4a6` | `internal/libreta/mrzcells.go`, `mrzcells_glyph.go`, `mrzcells_decode.go` e i loro test (`mrzcells_test.go`, `mrzcells_decode_test.go`, `mrzcells_glyphgen_test.go`, `mrzcells_eval_test.go`: gli ultimi due dietro il build tag `glyphgen`), `internal/libreta/glyphs.bin`, `docs/agenti/12-mrz-template.md` |
| VIZ (campi stampati) | `libretto-viz` (`ae6ef3fae229ffbd6`) | `internal/libreta/viz*.go` e i loro test |
| WIA / TWAIN (scanner) | `libretto-wia` (`afe9b41766b09784a`) | `internal/wia/**`, `internal/twain/**`, `internal/imgproc/autocrop.go`, `trimwhite.go` (con i test); `cmd/twainprobe/**` e `cmd/twainscan/**` (assegnazione proposta: nessuno li aveva in elenco) |
| Documentazione | `libretto-docs` (`a690832bc80224ae9`) | `README.md`, `REGISTRO.md`, `docs/campi.md`, `.gitignore` |
| Verifica dei JSON | `libretto-verifica` (`a031f4576e29aef0d`) | `internal/verify/**`, `cmd/verifyjson/**`, `scans\verita\**` (verita' scritta a occhio, contiene dati personali, ignorata da git), `docs/agenti/14-verifica-json.md` |
| Integratore | `libretto-exe` | `internal/libreta/extract.go`, `internal/libreta/mrzmerge.go` e i test `extract_hook_test.go` e `mrzmerge_test.go`, `cmd/libretto/**` (compreso `pdf.go`), `cmd/regress/**`, `internal/outsum/**`, `internal/pdfout/**` (scritto dal committente), `tools/**`, la build di `libretto.exe`, `twainscan32.exe` e `twainscan64.exe` nella radice, questa bacheca, `docs/agenti/13-libretto-exe.md`, `docs/baseline-regressione.tsv` |
| GUI (Panama Book) | agente GUI | `cmd/panamabook/**` (incluso `fakelibretto/`), `internal/memoria/**`, `docs/panamabook.md`, `docs/agenti/15-gui.md`, la build di `PanamaBook.exe` nella radice (la riga `dati/` in `.gitignore` e' sua per richiesta dell'utente) |
| OCR nativo | `libretto-ocr` | `internal/ocr/**` (backend nativo WinRT, vecchio backend PowerShell `winocr.ps1`, test), `docs/agenti/16-ocr-nativo.md` |
| Senza proprietario | | `internal/imgproc/imgproc.go`, `header.go`, `internal/libreta/regex.go`: chi deve cambiarli lo chiede qui sotto |

Le scansioni in `scans\`, `scans\vecchi test WIA\` e `facsimili\` sono dell'utente: si leggono, non si riscrivono.

## 2. Contratti (firme che non cambiano senza avviso)

**OCR** (`internal/ocr`, agente `libretto-ocr`): `Recognize(ctx, path) (*Result, error)` e `RecognizeMany(ctx, paths) ([]*Result, error)` con `Result{Language, Width, Height, Lines[]{Text, Words[]{Text, X, Y, W, H}}}`
(pixel). **Non cambiano.** Da 2026-10-23 il predefinito e' il backend **nativo** (WinRT chiamato dal processo, senza PowerShell); `LIBRETTO_OCR=powershell` o `native` forza uno dei due,
in automatico se il nativo non parte si ripiega su PowerShell con un avviso su stderr (una volta). Stessi risultati del vecchio backend (testo e riquadri identici sul corpus). Dettagli in `16-ocr-nativo.md`.

**Lettura MRZ con l'OCR** (`internal/libreta/mrzband.go`, agente MRZ):

```go
func readMRZBand(ctx context.Context, img image.Image, debugDir string) (*mrz.Data, []string, int, error)
```

L'immagine e' gia' orientata (testo dritto). Restituisce i dati, le correzioni applicate, un punteggio e un errore solo per guasti
tecnici (OCR non disponibile, contesto annullato). Punteggio: 0 se l'MRZ non e' stato trovato; altrimenti `scoreMRZ` (1, piu' 2 per ogni
check digit valido, piu' 2 se l'emittente e' `PAN`: massimo 13) **piu' 1 se la riga 1 e' arrivata a 44 caratteri esatti senza aggiustamenti**
(`scoreSure` = 14, dal round 4 dell'agente MRZ). La riga 1 non ha check digit: l'esattezza di lunghezza e il prefisso `ISPAN` sono l'unica
verifica che ha. Se non ci sono due righe adatte ritorna 0 senza avviare l'OCR.

**Lettura MRZ a template** (`internal/libreta/mrzcells.go`, agente MRZ a template):

```go
func readMRZCells(img image.Image, debugDir string) (*mrz.Data, []string, int)
```

Stessa immagine gia' orientata e stessa scala di punteggio di `scoreMRZ` (0 = non trovato; massimo 13, senza il punto in piu' della riga 1: una lettura
OCR completa a 14 non viene superata, una parziale si). Nessun errore, nessun OCR, nessun contesto; non deve andare in panic. `debugDir` e' vuoto
oppure una cartella dove scrivere immagini di debug (con dati personali).

**Punto di aggancio** (`internal/libreta/extract.go`, integratore):

```go
var mrzCellsReader func(img image.Image, debugDir string) (*mrz.Data, []string, int) // nil di default
```

Il lettore a template si registra da solo, nel suo file: `func init() { mrzCellsReader = readMRZCells }`. Per ogni rotazione `Extract` chiama prima
`readMRZBand` e, se `mrzCellsReader != nil`, **sempre** anche il lettore a template sulla stessa immagine ruotata (con `debugDir/celle` come cartella di debug;
circa 0,15 s). (1) Se la banda non trova l'MRZ o non tornano tutti i check digit, si tiene la lettura con il punteggio piu' alto (a parita' resta quella dell'OCR).
(2) Se la banda ha tutti i check digit, il lettore fa da **seconda lettura** per le parti che i check digit non coprono: riga 1 (cognome e nomi), nazionalita'
(celle 11-13) e sesso (cella 21) della riga 2 (`internal/libreta/mrzmerge.go`, `reconcileMRZ`). Dove le due letture differiscono: se il lettore a template non segnala
come incerta nessuna delle celle diverse si prende la sua lettura e lo si dice nelle correzioni (`mrz_corrections`); se ne segnala una resta l'OCR e c'e' un avviso
(`MRZ_LINE1_UNCERTAIN`, `MRZ_FIELD_UNCERTAIN`). Le parti coperte dai check digit restano sempre quelle dell'OCR: i check digit non peggiorano mai.
**Contratto delle note del lettore a template:** una nota che segnala una cella (incerta, cambiata dai vincoli, glifo sintetico) comincia con `rigaN pos P:` (N = 1 o 2,
P da 1): `extract.go` le legge cosi' per decidere se fidarsi. Se il lettore va in panic, `Extract` avvisa su stderr e continua. Il progetto compila anche senza `mrzcells.go`.

**VIZ** (`internal/libreta/viz.go`, agente VIZ), chiamate da `Extract` in quest'ordine: `prepareVIZImage(img image.Image) image.Image`,
poi `extractVIZ(res *ocr.Result, out *Result)`, poi `finalizeVIZ(r *Result)`. Chiavi interne dei campi (italiane) in `Result.Fields`;
`cmd/libretto/output.go` le traduce nei nomi inglesi del JSON (`schema_version` 1, vedi `docs/campi.md`).

**Riassunto numerico dell'uscita** (`internal/outsum`, integratore): `outsum.Parse(json) (Summary, error)`, `outsum.FieldKeys` (i 14 campi
stampati, nell'ordine fisso `type country_code nationality place_of_birth date_of_issue date_of_expiry date_of_birth level capacity
seamans_book id_number certificate seafarers_id regional`) e `outsum.CheckKeys` (`doc_number birth expiry optional composite`). Lo usano
`libretto summary` e `cmd/regress`; il verificatore puo' riusarlo invece di ridefinire gli elenchi. **Cambio di nome (2026-10-23, richiesta dell'utente: i nomi dei
campi sono quelli delle etichette stampate):** il campo che si chiamava `endorsement` ora e' `certificate` (etichetta stampata `Nº Título / Certificate`); la chiave interna
italiana `refrendo` del VIZ non cambia, la traduzione e' in `cmd/libretto/output.go`. `outsum.Parse` legge ancora le uscite con il nome vecchio (alias in lettura,
transitorio) cosi' i candidati compilati prima del cambio restano misurabili.

**Verita' per l'harness** (verificatore, integratore): per l'immagine `NOME.png` o `NOME.jpg` il file `scans\verita\NOME.json` con i 14 nomi
inglesi dei campi, al livello superiore oppure dentro `"fields"`; stringa vuota = campo non stampato o illeggibile; chiavi con `_` = metadati.
`regress -truth` legge il file con `verify.LoadTruth` e confronta con `verify.Verify` (che usa `Compare` e `NameMatchesMRZ`; `internal/verify`, del verificatore: non
cambiare queste firme, `Result.Fields`, `Result.Names` ne' i nomi `verify.Status*` e `verify.Name*` senza avviso) e nella tabella scrive solo `giusti n/m` e una mappa di simboli (G, S, M, X, `-`), mai i valori.

## 3. Stato degli agenti

| Agente | Stato (2026-10-23) | Ultimo lavoro consegnato |
|---|---|---|
| MRZ (`a3ab1a62451c268d9`) | **concluso (round 4)** | Candidato definitivo `_out\libretto_mrz.exe` (11:03, 6.920.704 byte, sha256 `0C41FC0D8C70`); `mrz_final.exe` ricompilato alle 11:10 e' identico. Il `libretto.exe` installato alle 11:06 era la build delle 10:52 (6.919.680 byte): da qui il KB di differenza. Valutato dall'integratore con le verita': stesso MRZ della build delle 10:52 (41/63), 0 regressioni contro la baseline delle 10:16 (un falso allarme su `librettook7-grezza` con 12 processi insieme, sparito riprovandola da sola: regola 9). Il suo `mrzband.go` (11:01) e' dentro `libretto.exe`. Dettagli in `01-mrz.md` (round 4) |
| MRZ a template (`af33114ed9381b4a6`) | **concluso** (consegna delle 11:46) | **`_out\libretto_template.exe`** (7.145.984 byte, sha256 `7A13AEC247B7`). Rispetto alla build 11:15 installata: spessore del tratto normalizzato, `glyphs.bin` rigenerata (707 glifi, 98 KB; `W` sintetica), plausibilita' con tetto al costo dei vincoli: un check digit stampato sbagliato ora non da' nessuna lettura (`TestReadMRZCellsNonInventaCheckDigit` passa; costo 44 nat nel caso sintetico, soglia 8; sulle letture giuste 0,0). `gofmt -l internal cmd` senza output, `go vet ./...` e `go test ./...` verdi. Harness `-baseline docs\baseline-regressione.tsv -truth scans\verita` (233 s, un processo alla volta): **0 regressioni, 63 invariate** (MRZ 42/63, campi 546/882, giusti 486/584); contro il candidato solo-OCR `libretto_mrz.exe` (11:03) l'unica differenza e' `vero9` (MRZ no -> 5/5). Leave-one-booklet-out su 54 scansioni di 8 libretti (libreria senza il libretto letto): riga 1 99,66%, riga 2 100%, check 5/5 54/54, 158 ms a immagine (79 ms a riga); `provamrz` e `DEFINITIVO_spero` 44/44 + 44/44 senza OCR; 0 falsi positivi su 162 letture di rotazioni sbagliate. Dettagli e tabella per immagine in `12-mrz-template.md` |
| VIZ (`ae6ef3fae229ffbd6`) | **candidata pronta (round 5)** | **Candidata `_out\libretto_viz.exe` (sha256 `821b992ac03b`, compilata dall'albero principale): harness `-truth` contro la baseline attuale (`docs/baseline-regressione.tsv`, 13:46) = 0 regressioni, 13 miglioramenti, 51 invariate.** Campi giusti 527 -> 540 su 598: cambia solo `capacity` (23 -> 36 su 44); gli altri 13 campi uguali. Le 3 regressioni (`place_of_birth` su `dimensioni_libretto_panama` e `vero`, `regional` su `librettook1`) sono corrette: il consenso per allineamento vale solo per la qualifica, gli altri testi tornano al voto per lunghezza. Calibrazione verticale spenta (misurata: 6 regressioni). I sorgenti nuovi SONO GIA' nell'albero principale (`internal/libreta/viz*.go`, `gofmt`, `go vet ./...` e `go test ./...` verdi); i sorgenti del VIZ congelato delle 12:42 sono in `_outiz_frozen_1242\`. Rapporto in `02-viz.md` (round 5). Da installare a cura dell'integratore |
| WIA / TWAIN (`afe9b41766b09784a`) | inattivo | WIA concluso; TWAIN sondato, scansione con un libretto vero non provata |
| Documentazione (`a690832bc80224ae9`) | inattivo | terza passata (README, `docs/campi.md`, `REGISTRO.md`) |
| Verifica (`a031f4576e29aef0d`) | consegnato | `scans\verita\` (75 file), `internal/verify` (`Equal`, `Compare`, `LoadTruth`, `Norm`, `NameMatchesMRZ`) e `cmd/verifyjson` (`_out\verifyjson.exe`, `-dir` verifica molte immagini senza stampare valori); rapporto in `14-verifica-json.md`. `libretto.exe` delle 10:16: 638/759 campi giusti (84%); build 11:15 (quella installata): 648/759 (85%), MRZ valido su 53 delle 60 immagini con pagina, nomi della riga 1 giusti 51/59. Difetto principale: il cargo con sigla di grado e' giusto 0 volte su 29; la verita' coincide con l'MRZ su tutte le immagini con MRZ letto (nascita, scadenza, numero libretto), quindi quei campi si possono riempire dall'MRZ. **Rinomina `endorsement` -> `certificate` fatta** (verita' di tutti i 75 file, `internal/verify`, `cmd/verifyjson`): il vecchio nome si legge ancora come alias, nell'uscita e nella verita'; il campo c'e' solo su alcuni titolari (visto su 2 di 9, con cargo con sigla), assente = NON_STAMPATO e il formato (9 cifre) e' un avviso solo se presente |
| OCR nativo (`libretto-ocr`) | **candidato pronto** | **`_out\libretto_ocr.exe`** (7.427.584 byte, sha256 `66192ad74871`): `internal/ocr` chiama `Windows.Media.Ocr` direttamente (WinRT a mano con `syscall` e `x/sys/windows`, niente cgo, niente PowerShell), API pubblica invariata; ripiego automatico su PowerShell e `LIBRETTO_OCR=powershell|native` per forzare. Con `-truth` contro la baseline: **0 regressioni**; lo stesso eseguibile con `LIBRETTO_OCR=powershell` contro `native` su 70 immagini: **70 invariate, nessuna differenza in nessuna colonna**; testo e riquadri OCR identici su 381 immagini (2946 parole, scarto 0 px); `libretto extract` con JSON identico su 10 copie. Tempi: una chiamata OCR 946 ms -> 25-39 ms, harness 320 s -> 87 s. `gofmt`, `go vet ./...`, `go test ./...` verdi. `go.mod`: `x/sys` da indiretta a diretta. Da installare a cura dell'integratore (regola 3). Dettagli in `16-ocr-nativo.md` |
| Integratore (`libretto-exe`) | attivo | **`libretto.exe` installato alle 16:35: 7.427.584 byte, sha256 `66192AD74871`** = tutto il gia' installato (VIZ 821B, avviso nome stampato, rename, pdf, ordine nomi) + **OCR nativo senza PowerShell** (`internal/ocr`, agente `libretto-ocr`). Prova `-truth` contro la baseline delle 14:35 con i percorsi rimappati (14 file spostati in `scans\vecchi test WIA\`): 64 invariate, 0 regressioni, 0 miglioramenti, campi giusti 540/598 e nomi 41/41 identici; 6 immagini nuove (3 scansioni `libretto-001..003` nuove e le loro copie, senza MRZ). Durata dell'harness: 90 s contro 249 s. Backup: `_out\libretto_nome.exe` (`F1F2C016E983`) e le precedenti. Sorgenti: `_out\src-66192ad74871\`. Baseline rigenerata (70 immagini); la precedente in `baseline-regressione-PRIMA-OCR.tsv`. `PanamaBook.exe` ricostruito dall'albero in `_out\PanamaBook.exe`: identico alla copia nella radice (`3F4E450D687F`), non era in esecuzione, nessuna copia necessaria. Questa bacheca |

| GUI (Panama Book) | **bozza funzionante** | `PanamaBook.exe` (radice e `_out\`): finestra Edge su server locale con token, scansione/lettura con annulla solo in lettura, correzione dei campi, memoria per libretto (`dati\memoria.json`), verita' in `scanserita\`, esporta PDF (1 MB), prova senza scanner con `-demo`; test verdi. Secondo giro: campi "Da controllare" -> "Controllato" (con memoria della conferma), pulsante "Ricomincia da capo", file rinominati `NOME - COGNOME - NUMERO LIBRETTO` dopo la scansione (usa `internal/filename`, sola lettura). Dettagli in `15-gui.md` e `docs/panamabook.md` |

Problema aperto principale, in gran parte chiuso: le scansioni TWAIN pulite `scans\provamrz.png` e `scans\DEFINITIVO_spero.png` non davano MRZ con `libretto.exe` delle
10:16 perche' il motore OCR di Windows restituisce testo vuoto sui ritagli delle righe; ora hanno MRZ 5/5 (agente MRZ, round 4). Con il `libretto.exe` di adesso, sul corpus
di 63 immagini: MRZ 42/63 (2 sono strisce MRZ senza pagina dati). Delle 21 senza MRZ, 14 non hanno pagina dati secondo la verita' (copertine, pagine interne, strisce) e 7
si': `libreta_2017_pagina-dati_PDF417_oscurata` (dati oscurati), `scana1`, `librettook1-grezza`, `librettook8` e `librettook10` con le loro grezze. Campi stampati giusti:
486/584 (83%). Dove l'MRZ c'e' e' il VIZ a perdere campi: `libretto-vero2-grezza` (6/14) e `certificate` (allora chiamato `endorsement`) sbagliato su `DEFINITIVO_spero` e `vero14`.

## 4. Regole di integrazione

1. Nessuno modifica i file altrui. Se serve una modifica li', si scrive la richiesta qui (sezione 5) o si manda un messaggio all'agente.
2. Ogni agente compila i suoi candidati in `_out\<nome>.exe` (`go build -o _out\mrz_x.exe .\cmd\libretto`), mai su `libretto.exe`,
   `twainscan32.exe` o `twainscan64.exe`.
3. `libretto.exe` lo sostituisce solo l'integratore, e solo dopo: `gofmt -l .` senza output, `go vet ./...`, `go test ./...` e l'harness
   `_out\regress.exe -exe _out\<candidato>.exe -baseline docs\baseline-regressione.tsv -truth scans\verita` con uscita 0 (nessuna REGRESSIONE, ne' sui campi
   letti ne' su quelli giusti). Si installa l'esatto eseguibile provato (stesso sha256), mai una ricompilazione fatta dopo la prova: se nel frattempo l'albero
   non e' piu' verde per il lavoro in corso di un altro agente, si installa la build provata e si segnala. Con MIGLIORAMENTI l'integratore rigenera la baseline
   (`-o docs\baseline-regressione.tsv -force`, tenendo la vecchia in `docs\baseline-regressione-<ora>.tsv`) e lo scrive in `13-libretto-exe.md`.
4. I moduli si collegano a `extract.go` con punti di aggancio (variabili funzione registrate da `init()` nel file dell'agente), non
   modificando `extract.go`. Un nuovo aggancio lo chiede l'agente nella sezione 5 e lo scrive l'integratore.
5. Mai lanciare lo scanner: niente `scan`, `read`, `tools\prova*.cmd`, `tools\scansiona.cmd`, `twainscan*.exe`. Ammessi `info`, `extract`,
   `summary`, `twainprobe*` e l'harness.
6. Nessun dato personale in file, log e rapporti: solo esiti ("MRZ 5/5", "14/14 campi"). Fa eccezione `scans\verita\`, locale e ignorata da git.
7. Non si cancella nulla: i resti vanno in `_da_eliminare\` con una nota; li elimina l'utente.
8. Il test di un agente non deve dipendere da file in `scans\`: o salta se manca il file, o sta dietro il build tag `integration`.
9. Una sola prova dell'harness alla volta, con `-j 3`. Con molti processi insieme (12 misurati) le letture marginali di `mrzband.go` perdono check digit per il
   tetto di tempo (`maxBandTime`, 8 s per banda): `librettook7` e la sua grezza sono passate da 5/5 a 3/5 anche con un eseguibile che da solo le legge bene, e
   l'harness ha segnalato regressioni che non c'erano. Una regressione sospetta si riprova da sola, a basso carico, prima di crederci.

## 5. Richieste aperte

| Da | A | Richiesta | Stato |
|---|---|---|---|
| integratore | MRZ a template | registra il lettore con `func init() { mrzCellsReader = readMRZCells }` nel tuo `mrzcells.go` | fatta (visto alle 11:15): da allora e' compilato in `libretto.exe` |
| integratore | MRZ a template | `gofmt -w` sui tuoi file prima di consegnare | fatta (11:46): `gofmt -l internal cmd` senza output; il file era CRLF dopo una scrittura da script, ora i miei file sono LF |
| integratore | MRZ a template | il tuo lettore e' in `libretto.exe` (build 11:15). Prima di dichiarare un candidato lancia `_out\regress.exe -exe _out\tuo.exe -baseline docs\baseline-regressione.tsv -truth scans\verita` e scrivimi il percorso. Il test `TestReadMRZCellsNonInventaCheckDigit`, che oggi fallisce, descrive un rischio reale: con un check digit stampato sbagliato il lettore non deve dare 5/5 | fatta (11:46): candidato `_out\libretto_template.exe` (sha256 `7A13AEC247B7`), harness con `-truth` 0 regressioni; il test passa: il lettore scarta la lettura se i vincoli costano piu' di 8 nat sulla riga 2 (la probabilita' relativa alle sole stringhe valide non basta: vale ~1 anche con un check forzato). Installato dall'integratore alle 11:54: stesso sha256 `7A13AEC247B7`; provato in modo indipendente con l'harness a 3 processi (0 regressioni contro la baseline delle 10:16 e contro la build 11:15; tempo uguale a quella a parita' di carico) e con gofmt, vet e test verdi alle 11:54 |
| MRZ a template | integratore | **proposta** (riga 1): con l'OCR a 5/5 il lettore a celle non viene chiamato, ma 4 immagini su 54 del suo corpus avevano la riga 1 sbagliata nell'etichetta OCR nonostante i 5/5 check (e 1 la nazionalita', che i check non coprono): chiamarlo sempre e, se le due righe 1 differiscono e la sua lettura non ha note di incertezza, preferirla o avvisare | **fatta (12:13)**: `mrzmerge.go` (`reconcileMRZ`). Il lettore a celle gira sempre; dove OCR e lettore differiscono in riga 1, nazionalita' o sesso e nessuna delle celle diverse e' segnalata nelle note, si prende la sua lettura (nota in `mrz_corrections`); se una e' segnalata resta l'OCR e c'e' l'avviso `MRZ_LINE1_UNCERTAIN` o `MRZ_FIELD_UNCERTAIN`; i check digit non peggiorano. Misura con le verita' (harness `-truth`, colonna `nomi`): nomi dell'MRZ giusti **39/40 -> 40/40** (`libretto-vero5`), nazionalita' sostituita su `libretto-vero5-grezza`, nessun avviso sulle altre 40, 0 regressioni, +0,2 s a immagine |
| integratore | verificatore | scrivere la verita' in `scans\verita\NOME.json` ed esporre un confronto di valori in `internal/verify` | fatta (11:22): l'harness usa `verify.LoadTruth` e `verify.Compare`; la copia provvisoria di `normValue` e' stata tolta |
| integratore | verificatore | esporre in `verify.Truth` il campo `_riga_nome` (`NameRow`) e una funzione che confronti cognome e nomi dell'MRZ con la riga del nome (parole senza ordine, accenti e punteggiatura ignorati, un prefisso conta come uguale perche' l'MRZ tronca a 39 caratteri): cosi' l'harness conta anche i **nomi giusti** della riga 1, che non ha check digit. Misura a mano dell'integratore: su 40 immagini con MRZ e nome in verita', 39 uguali e 1 diversa (`libretto-vero5`, la versione ritagliata; la grezza e' giusta) | fatta (2026-10-23): `Truth.NameRow`, `verify.NameMatchesMRZ(surname, givenNames, nameRow)`, `Result.Names` e `verify.NameStatus`. Sulla build 11:15: 51 giusti, 2 sbagliati (`libretto-vero5` e `libretto-vero5-pagina`), 6 senza MRZ su 59 immagini con la riga in verita'. Usata dall'integratore dalle 12:05: colonna `nomi` dell'harness (`verify.Verify`: G, S, M), con regressione se i nomi passano da giusti a sbagliati |
| verificatore | VIZ | **cargo con sigla di grado** (numero romano + barra + cifra): giusto 0 volte su 29 (senza sigla 23 su 27); esce un prefisso di 3 lettere o un testo breve, e con 4 parole le lettere si raddoppiano. Poi luogo di nascita (colonna di filigrana accanto al campo) e identificazione (cifra 9 letta come `S` o `5`). Dettagli e priorita' in `14-verifica-json.md` sezione 4; per provare: `_out\verifyjson.exe -dir CARTELLA_JSON -truth scans\verita -exclude "(-grezza\|-pagina[0-9]*)$"` | aperta |
| verificatore | integratore | in `extract.go`: con l'MRZ valido riempire dall'MRZ i campi stampati che mancano o sono sbagliati (`seamans_book`, `date_of_birth`, `date_of_expiry`, `nationality` con la tabella ISO; `type` e `country_code` dalla riga 1). La verita' coincide con l'MRZ su tutte le immagini con MRZ letto (45 con la build 10:16, 55 con la 11:15). Sulla build 11:15: 11 campi recuperabili subito. In piu': avviso quando la validita' emissione-scadenza supera 5 anni e un mese (coglie le 2 date di emissione sbagliate) | aperta |
| verificatore | MRZ | la riga 1 puo' uscire storpiata con i 5 check digit della riga 2 validi (`libretto-vero5` e `-pagina`, build 11:15): confrontare con `verify.NameMatchesMRZ`. E la robustezza all'inquadratura: `libretto-vero2-grezza` (MRZ valido ma 3 campi su 13) e `librettook1-grezza` (senza MRZ) con gli stessi pixel della versione ritagliata che funziona (`14-verifica-json.md` 3.4 e 3.5) | aperta |
| verificatore | integratore | le verita' in `scans\verita\` sono state riscritte per la rinomina `endorsement` -> `certificate` (stessi valori: 75 file, solo il nome del campo e le note): l'impronta della cartella cambia, rigenerare la baseline con `-truth`. `verify.LoadTruth`/`verify.Compare` leggono anche il vecchio nome; `verify.Canonical(field)` dice il nome attuale. `_out\verifyjson.exe` ricostruito | fatta dall'integratore (12:55): baseline rigenerata con le verita' nuove (impronta `de1c39274ff2`, 75 file); risultati invariati (64 invariate, 0 regressioni, campi giusti 514/584). Le baseline vecchie in `docs/` restano con l'impronta di prima |
| verificatore | documentazione | aggiungere a `REGISTRO.md` e all'indice di `docs/agenti/README.md`: `internal/verify`, `cmd/verifyjson` (`_out\verifyjson.exe`), `scans\verita\` (dati personali, ignorata da git), `docs/agenti/14-verifica-json.md` | aperta |
| integratore | documentazione | aggiungere a `REGISTRO.md`: `cmd/regress`, `internal/outsum`, il punto di aggancio `mrzCellsReader`, `docs/baseline-regressione.tsv` | fatta dall'integratore il 2026-10-23 (solo queste voci e `cmd/libretto/summary.go`; il resto del registro e il conteggio dei test restano della passata del 2026-10-22 sera: tocca alla documentazione aggiornarli) |
| integratore | VIZ | scrivere il rapporto del round 4 in `02-viz.md` (richiesta gia' nel README) | aperta |
| MRZ | integratore | candidato definitivo `_out\libretto_mrz.exe` (11:03): riprovarlo, sostituirlo a `libretto.exe`, aggiornare il contratto (punteggio di `readMRZBand` fino a 14) | fatta (11:35): contratto aggiornato nella sezione 2; il suo codice e' dentro il `libretto.exe` installato. L'errore di compilazione di `mrzcells_glyph.go:203` che segnalavi alle 11:05 non c'e' piu' |
| MRZ | integratore | in `extract.go`: confrontare cognome e nomi dell'MRZ con quelli stampati letti dal VIZ e avvisare se sono molto diversi | valutata: il VIZ non legge i nomi, quindi richiesta al VIZ (riga sotto). Misura sulle verita': riga 1 giusta su 39 immagini su 40 verificabili, e nessun check digit puo' accorgersi dell'errore |
| integratore | VIZ | **leggere cognome e nomi stampati** (etichette `Apellidos / Surname` e `Nombres / Given Names`: oggi sono in `vizLabels` ma il valore non viene estratto; `vizIsMRZName` serve solo a scartare il testo dei nomi). Contratto: in `extractVIZ` metti in `Result.Fields` le chiavi interne `cognome_stampato` e `nomi_stampati` (maiuscolo, senza simboli spuri; assenti se non letti) e NON toccare `cognome` e `nomi` (li scrive `crossCheck` dall'MRZ). Poi l'integratore, in `crossCheck`, li confronta con l'MRZ (parole senza ordine, senza accenti, un prefisso conta come uguale; misura `vizSim`) e, se molto diversi, aggiunge l'avviso `PRINTED_FIELD_DIFFERS_FROM_MRZ` con `field` `surname` o `given_names` (codice gia' esistente: si estende l'elenco in `cmd/libretto/output.go` e `docs/campi.md`; se si preferisce un codice dedicato, per esempio `MRZ_NAME_DIFFERS_FROM_PRINTED`, cambia solo `toWarning`) e toglie le due chiavi dall'uscita. La soglia si tara sulle verita' (`_riga_nome`) | aperta |
| integratore | VIZ | con l'MRZ trovato, `certificate` (fino al 2026-10-23 `endorsement`) diventa SBAGLIATO su `DEFINITIVO_spero` e `vero14` (era giusto prima, quando l'MRZ non si trovava e il VIZ lavorava in modo pagina): confrontare le due letture | in parte: con la versione del VIZ delle 12:39 (installata) `vero14` ha di nuovo il `certificate` giusto, `DEFINITIVO_spero` lo ha ancora sbagliato |
| utente (via committente) | integratore, VIZ, verificatore | rinominare il campo JSON `endorsement` in `certificate` (etichetta stampata `Nº Título / Certificate`) | **fatta (12:49)** per l'integratore: `output.go`, `outsum` (con alias in lettura del nome vecchio), `regress`, test, `tools/summary.py`, `docs/campi.md`, `README.md`, `REGISTRO.md`; `libretto.exe` e `_out\regress.exe` ricostruiti; baseline rigenerata con le verita' nuove (12:55). Verificatore: verita' e `internal/verify` fatti. VIZ: la chiave interna `refrendo` non cambia; restano da aggiornare i suoi test di laboratorio con la mappa `refrendo -> endorsement` (avvisato) |
| committente | integratore | includere il comando `libretto pdf` nella prossima build di `libretto.exe` | **fatta (13:25)** nella build `9197CA1E52D3`, senza le modifiche del VIZ dopo le 12:42 (decisione: nessuna regressione di campo). `pdf` provato: 924 KB a qualita' 92, rifiuta di sovrascrivere senza `-force`. Il VIZ delle 13:04 resta in `_out\libretto_cert_pdf2.exe` (`5D72DFE0939A`): +9 `capacity`, 3 regressioni; da correggere prima di riproporlo |
| integratore | VIZ | versione dei sorgenti delle 12:57: campi giusti 523/584 (514 nella baseline: +9, soprattutto `capacity`), ma 4 regressioni per campo con `-truth`: `place_of_birth` su `dimensioni_libretto_panama` e `vero`, `regional` su `libretto-vero7` e `librettook1`; avvisi in aumento su molte immagini. Rifatta la prova sulla versione delle 13:04: stessi 523/584 e **3 regressioni** (`place_of_birth` su `dimensioni_libretto_panama` e `vero`, `regional` su `librettook1`; `libretto-vero7` e' rientrata). Se e' un compromesso voluto (`capacity` guadagna 9 campi, i testi liberi ne perdono 3) dirlo: decide il committente | **corretta**: candidata `_out\libretto_viz.exe` (`821b992ac03b`), 0 regressioni con `-truth`, +13 miglioramenti (`capacity` 23 -> 36 su 44) |
| integratore | VIZ | `libretto-vero2-grezza`: MRZ 5/5 ma solo 6/14 campi letti. (`scana` e `vero10` sono strisce MRZ senza pagina dati: i loro pochi campi sono ricavati dall'MRZ, la verita' e' vuota) | aperta |
| GUI | integratore | `PanamaBook.exe` lancia `libretto.exe extract -i scans\NOME.png` e legge `fields`/`mrz`/`warnings` (schema_version 1, accetta `endorsement` e `certificate`): non cambiare quelle chiavi senza avviso. Il pulsante PDF cerca `libretto pdf` in `libretto.exe` (guida con `libretto pdf`) e ripiega su `_out\libretto_pdf.exe`: alla prossima build il ripiego non servira' piu' | aperta |
| OCR nativo | integratore | provare `_out\libretto_ocr.exe` (sha256 `66192ad74871`) con l'harness e installarlo se e' tutto verde. Dopo l'installazione l'albero `PanamaBook -> libretto.exe` non avvia piu' PowerShell per l'OCR (era il processo fermato con `0xC0400065`). Per tornare al vecchio percorso senza ricompilare: variabile d'ambiente `LIBRETTO_OCR=powershell` | aperta |
| OCR nativo | MRZ | `mrzband.go` riprova una volta `ocr.RecognizeMany` dopo un fallimento "sporadico di PowerShell (0xc0400065)": con il backend nativo non serve piu' ma e' innocuo (il secondo tentativo parte solo se l'OCR ha dato errore); nessuna modifica richiesta | informazione |
| OCR nativo | documentazione | aggiungere a `REGISTRO.md`: `internal/ocr` (`ocr_native_windows.go`, `ocr_bitmap.go`, `ocr_powershell.go`, variabile `LIBRETTO_OCR`, `Backend()`) e `docs/agenti/16-ocr-nativo.md` | aperta |
| MRZ, MRZ a template | integratore | quando un candidato e' pronto: scrivere il percorso `_out\...exe` qui o con un messaggio; l'integratore lancia l'harness e riporta il confronto | permanente |
| integratore | MRZ | se si riapre il lavoro MRZ: `libretto-vero5.png` (la versione ritagliata) ha la riga 1 diversa dalla riga del nome in verita' (`_riga_nome`), con tutti e 5 i check digit validi; la sua grezza e' giusta. E' l'unico dei 40 casi verificabili | informazione |
| integratore | MRZ | solo informazione: con molto carico (12 processi insieme) `librettook7` e la sua grezza perdono `optional` e `composite` per il tetto `maxBandTime` (8 s); a 3 processi o da sole no. In uso normale non succede; se serve robustezza il tetto si puo' scalare sul tempo reale di una chiamata OCR | informazione |

## 6. Harness di regressione

`_out\regress.exe` (sorgente `cmd/regress`) esegue `libretto extract -i FILE` su un corpus di immagini, 3 processi in parallelo, e stampa una tabella
TSV senza dati personali. Corpus predefinito (63 immagini il 2026-10-23): `scans\*.png` senza le pagine ritagliate `*-pagina*.png`,
`scans\vecchi test WIA\*.png` senza `*-pagina*.png`, `facsimili\panama\*.jpg` e `*.png`.
Colonne: `file`, `rot`, `mrz` (si/no/err), `check` (n/5), `campi` (n/14), `avvisi`, `sec`, `giusti` (n/m con `-truth`), `nomi` (con `-truth`: cognome e nomi della riga 1 dell'MRZ contro la riga del nome in verita', G uguali, S diversi, M MRZ non trovato, `-` niente da confrontare), poi tre mappe di simboli
(`mappa_check`, `mappa_campi`, `mappa_giusti`) che dicono QUALI elementi sono validi, usate dal confronto.

```
_out\regress.exe -exe _out\candidato.exe -baseline docs\baseline-regressione.tsv     confronta un candidato (uscita 1 se c'e' una REGRESSIONE)
_out\regress.exe -exe libretto.exe -o docs\baseline-regressione.tsv -force           rigenera la baseline
_out\regress.exe -exe _out\candidato.exe scans\scana2.png scans\provamrz.png         solo alcune immagini
_out\regress.exe -list -only vero                                                    elenca il corpus senza eseguire nulla
_out\regress.exe -exe _out\candidato.exe -truth scans\verita -baseline ...           conta anche i campi giusti
_out\regress.exe -baseline PRIMA.tsv -against DOPO.tsv                               confronta due tabelle gia' prodotte, senza eseguire nulla
```

I `giusti` si confrontano solo tra tabelle prodotte con la stessa verita': se il verificatore cambia `scans\verita\`, la baseline con `-truth` va rigenerata.

Il confronto e' per nome: perdere un check digit, un campo letto o un campo che era giusto e' una REGRESSIONE anche se un altro migliora;
l'avanzamento e il confronto vanno su stderr, la tabella su stdout (o in `-o`). L'harness non modifica le immagini (controlla dimensione e data
prima e dopo) e prova una copia temporanea dell'eseguibile, cosi' non blocca il file che un altro agente sta ricompilando. Un'esecuzione intera
richiede circa 4-6 minuti se e' l'unica prova in corso (regola 9).

## 7. Storico

| Data | Cosa |
|---|---|
| 2026-10-23 | Creata la bacheca. Harness `cmd/regress`, pacchetto `internal/outsum` (ora usato anche da `libretto summary`), punto di aggancio `mrzCellsReader` in `extract.go`, baseline di `libretto.exe` (10:16): MRZ 33/63, check 5/5 32/63, campi 533/882. |
| 2026-10-23 | Prima valutazione: `_out\mrz_final.exe` (MRZ, 10:52) contro la baseline: 0 regressioni, 9 miglioramenti (MRZ 41/63). `libretto.exe` non sostituito. |
| 2026-10-23 | Seconda valutazione (11:15-11:32, con le verita'): `_out\libretto_mrz.exe` (MRZ, 11:03) con gli stessi risultati della build delle 10:52; build dai sorgenti (MRZ + lettore a template) delle 11:15: MRZ 42/63, check 5/5 42/63, campi 546/882, giusti 486/584, 0 regressioni contro la baseline delle 10:16. Con 12 processi insieme falsi allarmi su `librettook7` (regola 9). |
| 2026-10-23 | **`libretto.exe` sostituito** (11:54) con la build dai sorgenti delle 11:46, quella con il lettore a template corretto (sha256 `7A13AEC247B7`, uguale a `_out\libretto_template.exe`): MRZ 42/63, check 5/5 42/63, campi 546/882, giusti 486/584, 0 regressioni; prima, alle 11:35, la build delle 11:15 (`EF97DBB5294E`). Richieste nuove all'integratore, non iniziate: riempire i campi stampati dall'MRZ valido (verificatore) e usare il lettore a celle come seconda lettura della riga 1 (template). Baseline rigenerata con le verita' (`docs/baseline-regressione.tsv`; la vecchia in `baseline-regressione-1016.tsv`). |
| 2026-10-23 | **`libretto.exe` sostituito** (12:13) con la build dai sorgenti delle 12:05 (sha256 `B13F115F66A1`): lettore a template **sempre chiamato** e confronto con l'OCR per riga 1, nazionalita' e sesso (`mrzmerge.go`, proposta dell'agente template). Con le verita': nomi dell'MRZ giusti 39/40 -> 40/40, MRZ 42/63, check 5/5 42/63, campi 546/882, giusti 486/584, 0 regressioni; sostituzioni su 2 delle 42 immagini con MRZ, nessun avviso nuovo. Baseline rigenerata con la colonna `nomi`. |
| 2026-10-23 | **Il campo `endorsement` diventa `certificate`** (richiesta dell'utente: i nomi sono quelli delle etichette stampate). `libretto.exe` sostituito alle 12:49 (sha256 `BEE3E669E791`, sorgenti delle 12:42: nome nuovo + VIZ delle 12:39): campi giusti 486 -> 514 su 584, 0 regressioni, nomi dell'MRZ 40/40. `outsum.Parse` legge ancora il nome vecchio. Harness: i campi letti dove la verita' e' vuota non contano piu' come regressioni. Baseline rigenerata alle 12:55 con le verita' riscritte (impronta `de1c39274ff2`). Comando `pdf`: candidato `_out\libretto_cert_pdf.exe`, non installato per 4 regressioni per campo del VIZ (12:57). |
| 2026-10-23 | **`libretto.exe` 13:50** (`CBF05BFB7E04`): `libretto rename` (`internal/filename`), ordine dei nomi di Panama corretto nell'uscita JSON, `tools\estrai.cmd` che rinomina. 0 regressioni. Il VIZ delle 13:04 resta candidato (`_out\libretto_cert_pdf2.exe`, 3 regressioni). |
| 2026-10-23 | **`libretto.exe` 14:11 e 14:35**: VIZ corretto (`821B992AC03B`, 0 regressioni, 13 miglioramenti), poi `F1F2C016E983` con l'avviso `MRZ_NAME_DIFFERS_FROM_PRINTED` (soglia di confidenza 0,9: sul corpus non scatta mai). |
| 2026-10-23 | **OCR nativo** (agente `libretto-ocr`): candidato `_out\libretto_ocr.exe` (`66192ad74871`), 0 regressioni, 70/70 invariate contro lo stesso eseguibile con `LIBRETTO_OCR=powershell`, una chiamata OCR da 946 ms a 25-39 ms, harness da 320 s a 87 s. Non installato: tocca all'integratore. |
| 2026-10-23 | **`libretto.exe` 16:35** (`66192AD74871`): OCR nativo predefinito (`LIBRETTO_OCR=powershell` forza il vecchio, `native` esclude il ripiego). 0 regressioni. Un controllo di sicurezza del PC uccideva i processi PowerShell (0xC0400065) e con loro l'albero di PanamaBook. |
