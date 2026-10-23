# 13 · Agente integratore (`libretto-exe`)

**Ruolo:** collegare i moduli degli altri agenti (MRZ, MRZ a template, VIZ, WIA/TWAIN, verifica) in `libretto.exe`, tenere l'harness di regressione e la bacheca [COORDINAMENTO.md](COORDINAMENTO.md), ricostruire `libretto.exe` solo se non ci sono regressioni.
**File suoi:** `internal/libreta/extract.go` e `extract_hook_test.go`, `cmd/libretto/**`, `cmd/regress/**`, `internal/outsum/**`, `tools/**`, la build di `libretto.exe`, `twainscan32.exe` e `twainscan64.exe`, `COORDINAMENTO.md`, questo file, `docs/baseline-regressione.tsv`.
**Regole:** mai lo scanner (`scan`, `read`, `tools\prova*.cmd`, `tools\scansiona.cmd`, `twainscan*.exe`); nessun dato personale (solo esiti numerici); niente cancellazioni; non si sovrascrive `libretto.exe` senza `gofmt`, `go vet`, `go test` e harness senza regressioni.

## Round 1 (2026-10-23) — harness, aggancio del lettore MRZ a template, bacheca

### Harness di regressione `cmd/regress` (`_out\regress.exe`)
Esegue `libretto extract -i FILE` (solo `extract`) su un corpus di immagini, 3 processi in parallelo, legge il JSON in uscita e stampa una tabella TSV senza dati personali.
Corpus predefinito (63 immagini): `scans\*.png` (13), `scans\vecchi test WIA\*.png` (40), `facsimili\panama\*.jpg` e `*.png` (10), senza le pagine ritagliate `*-pagina*.png` e senza sottocartelle (debug incluse).
Colonne: `file`, `rot`, `mrz` (si/no/err), `check` (n/5), `campi` (n/14), `avvisi`, `sec`, `giusti` (n/m, solo con `-truth`) e tre mappe di simboli (`mappa_check`, `mappa_campi`, `mappa_giusti`) che dicono QUALI elementi sono validi.

Scelte di progetto:
- **Confronto per nome.** Con `-baseline FILE` ogni dimensione (MRZ, check digit, campi letti, campi giusti) e' confrontata elemento per elemento: perdere anche un solo elemento e' una REGRESSIONE (uscita 1), anche se un altro intanto migliora (si vedono entrambe le righe); guadagnarne e' un MIGLIORAMENTO. Rotazione, numero di avvisi e tempo (oltre 1,5 volte e 4 s in piu') sono solo NOTE. Una prova che prima riusciva e ora fallisce e' una regressione.
- **Sola lettura.** L'harness non modifica le immagini: registra dimensione e data di ognuna prima e dopo la prova e, se cambiano, esce con errore. Prova una **copia temporanea** dell'eseguibile (nome e data uguali all'originale, impronta SHA-256 in intestazione), cosi' non blocca il file che un altro agente sta ricompilando e la prova non cambia a meta' se l'eseguibile viene sostituito; la copia e' eliminata a fine prova.
- **Tabella su stdout (o `-o FILE`, che non sovrascrive senza `-force`); avanzamento e confronto su stderr.** Intestazione di commenti con eseguibile, data, totali e legenda delle mappe; una baseline e' semplicemente una tabella salvata.
- **Opzioni:** `-exe`, `-baseline`, `-o`, `-force`, `-truth CARTELLA`, `-root`, `-j` (3), `-timeout` (4 min per immagine), `-only` e `-skip` (parole nel percorso), `-list` (elenca il corpus senza eseguire), `-q`, e un elenco di immagini a mano al posto del corpus.
- **`-truth`.** Per `NOME.png` legge `CARTELLA\NOME.json` (i 14 nomi inglesi al livello superiore o in `fields`) e per campo distingue G giusto, S sbagliato, M mancante, X letto ma vuoto in verita'; confronta in memoria con normalizzazione (maiuscole, spazi, trattini, punteggiatura, accenti, date anche ISO) e nella tabella scrive solo `giusti n/m` e i simboli. Un campo che passa da giusto a sbagliato o mancante e' una REGRESSIONE. Nel round 1 provato con verita' sintetiche; dal round 2 legge il file con `verify.LoadTruth` e confronta con `verify.Compare` (`internal/verify`, del verificatore) e la normalizzazione provvisoria e' stata tolta.
- **Pacchetto `internal/outsum`.** Il riassunto numerico dell'uscita JSON (14 campi, 5 check digit, avvisi) viveva in `cmd/libretto/summary.go`, che e' `package main` e non si puo' importare: l'ho spostato in `internal/outsum` e ora lo usano sia `libretto summary` (stessa riga di prima) sia l'harness. Un solo elenco dei 14 campi.
- **29 test** in `cmd/regress` (corpus, tabella, confronto, verita', comando intero con un "libretto finto" che e' il binario di test stesso: nessun OCR) e 4 in `outsum`.

### Aggancio del lettore MRZ a template (`extract.go`)
`var mrzCellsReader func(img image.Image, debugDir string) (*mrz.Data, []string, int)`, nil di default. Per ogni rotazione `Extract` (ora tramite `findOrientation` e `readMRZOriented`) chiama prima `readMRZBand`; se non trova l'MRZ o non tornano tutti i check digit e `mrzCellsReader != nil`, chiama anche il lettore sulla stessa immagine ruotata (cartella di debug `rotN\celle`) e tiene il risultato con il punteggio piu' alto di `scoreMRZ` (a parita' resta l'OCR); non lo chiama se la banda ha gia' 5/5 o se il contesto e' annullato; un suo panic e' intercettato (avviso su stderr). Il lettore si registra con `func init() { mrzCellsReader = readMRZCells }` nel suo `mrzcells.go` (richiesta inviata all'agente). **Verificato che il progetto compila, passa `go vet` e i test anche senza `mrzcells*.go`** (build con `-overlay` che toglie i file, senza toccarli). 11 test in `extract_hook_test.go` con un lettore finto: usato se la banda non trova nulla, non chiamato se la banda e' completa, vince il punteggio piu' alto (parita' = banda), `nil` non cambia nulla, panic non fatale, errore e contesto annullato propagati, cartella di debug separata, rotazione 180 e 270 trovate solo dal lettore.

### Baseline di `libretto.exe` (6.879.744 byte, 2026-10-23 10:16, sha256 `80b3a9604441`)
Esito numerico, 63 immagini, 306 s con 3 processi (i secondi dipendono dal carico: altri agenti lavoravano in parallelo). File: `docs/baseline-regressione.tsv`.

| Gruppo | Immagini | MRZ trovata | Check 5/5 | Campi letti | 14/14 | Tempo medio |
|---|---|---|---|---|---|---|
| `scans\` (radice) | 13 | 6 | 6 | 112/182 | 6 | 16,4 s |
| `scans\vecchi test WIA\` | 40 | 23 | 22 | 355/560 | 10 | 14,4 s |
| `facsimili\panama\` | 10 | 4 | 4 | 66/140 | 2 | 12,3 s |
| **Totale** | **63** | **33** | **32** | **533/882** | **18** | **14,5 s** |

Nessun errore di esecuzione. Casi da tenere d'occhio (solo esiti):
- **7 immagini con i campi stampati letti (11-14 su 14) ma senza MRZ**: `provamrz` (14/14), `DEFINITIVO_spero` (11/14), `vero14` (12/14), `vecchi test WIA\vero6` (12/14), `libretto-vero4-grezza` (12/14), `libretto-vero5` e la sua grezza (11/14). Sono il bersaglio del lettore a template.
- `scans\vero10.png`: MRZ 5/5 ma solo 3/14 campi. `libretto-vero2-grezza.png`: MRZ con 3 check su 5 e 6/14 campi.
- 15 immagini senza MRZ e con 0/14 campi: 13 scansioni (`scana`, `vero8`, `vero9`, `vero11`, `vero5` e le serie `librettook`, `librettook2/6/10` con le loro grezze) e le due copertine dei facsimili, che non hanno pagina dati.

### Prima valutazione di un candidato (`_out\mrz_final.exe`, agente MRZ, 10:52, sha256 `703752da85e6`)
`_out\regress.exe -exe _out\mrz_final.exe -baseline docs\baseline-regressione.tsv`: **0 regressioni, 9 miglioramenti, 54 invariate** (313 s, nessun errore di esecuzione). Tabella e confronto salvati in `_out\valutazione-mrz_final.tsv` e `.log`.

| | `libretto.exe` (baseline) | `mrz_final.exe` |
|---|---|---|
| MRZ trovata | 33/63 | **41/63** |
| Check 5/5 | 32/63 | **41/63** |
| Check digit validi | 163/315 | **205/315** |
| Campi stampati letti | 533/882 | 541/882 |
| Tempo medio per immagine | 14,5 s | 14,8 s |

MRZ trovata per gruppo: `scans\` 6 -> 10 su 13; `scans\vecchi test WIA\` 23 -> 27 su 40; facsimili 4 -> 4 su 10.
- **Miglioramenti:** 8 immagini guadagnano l'MRZ con 5/5 check: `provamrz` e `DEFINITIVO_spero` (le due scansioni TWAIN pulite del problema aperto), `scana`, `vero14`, `vecchi test WIA\vero6`, `libretto-vero4-grezza`, `libretto-vero5` e la sua grezza; `libretto-vero2-grezza` passa da 3/5 a 5/5. Con l'MRZ guadagnano anche il campo `type` (ricavato dal codice documento) su 5 immagini.
- **Restano senza MRZ 22 immagini:** 14 con 0/14 campi (12 scansioni e le 2 copertine dei facsimili) e 8 con pochi campi (4 facsimili 2017/2019, `scana1`, `librettook1-grezza`, `librettook8` e la sua grezza).
- **Con MRZ 5/5 ma pochi campi stampati** (lavoro per l'agente VIZ): `scana` 3/14, `vero10` 3/14, `libretto-vero2-grezza` 6/14.
- `libretto.exe` **non e' stato sostituito** (richiesta di questo giro). Il sorgente attuale passa `gofmt`, `go vet ./...` e `go test ./...`: quando l'agente MRZ conferma di aver finito riprovo il suo ultimo candidato e, se non ci sono regressioni, lo sostituisco e rigenero la baseline.

### Non verificato (al termine del round 1; il round 2 ne chiude alcuni)
- Determinismo: ripetuto `libretto.exe` su 9 immagini del corpus (83 s) con risultato identico alla baseline (0 differenze, una sola nota di tempo); l'intero corpus non e' stato ripetuto due volte. *Round 2: a basso carico il risultato e' stabile, con 12 processi insieme no (vedi sotto).*
- `-truth` solo con verita' sintetiche. *Round 2: fatto con le verita' vere.*
- Il lettore a template reale non era ancora agganciato (il suo file non registrava `mrzCellsReader`). *Round 2: agganciato e provato.*
- Le pagine ritagliate `*-pagina*.png` sono escluse anche da `scans\vecchi test WIA\` (la richiesta diceva `*-pagina.png`): cambia solo `librettook1-pagina2.png`, che e' un ritaglio.

### Da decidere (all'utente)
1. `tools\summary.py` e' un duplicato di `libretto summary` e dell'harness: proposto per `_da_eliminare\` (non spostato).
2. Quando un candidato e' senza regressioni e con miglioramenti: sostituire `libretto.exe` e rigenerare la baseline (lo fa l'integratore).

### File temporanei e resti
- `_out\regress.exe`: eseguibile di lavoro, ricostruibile con `go build -o _out\regress.exe .\cmd\regress` (la cartella `_out\` e' ignorata da git).
- Nessun file eliminato o spostato. Nessun file in `scans\` toccato.
- Fuori dal repository, nella cartella temporanea della sessione: i log di avanzamento delle prove.

## Round 2 (2026-10-23, 11:10-12:15) — candidato MRZ definitivo, build dai sorgenti, seconda lettura della riga 1, `libretto.exe` sostituito tre volte

Richieste del committente: valutare `_out\libretto_mrz.exe` (definitivo dell'agente MRZ) contro la baseline e contro le verita' di `scans\verita`; installarlo o, se l'albero compila, ricostruire `libretto.exe` dai sorgenti; aggiornare il contratto del punteggio; valutare un controllo dei nomi dell'MRZ contro quelli stampati. Seconda richiesta (alle 11:55, a lavoro del template concluso): implementare la proposta dell'agente template, cioe' il lettore a celle sempre acceso come seconda lettura della riga 1 e della nazionalita', misurarla prima e dopo e reinstallare solo senza regressioni.

### Eseguibili in gioco (sha256 verificati)
| Eseguibile | Byte | sha256 | Cos'e' |
|---|---|---|---|
| `_out\libretto_1016_backup.exe` | 6.879.744 | `80B3A9604441` | il `libretto.exe` delle 10:16 (baseline di partenza) |
| `_out\libretto_precedente_1052.exe` | 6.919.680 | `703752DA85E6` | build MRZ delle 10:52, installata dal committente alle 11:06 |
| `_out\libretto_mrz.exe` (11:03) = `_out\mrz_final.exe` (ricompilato alle 11:10) | 6.920.704 | `0C41FC0D8C70` | definitivo dell'agente MRZ; il KB in piu' rispetto alla build delle 10:52 e' l'ultimo giro di `mrzband.go` (riga 1 esatta) |
| `_out\libretto_build.exe` (11:15) | 7.137.280 | `EF97DBB5294E` | build dai sorgenti: MRZ definitivo + lettore a template registrato con `init`. Installata come `libretto.exe` alle 11:35, sostituita alle 11:54 |
| `_out\libretto_build2.exe` (11:46) = `_out\libretto_template.exe` | 7.145.984 | `7A13AEC247B7` | build dai sorgenti con il lettore a template corretto (un check digit stampato sbagliato non da' lettura). Installata alle 11:54, sostituita alle 12:13 |
| `_out\libretto_build3.exe` (12:05) | 7.163.392 | `B13F115F66A1` | build dai sorgenti con la seconda lettura della riga 1 (`mrzmerge.go`). **Installata come `libretto.exe` alle 12:13** |

### Valutazione con le verita' (62 delle 63 immagini hanno una verita'; 584 campi con valore)
Prima quattro prove insieme (12 processi) per confrontare in una volta sola, poi una prova pulita (3 processi) della build dai sorgenti. Tabelle in `_out\valutazione-*.tsv`, confronto offline con `regress -baseline A.tsv -against B.tsv`.

| Eseguibile | MRZ | Check 5/5 | Check digit | Campi letti | Campi giusti |
|---|---|---|---|---|---|
| 10:16 (baseline, prova pulita) | 33/63 | 32/63 | 163/315 | 533/882 | non misurati (le verita' non c'erano) |
| 10:16 (12 processi) | 33/63 | 30/63 * | 159/315 * | 533/882 | 477/584 |
| 10:52, quello installato prima | 41/63 | 41/63 | 205/315 | 541/882 | 484/584 |
| MRZ definitivo 11:03 | 41/63 | 40/63 * | 203/315 * | 541/882 | 484/584 |
| **build dai sorgenti 11:15 (prova pulita, 3 processi)** | **42/63** | **42/63** | **210/315** | **546/882** | **486/584** |

\* sotto carico (12 processi insieme) `librettook7` e la sua grezza perdono `optional` e `composite` (5/5 -> 3/5), anche con l'eseguibile delle 10:16: il tetto di tempo `maxBandTime` (8 s) di `mrzband.go` scatta. Riprovate da sole, tutti e tre gli eseguibili le leggono 5/5 e 14/14 (3 volte l'MRZ definitivo, 2 il 10:52, 2 la build). Per questo la regola 9 della bacheca: una prova alla volta.

Risultati:
- **L'MRZ definitivo equivale alla build delle 10:52** sul corpus (41/63 con MRZ): la differenza e' solo la riga 1 esatta, che porta il punteggio a 14.
- **Il lettore a template, agganciato per la prima volta, recupera `vero9`** (`vecchi test WIA`): MRZ 5/5 dove l'OCR non trovava nulla; nascita, scadenza e numero libretto dell'MRZ coincidono con la verita'. Nessun falso positivo sulle altre 21 immagini senza MRZ. Sotto carico, dove l'OCR perdeva due check su `librettook7-grezza`, la build con il lettore a template la legge 5/5 (probabilmente interviene il lettore).
- **Controllo indipendente dell'MRZ contro la verita'** (nascita, scadenza e numero libretto ricavati dall'MRZ contro quelli scritti a occhio): uguali su tutte le 9 immagini verificabili tra quelle guadagnate dopo le 10:16 o dal lettore a template; `scana` non ha verita' (striscia MRZ). L'agente verificatore ha controllato le 45 immagini con MRZ valido: tutte coincidono.
- **Con le verita' emerge una regressione per campo che il confronto senza verita' non vedeva:** `endorsement` passa da giusto a sbagliato su `DEFINITIVO_spero` e `vero14` (guadagnano pero' altri campi su `DEFINITIVO_spero`: +2 giusti nette). Succede con la build delle 10:52 e con tutte le successive: quando l'MRZ si trova il VIZ passa dal modo pagina al modo foglio e legge il numero diversamente. Richiesta al VIZ nella bacheca.
- **Riga 1 dell'MRZ (nomi), confronto con la riga del nome scritta dal verificatore** (`_riga_nome`, 59 file su 75): su 40 immagini con MRZ e nome in verita', 39 uguali e 1 diversa (`libretto-vero5`, la versione ritagliata; la sua grezza e' giusta). La riga 1 non ha check digit, quindi questo errore passa tutti i controlli.
- Tempo medio per immagine, prova pulita: 10,6 s (la baseline delle 10:16 era 14,5 s con la macchina piu' carica: non confrontabile).

### Installazione di `libretto.exe`
1. Il committente aveva gia' installato la build delle 10:52 (`703752DA85E6`); copiata in `_out\libretto_precedente_1052.exe` (il vecchio delle 10:16 era gia' in `_out\libretto_1016_backup.exe`).
2. Decisione: **installata la build dai sorgenti delle 11:15** (`EF97DBB5294E`), non il solo candidato MRZ, perche' (a) l'albero compilava e la richiesta era di ricostruire dai sorgenti, (b) in tre prove diverse non ha regressioni rispetto alla baseline, alla build delle 10:52 e al definitivo MRZ, (c) recupera un'immagine in piu'. Si installa l'esatto eseguibile provato (copia, sha256 controllato prima e dopo), non una ricompilazione.
3. Controlli sull'albero: alle 11:15 `gofmt -l`, `go vet ./...` e `go test ./...` erano puliti. **Alle 11:31 l'albero non e' piu' verde per il lavoro in corso dell'agente template** (`gofmt -l` segnala `mrzcells.go`; `go test ./internal/libreta` fallisce su `TestReadMRZCellsNonInventaCheckDigit`): la build installata e' quella delle 11:15 e non contiene quelle modifiche. `go vet ./...` pulito.
4. `libretto.exe summary`, `-driver`, `-rotate`, `-top-margin` (e `-page-out`, `-page-only`, `-from`, `-len`, `-width`, `-raw`, `-debug`) sono nell'uso dell'eseguibile installato; `libretto summary` su un JSON esistente stampa la riga di sempre.
5. **Baseline rigenerata** con la build installata e le verita' (`docs/baseline-regressione.tsv`; l'intestazione porta l'impronta della verita': 75 file). La vecchia e' in `docs/baseline-regressione-1016.tsv`.
6. **11:54, seconda installazione.** Alle 11:46 l'albero era di nuovo verde e l'agente template aveva consegnato `_out\libretto_template.exe`, con lo stesso sha256 (`7A13AEC247B7`) di una build che avevo appena fatto dai sorgenti. Prova pulita a 3 processi contro la baseline: 0 regressioni, 0 miglioramenti, 63 invariate (MRZ 42/63, giusti 486/584); tempo uguale alla build delle 11:15 misurando una sola immagine alla volta (13 s per entrambe; con la macchina carica la prova intera sembrava piu' lenta). `gofmt`, `go vet`, `go test` verdi alle 11:54: installata al posto della build delle 11:15, che resta in `_out\libretto_build.exe`.
7. **12:13, terza installazione:** vedi la sezione sulla seconda lettura. Rifatto il controllo di riproducibilita': ricompilando i sorgenti alle 12:13 si ottiene lo stesso sha256 (`B13F115F66A1`) della build provata.

### Seconda lettura della riga 1, nazionalita' e sesso (proposta dell'agente template, fatta alle 12:13)
**Problema.** La riga 1 dell'MRZ (cognome e nomi) e, nella riga 2, la nazionalita' (celle 11-13) e il sesso (cella 21) non hanno check digit: una lettura sbagliata passa tutti i controlli. L'agente template ha misurato che su 54 immagini 5 avevano l'etichetta ricavata dall'OCR sbagliata pur con 5/5 check (4 nei nomi, 1 nella nazionalita'); le verita' del verificatore danno 1 immagine su 40 con i nomi sbagliati.

**Cosa fa ora `Extract`** (`extract.go` e il nuovo `mrzmerge.go`). Il lettore a template gira sempre (circa 0,15-0,2 s a immagine). Se l'OCR ha tutti i check digit, `reconcileMRZ` confronta le due letture:
1. le parti coperte dai check digit devono coincidere, altrimenti non si decide niente;
2. riga 1: contano le lettere, non la posizione dei riempimenti `<`; nazionalita' e sesso: carattere per carattere;
3. dove differiscono, se **nessuna delle celle diverse** e' segnalata nelle note del lettore (incerta, cambiata dai vincoli, glifo sintetico) si prende la sua lettura e lo si scrive in `mrz_corrections`; se una e' segnalata resta l'OCR e c'e' un avviso con codice stabile: `MRZ_LINE1_UNCERTAIN`, oppure `MRZ_FIELD_UNCERTAIN` con `field` `nationality_iso` o `sex`;
4. le parti coperte dai check digit restano quelle dell'OCR e il risultato e' riletto con `mrz.Parse`: se i check non fossero tutti validi resta l'OCR. I check digit non peggiorano mai.

Scelta diversa dalla proposta: contano solo le celle che differiscono, non "nessuna cella incerta in tutta la riga". Il lettore segnala circa l'1,2% delle celle giuste (55 su 4752): in una riga di 44 celle ne segnala qualcuna nel 40% dei casi, e la regola larga avrebbe lasciato quasi sempre l'OCR. Le celle segnalate restano pero' quelle che il lettore stesso dice incerte, e nel suo corpus tutte le celle sbagliate erano segnalate.
Contratto verso il lettore a template: le note che segnalano una cella cominciano con `rigaN pos P:` (scritto nella bacheca, sezione 2). Codici nuovi in `cmd/libretto/output.go` (`toWarning`) e in `docs/campi.md`; `correction()` traduce in inglese anche le frasi del lettore a template e quelle della seconda lettura (`mrz_corrections` resta tutto in inglese).

**Misura** (harness con `-truth`, prima = build delle 11:46, dopo = build delle 12:05, una prova alla volta):

| | prima | dopo |
|---|---|---|
| nomi dell'MRZ giusti (riga 1) | 39/40 | **40/40** |
| MRZ trovata, check 5/5, campi letti, campi giusti | 42/63, 42/63, 546/882, 486/584 | identici |
| esito del confronto | | 0 regressioni, 1 miglioramento (`libretto-vero5`), 62 invariate |
| durata della prova intera | 221 s | 233 s (+0,2 s a immagine) |

Dove interviene (controllo a parte sulle 42 immagini con MRZ): 2 sostituzioni e nessun avviso. `libretto-vero5`: nomi da sbagliati a giusti. `libretto-vero5-grezza`: nazionalita' (gli avvisi scendono da 4 a 3 perche' sparisce una discordanza tra nazionalita' stampata e MRZ). Sulle altre 40 le due letture coincidono.
**Test:** `mrzmerge_test.go` (8: letture uguali, nomi con cella sicura e incerta, nazionalita' e sesso, riempimenti, parti controllate diverse, note delle celle, dentro `readMRZOriented`), `extract_hook_test.go` riscritto per la nuova firma (12, compreso il passaggio degli avvisi fino a `Result.Warnings`), `output_test.go` (+2: codici e traduzioni), `cmd/regress` (+1: nomi).

### Confronto dei nomi MRZ / stampati (proposta dell'agente MRZ)
Il VIZ non legge cognome e nomi: in `viz.go` le etichette `Apellidos / Surname` e `Nombres / Given Names` stanno in `vizLabels`, ma il valore non viene estratto (`vizIsMRZName` serve solo a scartare il testo dei nomi quando si cerca la nazionalita'). Quindi il controllo con i nomi stampati non si puo' fare in `extract.go` oggi. Scritta in bacheca la richiesta al VIZ: leggere i nomi stampati e metterli in `Result.Fields` come `cognome_stampato` e `nomi_stampati`; poi `crossCheck` li confronta (parole senza ordine, senza accenti, un prefisso e' uguale perche' l'MRZ tronca a 39 caratteri, misura `vizSim`) e, se molto diversi, aggiunge un avviso. **Codice dell'avviso:** propongo di riusare `PRINTED_FIELD_DIFFERS_FROM_MRZ` con `field` `surname` o `given_names`; se si preferisce un codice dedicato (per esempio `MRZ_NAME_DIFFERS_FROM_PRINTED`) cambia solo `toWarning`. Non ho scritto codice per questo: il rischio sulla riga 1 e' ora coperto dalla seconda lettura (sopra) e la verita' ha la riga del nome per tarare la soglia quando il VIZ leggera' i nomi. L'agente verificatore ha esposto `verify.NameMatchesMRZ` e la riga del nome in `verify.Truth`: l'harness li usa dalle 12:05 per contare i **nomi giusti**.

### Novita' dell'harness
- `-baseline PRIMA.tsv -against DOPO.tsv`: confronta due tabelle gia' prodotte senza eseguire nulla (uscita 1 se c'e' una regressione).
- `-truth` usa `verify.LoadTruth`, `verify.Compare` e `verify.Verify`: tolti `normValue`, `normDate`, `alnum` e il parser JSON della verita' (duplicati del verificatore).
- Colonna `nomi` (G, S, M, `-`): cognome e nomi della riga 1 dell'MRZ contro la riga del nome in verita'. Una riga 1 che passa da giusta a sbagliata e' una REGRESSIONE (la riga 1 non ha check digit: prima nessuna prova poteva accorgersene). Le baseline vecchie, senza la colonna, restano leggibili (niente da confrontare sui nomi).
- L'intestazione della tabella porta l'impronta della cartella delle verita' (SHA-256 di nomi e contenuto dei `.json`): se la verita' cambia, il confronto avverte che i campi giusti non sono confrontabili.
- Nota nuova: campi che ora sono sbagliati e prima mancavano (un errore si nota meno di un vuoto). `-h` esce con 0.
- 30 test in `cmd/regress`.

### Non verificato
- Il lettore a template installato e' quello delle 11:46 (il suo agente ha concluso): scarta la lettura se i vincoli costano troppo, quindi un check digit stampato sbagliato non da' piu' 5/5. Nel corpus interviene da solo su `vero9` e, con la seconda lettura, su 2 immagini.
- La seconda lettura si fida delle note del lettore: una cella sbagliata e non segnalata verrebbe presa in silenzio. Nel suo corpus non ne risultano (8 celle sbagliate su 4752, tutte segnalate) ma il corpus e' di 8 libretti e la libreria dei glifi contiene anche le scansioni di prova: su un libretto nuovo con lettere mai viste conviene guardare gli avvisi e `mrz_corrections`.
- Le verita' dei `*-grezza` e `*-pagina` sono quelle della stessa scansione; le tabelle di prima delle 11:22 usano la verita' di prima (stessi 584 campi con valore).
- La misura dei nomi e' su 40 immagini con MRZ e riga del nome in verita' (8 libretti).

### Richieste nuove in bacheca (non iniziate)
- Verificatore: riempire dall'MRZ i campi stampati che mancano o sono sbagliati (`seamans_book`, `date_of_birth`, `date_of_expiry`, `nationality`; `type` e `country_code` dalla riga 1): 11 campi recuperabili sulla build delle 11:15, la verita' coincide con l'MRZ su tutte le 45 immagini valide. Cambia il significato di quei campi nel JSON (oggi il valore stampato e quello MRZ sono separati): da decidere prima di toccare `crossCheck`.
- Verificatore: avviso quando la validita' emissione-scadenza supera 5 anni e un mese.

### Da decidere (all'utente)
1. Tenere `libretto.exe` delle 12:13 (MRZ definitivo + lettore a template + seconda lettura) oppure tornare a una build precedente: basta copiare il file (`_out\libretto_build2.exe` senza la seconda lettura; `_out\libretto_mrz.exe` solo MRZ OCR, 41/63). I backup `_out\libretto_precedente_1052.exe` e `_out\libretto_1016_backup.exe` li elimina l'utente quando non servono piu'.
2. Riempire i campi stampati dall'MRZ valido (richiesta del verificatore): migliora i campi giusti ma cambia il contenuto di quei campi.
3. `tools\summary.py` (duplicato di `libretto summary`): proposto per `_da_eliminare\`, ancora non spostato.

### File nuovi e resti
- `internal/libreta/mrzmerge.go` e `mrzmerge_test.go` (sorgenti, dell'integratore).
- In `_out\` (ignorata da git): `libretto_build.exe`, `libretto_build2.exe`, `libretto_build3.exe`, `libretto_precedente_1052.exe`, `valutazione-*.tsv` e `.log` (tabelle senza dati personali), `regress.exe` ricostruito.
- In `docs\`: `baseline-regressione-1016.tsv` e `baseline-regressione-1154.tsv`, le baseline vecchie tenute per storia.
- Nessun file eliminato. Nessun file in `scans\` toccato.

## Round 3 (2026-10-23, 12:35-13:00) — il campo `endorsement` diventa `certificate`

**Richiesta dell'utente (via committente):** i nomi dei campi JSON devono essere uguali alle etichette STAMPATE. Verificato a occhio sul facsimile AMP Anexo 8 (esempio A): il campo che chiamavamo `endorsement` ha l'etichetta `Nº Título/Certificate` (9 cifre, tra Identificación/ID number e Fecha de nacimiento/Date of birth), non "Refrendo/Endorsement"; l'esempio B non ha il campo. Nuovo nome JSON: **`certificate`**. La chiave interna italiana `refrendo` del VIZ non cambia.

### Cosa e' cambiato (un solo giro, tutto insieme)
- `cmd/libretto/output.go`: `fieldNames["refrendo"] = "certificate"`. Gli avvisi che nominano il campo (`PRINTED_FIELDS_NOT_READ`, `UNCERTAIN_FIELD`) lo dicono con il nome nuovo perche' passano da `fieldName`. Test nuovo `TestRefrendoEsceComeCertificate` (campo, assenza della vecchia chiave, due avvisi); `TestToOutputNomiInglesi` aggiornato.
- `internal/outsum`: l'elenco dei 14 campi ha `certificate` (indice 11, stesso ordine). **`outsum.Parse` legge ancora la vecchia chiave `endorsement` come `certificate`** (`legacyFieldKeys`, alias in lettura TRANSITORIO): cosi' `libretto summary` su un JSON salvato prima del cambio e i candidati compilati prima del cambio restano misurabili dall'harness senza finti "campo perso". Test nuovo con quattro casi.
- `cmd/regress`: i nomi vengono da `outsum` (legenda e messaggi); aggiornati i test. Le baseline hanno le mappe per posizione, non per nome: restano confrontabili.
- `tools/summary.py` (elenco dei 14 campi), `docs/campi.md` (tabella, mappa delle chiavi, avvisi, nota sul cambio), `README.md` (esempio JSON e quattro citazioni: e' un file della documentazione, toccato solo per la chiave), `REGISTRO.md` (voce di `output.go` e di `outsum`), bacheca.
- `schema_version` resta 1: nessun lettore esterno del JSON.
- Non toccati (non miei): codice e test del VIZ (la chiave interna `refrendo`; i suoi test di laboratorio con la mappa `refrendo -> endorsement` vanno aggiornati da lui, avvisato), `internal/verify`, `cmd/verifyjson` e le verita' (il verificatore li ha aggiornati in parallelo, con alias in lettura e `verify.Canonical`).

### Modifica all'harness emersa dalla prova
Nella prova del candidato, un'immagine (una copertina di facsimile, senza pagina dati) risultava "regressione": perdeva il campo letto `seafarers_id` e ne guadagnava un altro, ma per la verita' sono tutti campi vuoti (letti per errore). **Perdere un valore inventato non e' una regressione e guadagnarne uno non e' un miglioramento:** con la verita' disponibile su entrambe le tabelle il confronto dei "campi letti" ora ignora i campi che la verita' dice vuoti (`senzaSpuri`); restano visibili nella nota "campi letti ma vuoti in verita'". Un campo vero perso resta una regressione. Test nuovo `TestCompareCampiInventatiNonContano`. Senza verita' il comportamento e' quello di prima.

### Prova e installazione
- Gate sui sorgenti delle 12:41: `go vet ./...` e `go test ./...` verdi (`gofmt -l` segnala solo `internal/libreta/viz_words_test.go`, del VIZ, in lavorazione).
- **La build dai sorgenti contiene anche il lavoro in corso del VIZ** (riorganizzazione di `viz*.go`, `viz_name.go`, `viz_rank.go`, `viz_words.go`: non e' un effetto del cambio di nome, che non tocca la lettura). Harness a 3 processi con `-truth`, contro la baseline delle 12:13 (nuova logica): **0 regressioni, 12 miglioramenti, 51 invariate, 1 nuova** (la scansione `scans\provamrz2.png`, dell'utente, senza verita'). MRZ 43/64 (42/63 prima; la nuova ha MRZ 5/5 e 14/14 campi), check 5/5 43/64, campi letti 586/896 (546/882), **campi giusti 514/584 (486/584)**, nomi dell'MRZ giusti 40/40. Tempo medio per immagine 13,5 s (11,0 s): il VIZ lavora di piu'. I miglioramenti sono su 12 immagini, scansioni e facsimili (per esempio `vero9` da 2 a 12 giusti su 13, `libretto-vero2-grezza` da 3 a 10, `vero14` da 11 a 13 con il `certificate` di nuovo giusto).
- **Installato `libretto.exe` alle 12:49:** `_out\libretto_cert2.exe`, 7.215.104 byte, sha256 `BEE3E669E791`, cioe' l'esatto eseguibile provato (ricompilando alle 12:48 il VIZ era gia' cambiato: sha256 diverso). Prima: `B13F115F66A1` (12:13, ancora in `_out\libretto_build3.exe`). Uso: `summary`, `-driver`, `-rotate`, `-top-margin` presenti; l'uscita ha `certificate` e non `endorsement`.
- `_out\regress.exe` sostituito con la versione nuova (`0E8974CFAA94`): legge sia le uscite col nome nuovo sia quelle col nome vecchio.
- Baseline `docs/baseline-regressione.tsv` rigenerata (64 immagini; legenda con `certificate`); la precedente in `docs/baseline-regressione-1213.tsv`. Il verificatore ha riscritto le verita' con la chiave `certificate` dopo la mia prova: l'impronta della cartella cambia e la baseline va rigenerata con le verita' nuove (vedi sotto).

### Non verificato
- La build installata congela lo stato del VIZ delle 12:39: validato dall'harness (0 regressioni) ma il VIZ ha continuato a cambiare. Ogni sua versione nuova passa dallo stesso controllo.
- Le uscite JSON salvate prima del cambio (`scans\*.json`) hanno ancora `endorsement`: restano leggibili con `libretto summary` grazie all'alias; chi legge il JSON direttamente deve usare il nome nuovo.
- Il VIZ espone `vizPrintedName` (nome stampato) per un confronto con i nomi dell'MRZ in `extract.go`: non e' ancora usato.

### Comando `pdf` (richiesta del committente, 12:55)
`libretto pdf -i scan.png [-o scan.pdf] [-max 1000000] [-dpi 300] [-force]`: PDF di una pagina entro 1 MB per il gestionale. File nuovi, scritti dal committente: `cmd/libretto/pdf.go` (con la voce `case "pdf"` e la riga di uso in `main.go`), `internal/pdfout` (JPEG incorporato senza ricodifiche, qualita' massima che sta nel limite con ricerca binaria 95..80, poi riduzione dell'immagine a passi del 7%) e `tools/pdf.cmd` (per l'utente; non usa lo scanner; ripiega su `_out\libretto_pdf.exe`).
- Letto il codice: rifiuta di sovrascrivere senza `-force`, scrive con permessi 0600, ricorda che il PDF contiene dati personali. Provato su un'immagine sintetica 1560x1093 (nessun dato reale): 946 KB a qualita' 92 con il limite predefinito; con `-max 300000` 289 KB a qualita' 80 con l'immagine al 75%; secondo giro senza `-force` rifiutato. 4 test in `pdfout`.
- Aggiunto a `README.md` (comando, opzioni, tabella `Struttura`) e a `REGISTRO.md` (voci "CLI libretto (pdf)" e "pdfout"). `.gitignore` ora ignora `*.pdf` tranne in `docs/` (il PDF contiene la scansione; l'uscita predefinita sta in `scans\`, gia' ignorata, ma `-o` puo' puntare ovunque).
- **Non ancora in `libretto.exe`.** Due candidati con il `pdf`, contro la baseline delle 12:55 con `-truth`: `_out\libretto_cert_pdf.exe` (12:57, sha256 `71EC6897389F`): **4 regressioni per campo e 13 miglioramenti**; `_out\libretto_cert_pdf2.exe` (13:06, 7.279.104 byte, sha256 `5D72DFE0939A`, sorgenti copiati in `_out\src-5d72dfe0939a\`, stato del VIZ delle 13:04 fermo da almeno due minuti): **3 regressioni e 12 miglioramenti**, campi giusti 514 -> 523 su 584, MRZ 43/64, nomi dell'MRZ 40/40, tempo medio 12,8 s a immagine (13,8 s nella baseline: la macchina era piu' carica). Le regressioni non vengono dal `pdf` (che non tocca la lettura) ma dal lavoro in corso del VIZ dopo le 12:42 (`viz_vote.go`, `viz_consensus.go`, `viz_words.go`): `capacity` guadagna 9 campi (provamrz, libretto-2, libretto-vero7, librettook4/11, prova1, vero14, vero16...), mentre `place_of_birth` si perde su `dimensioni_libretto_panama` e `vero` e `regional` su `librettook1`. Per la regola 3 della bacheca non si installa una build con regressioni; scritto al VIZ e al committente. Quando il VIZ e' stabile (o se il committente accetta il compromesso) si installa `_out\libretto_cert_pdf2.exe` o il suo successore: e' una copia, un minuto.
- **Decisione del committente (13:15): nessuna regressione di campo.** Il VIZ ha ripristinato nell'albero lo stato delle 12:42 (il suo lavoro nuovo e' in `_out\viz_wip\`). Build `_out\libretto_pdf3.exe` (sha256 `9197CA1E52D3`): gate verde, harness con `-truth` contro la baseline delle 12:55: **64 invariate, 0 regressioni, 0 miglioramenti** (cioe' stessa lettura della build installata). **Installata come `libretto.exe` alle 13:25**; backup `_out\libretto_cert_1249.exe`; sorgenti in `_out\src-9197ca1e52d3\`; baseline rigenerata. Provato: l'uso mostra `pdf`, `libretto.exe pdf` su un'immagine sintetica da' 924 KB a qualita' 92 e rifiuta di sovrascrivere senza `-force`; `tools\pdf.cmd` riconosce il comando e usa `libretto.exe`.
- **13:50, nuove modifiche del committente (richiesta dell'utente: salvare con nome `NOME - COGNOME - NUMERO LIBRETTO`):** `internal/filename`, `libretto rename`, correzione dell'ordine dei nomi nel JSON per i libretti di Panama (nomi PRIMA del cognome, `IS`+`PAN`) in `toOutput`, `tools\estrai.cmd`. Gate verde sui 15 pacchetti; `regress -truth` (colonna nomi, `verify.NameMatchesMRZ` insensibile all'ordine): 64 invariate, 0 regressioni, nomi 41/41. Installata la build `CBF05BFB7E04` (backup `_out\libretto_pdf_1325.exe`, sorgenti in `_out\src-cbf05bfb7e04\`, baseline rigenerata: la verita' ora ha 63 immagini, impronta `4dc9aeb67dbb`). Provato `rename -n` su un JSON di una scansione: nome fatto di sole lettere, cifre e spazi con due separatori ` - `, exit 0. Nota: un JSON salvato con BOM (per esempio da PowerShell 5.1 con `Set-Content -Encoding UTF8`) non si legge. Documentato in `docs/campi.md` e `REGISTRO.md`.
- **14:35, VIZ corretto e avviso sul nome stampato.** (1) Candidata del VIZ `_out\libretto_viz.exe` (`821B992AC03B`): `regress -truth` contro la baseline delle 13:46, con 3 processi, senza falsi allarmi da carico: 0 regressioni, 13 miglioramenti (tutti `capacity`), 51 invariate, campi giusti 527->540/598. Ricostruita dall'albero principale (gate verde su 13 pacchetti): binario identico alla candidata. Installata con backup (`_out\libretto_rename.exe`, `CBF05BFB7E04`); baseline vecchia in `docs\baseline-regressione-1346.tsv`. (2) `internal/libreta/namecheck.go`: `checkPrintedName`, chiamata in `extract.go` DOPO `finalizeVIZ` (che aggiorna il nome con il voto finale; `crossCheck` gira prima e vedrebbe il voto vecchio). Confronto per parole senza ordine e senza accenti; salta se il nome non e' stato letto, se la confidenza e' < 0,9 o se il campo nomi della riga 1 e' tutto occupato. Misura sul corpus (conf del nome): su 40 immagini con nome letto, solo 2 arrivano a 0,9 e coincidono; un caso con 0,88 differisce pur con MRZ giusta (errore del nome stampato), quindi una soglia piu' bassa darebbe falsi allarmi. Esito: 0 avvisi nuovi, 0 regressioni; build `F1F2C016E983` installata (backup `_out\libretto_viz_main.exe`), sorgenti in `_out\src-f1f2c016e983\`, baseline rigenerata (`-1411.tsv` la precedente). Richiesta al VIZ: alzare la confidenza del nome stampato (oggi mediana 0,5) renderebbe l'avviso utile.
- **16:35, OCR nativo (urgente).** Candidata `_out\libretto_ocr.exe` (`66192AD74871`). Gate verde su 13 pacchetti piu' `cmd/panamabook`; ricostruita dall'albero: binario identico alla candidata. `regress -truth`: nel frattempo qualcuno ha spostato 14 immagini da `scans\` a `scans\vecchi test WIA\` e ha aggiunto 3 scansioni, quindi il confronto per percorso dava 20 nuove e 14 assenti: ho rimappato i percorsi nella baseline (`_out\baseline-rimappata.tsv`) e confrontato con `-against`: **64 invariate, 0 regressioni, 0 miglioramenti**, nuove 6 (le 3 scansioni nuove, senza MRZ, e le loro copie). Totali identici a prima: MRZ 43, check 5/5 43, campi 586, giusti 540/598, nomi 41/41, e anche gli avvisi: il JSON nativo coincide con quello di PowerShell. Harness 90 s (prima 249 s). Installata (backup `_out\libretto_nome.exe`); l'usage mostra summary, pdf, rename, `-driver`, `-rotate`, `-top-margin`. Baseline vecchia in `docs\baseline-regressione-PRIMA-OCR.tsv`, nuova da `valutazione-ocr.tsv` (70 immagini). `PanamaBook.exe`: nessun processo in esecuzione (Get-Process in sola lettura), costruito prima in `_out\PanamaBook.exe`, `vet` e test verdi, hash identico alla copia nella radice, quindi non copiato. `twainscan32/64.exe` non ricostruiti (non dipendono dall'OCR). `tools\*.cmd`: nessuno cita PowerShell per l'OCR; solo `tools\panamabook.cmd` lancia `powershell -Command` per mostrare le ultime righe dei log (non e' OCR). README e REGISTRO aggiornati.
- Lezione: dopo ogni installazione conviene conservare una copia dei sorgenti della build (`_out\src-<sha256>\`), altrimenti con piu' agenti che cambiano file non si puo' piu' ricostruire "la build installata piu' una modifica". Dalla prossima installazione la faccio.
