> **Nota (pubblicazione):** gli script di `tools\` citati in questi log hanno cambiato nome (`prova-libretto.cmd` -> `scan-wia.cmd`, `scansiona.cmd` -> `scan-twain.cmd`, `estrai.cmd` -> `extract.cmd`, `pdf.cmd` -> `make-pdf.cmd`, `panamabook.cmd` -> `start-panamabook.cmd`, `verifica-twain64.cmd` -> `check-twain64.cmd`); gli script di prova sono stati tolti. Vedi `tools/README.md`.

# Agenti del progetto `libretto`: cosa hanno fatto

Cartella di studio. Un file per agente, aggiornato a ogni nuovo rapporto. Ultimo aggiornamento: **2026-10-23**.
Chi possiede quali file, le firme che collegano i moduli, lo stato di ogni agente e le richieste aperte sono nella bacheca [COORDINAMENTO.md](COORDINAMENTO.md).
Nessun file qui contiene dati personali: dove servivano esempi si descrive solo l'esito (es. "check digit 5/5").

## Indice

| File | Agente | Ruolo | Stato |
|---|---|---|---|
| [01-mrz.md](01-mrz.md) | `libretto-mrz` | lettura delle righe MRZ, ritaglio della pagina dati | round 3 concluso |
| [02-viz.md](02-viz.md) | `libretto-viz` | campi stampati (zona visiva) | round 3 concluso nel rapporto; **round 4 in corso** (tabella nazionalita', jitter, geometria): rapporto da scrivere |
| [03-wia.md](03-wia.md) | `libretto-wia` | scanner WIA e TWAIN, ritaglio a texture, scansione vuota, area | WIA concluso; **TWAIN studiato e sondato, scansione non provata** |
| [04-documentazione.md](04-documentazione.md) | `libretto-docs` | README, campi, registro, pulizia | concluso (3 passate; vedi sotto) |
| [05-revisione-confini.md](05-revisione-confini.md) | `architect-boundary-reviewer` | confini tra pacchetti | concluso |
| [06-revisione-dati.md](06-revisione-dati.md) | `architect-data-reviewer` | schema JSON e dati su disco | concluso (schema inglese applicato) |
| [07-revisione-dominio.md](07-revisione-dominio.md) | `architect-domain-modeler` | modello del dominio | concluso |
| [08-piano-rifattorizzazione.md](08-piano-rifattorizzazione.md) | `architect-migration-planner` | piano di rifattorizzazione | concluso (piano, non applicato) |
| [09-revisione-prestazioni.md](09-revisione-prestazioni.md) | `architect-scalability-reviewer` | prestazioni e operativita' | concluso |
| [10-revisione-sicurezza.md](10-revisione-sicurezza.md) | `architect-security-reviewer` | sicurezza e privacy | concluso |
| [11-revisione-test.md](11-revisione-test.md) | `architect-test-strategist` | strategia di test | concluso |
| [13-libretto-exe.md](13-libretto-exe.md) | `libretto-exe` | integratore: `extract.go` e punti di aggancio, harness di regressione `cmd/regress`, build di `libretto.exe`, bacheca | attivo: harness, aggancio `mrzCellsReader`, seconda lettura della riga 1; **round 2: `libretto.exe` sostituito alle 12:13** (MRZ 42/63, nomi MRZ 40/40) |

| [15-gui.md](15-gui.md) | agente GUI | **Panama Book** (`PanamaBook.exe`): interfaccia per l'operatore, correzione dei campi e memoria per libretto, manuale in [../panamabook.md](../panamabook.md) | bozza funzionante |

Le definizioni degli agenti del progetto sono in `.claude/agents/` (libretto-mrz, libretto-viz, libretto-wia, libretto-docs, libretto-exe, libretto-verifica).
I sette revisori sono gli agenti di architettura di sola lettura.

## Regole comuni a tutti
- **Mai `scan` o `read`**, mai `tools\prova*.cmd`, mai `twainscan32.exe` ne' alcun programma che avvii lo scanner: lo scanner reale puo' contenere un documento. Ammessi `info`, `extract` e `twainprobe32.exe` (non scansiona).
- **Nessun dato personale** nei rapporti: solo esiti.
- **Ogni agente possiede solo i suoi file**: gli altri si toccano solo tramite il rapporto o la bacheca [COORDINAMENTO.md](COORDINAMENTO.md) (che ha anche la tabella dei proprietari e le regole per sostituire `libretto.exe`).
- **Nessuna cancellazione senza conferma** dell'utente: i resti si spostano in `_da_eliminare/` e li cancella l'utente.
- **Non riscrivere i file in `scans/`** (e nelle sue sottocartelle): sono le scansioni reali.

## Stato del programma (verificato il 2026-10-22)

| Immagine | Rotazione | MRZ | Campi trovati | Tempo |
|---|---|---|---|---|
| scansione reale `prova1.png` (stampa, 300 dpi) | 180 | 5/5 | 14/14 | 10,8 s |
| facsimile 2020 (QR) | 0 | 5/5 | 13/14 | 5,5 s |
| facsimile 2026 A | 0 | 5/5 | 14/14 | 11,9 s |
| facsimile 2026 B | 0 | 5/5 | 13/14 | 11,3 s |
| `panama.png` | 0 | 5/5 | 14/14 | 10,9 s |
| `a4-1` (stampa) | 180 | 5/5 | 13/14 (refrendo non stampato) | 12,2 s |
| `libretto-2` | 0 | 5/5 | 14/14 | 9,0 s |
| `librettook7` | 0 | 5/5 | 13/14 | 13,1 s |

Campi *trovati*, non per forza esatti: sul VIZ risultano 13-14/14 esatti sulle scansioni reali (sbaglia `cargo`, ora `capacity`, quando c'e' la sigla `II/2`). Misure dal round 3 del VIZ
(`02-viz.md`); quelle dopo nazionalita'/jitter non sono ancora riportate.

Percorso frontale "Return Scan": tira dentro il documento e lo riespelle dalla stessa apertura (comportamento previsto, manuale PFU p. 12 e 45).
**Misure sul libretto vero (WIA):** la pagina dati sta tra 90 e 180 mm dal bordo d'ingresso (216 mm di larghezza, 46-173 mm), il driver WIA ignora l'inizio dell'area, quindi si legge 190 mm
(`tools\prova-libretto.cmd`); inserendo per primo il bordo con l'MRZ, in cima all'immagine finisce l'informativa. Dettagli in [03-wia.md](03-wia.md).

## Decisioni aperte (all'utente)
1. ~~Correzioni immediate della CLI~~ **fatte**: conferma prima di scansionare (`-yes` per gli script), rifiuto di sovrascrivere, `scans/` in `.gitignore`, Ctrl+C solo per OCR, avviso di `-debug`.
2. **Snapshot prima della rifattorizzazione**: `git init` oppure copia fuori dall'albero. Il progetto **non e' un repository** e `viz.go` (circa 2400 righe) e' il file piu' esposto.
3. ~~Lingua delle chiavi JSON~~ **decisa**: inglese, uguale alle etichette stampate (`schema_version` 1). Resta da sapere se qualcuno legge ancora il JSON vecchio (mappa in `docs/campi.md`).
4. **Pulizia dei file temporanei**: elenco in `REGISTRO.md`. Non e' stato eliminato nulla. La quarantena `_da_eliminare/` e' salita a circa 366 MB e `scans/vecchi test WIA/` pesa circa 380 MB.
5. **Driver predefinito**: oggi `wia`; passare a `twain` solo dopo la prova di `twainscan32.exe -from 90 -len 100` (o `tools\prova-twain.cmd`) con un libretto vero.
6. **Rifattorizzare `viz.go`**, togliere lo stash globale (ora tre variabili globali: ritagli, pagina, letture) e dare un `context` a `extractVIZ`: piano in [08-piano-rifattorizzazione.md](08-piano-rifattorizzazione.md), non applicato.
7. **Test di integrazione e percorsi**: alcuni (`imgproc`, `mrzband`) cercano le scansioni in `scans\` e non in `scans\vecchi test WIA\`.

## Storico aggiornamenti

| Data | Cosa |
|---|---|
| 2026-10-22 | Creata la cartella. Round 1 di MRZ, VIZ, WIA, Documentazione. Prima scansione reale. Round 2 MRZ (tratti spessi). Sette revisioni di architettura. |
| 2026-10-22 | Riordino file: resti degli agenti in `_da_eliminare/` (non cancellati). Driver per installazione manuale in `drivers/installazione_manuale/`. |
| 2026-10-22 | Round 2 VIZ concluso: `prova1` 14/14 campi trovati (13 esatti). Tempi quasi raddoppiati. Resta da fare: stash globale e `context` (richiede `extract.go`). |
| 2026-10-22 | Round 3: ritaglio pagina dati in Go (agente MRZ), niente sovrascrittura e temporanei sicuri (agente WIA); CLI con conferma, `-force`, Ctrl+C, `-page-out` e `tools/prova.cmd` (committente). |
| 2026-10-22 | Round 4: VIZ generalizzato su 3 scansioni reali (14/14 trovati su prova1 e libretto-2); area di scansione limitata (`-len` funziona, `-from` ignorato dal driver); prove con il foglio: la pagina dati si ottiene con il ritaglio software. |
| 2026-10-22 | Round 5 (sera). **Uscita JSON in inglese** con `schema_version` 1 e avvisi con codice (`cmd/libretto/output.go`, `output_test.go`). Nuove opzioni `-page-only`, `-len`, `-from`, `-width`, `-raw`, `-driver wia\|twain`. Script `tools/prova-libretto.cmd` (190 mm, ciclo "prossima prova", finestra aperta), `prova-pagina.cmd`, `prova-twain.cmd`. Misure sul libretto vero: pagina dati tra 90 e 180 mm, il driver WIA ignora l'inizio dell'area. |
| 2026-10-22 | Ritaglio automatico del documento a **texture** (`imgproc/autocrop.go`) e raddrizzamento `imgproc/deskew.go` (usato da `Extract`, `DataPage`, `readMRZBand`). Le scansioni di prova sono ora in `scans\vecchi test WIA\` (ignorate da git). |
| 2026-10-22 | **TWAIN**: `internal/twain` (32/64 bit, strutture a 2 byte, test con data source finto), `cmd/twainprobe`, `cmd/twainscan`, driver `-driver twain` in `cmd/libretto/twaindrv.go`. Studio statico di PaperStream IP 3.40.2 (solo 32 bit). **Il driver accetta l'alto del riquadro** (`ICAP_FRAMES`); resta da provare con un libretto vero (`twainscan32.exe -from 90 -len 100`). Dettagli in [03-wia.md](03-wia.md). |
| 2026-10-22 | VIZ round 4 (in corso): `viz_nationality.go` (tabella ISO3 -> aggettivi, 199 codici), `viz_jitter.go` (ritagli spostati per i testi liberi), `viz_vote.go`/`viz_page.go`. Il rapporto in `02-viz.md` non e' ancora scritto. |
| 2026-10-22 | Documentazione: terza passata di README, `docs/campi.md` (chiavi inglesi, codici degli avvisi, mappa delle vecchie chiavi), `REGISTRO.md` (conteggio test, moduli TWAIN, nuova pulizia proposta). Nessun file eliminato o spostato. |
| 2026-10-23 | **Integratore** (`libretto-exe`, [13-libretto-exe.md](13-libretto-exe.md)): harness di regressione `cmd/regress` (`_out\regress.exe`) con baseline di `libretto.exe` in `docs/baseline-regressione.tsv`; pacchetto `internal/outsum` (14 campi e 5 check digit in un posto solo, usato da `libretto summary` e dall'harness); punto di aggancio `mrzCellsReader` in `extract.go` per il lettore MRZ a template; bacheca [COORDINAMENTO.md](COORDINAMENTO.md). |
| 2026-10-23 | **Integratore, round 2** ([13-libretto-exe.md](13-libretto-exe.md)): candidato MRZ definitivo valutato anche con le verita' di `scans\verita`; `libretto.exe` sostituito tre volte (11:35, 11:54, **12:13**), l'ultima con la build dai sorgenti delle 12:05: MRZ definitivo + lettore a template + **seconda lettura della riga 1, nazionalita' e sesso** (`internal/libreta/mrzmerge.go`, proposta dell'agente template). Risultato sul corpus di 63 immagini rispetto al `libretto.exe` delle 10:16: MRZ trovata 33 -> 42, check 5/5 32 -> 42, campi stampati giusti 477 -> 486 su 584, nomi dell'MRZ giusti 39/40 -> 40/40 (la riga 1 non ha check digit). Harness con `-against`, impronta della verita' e colonna `nomi`. Richieste in bacheca al VIZ (nomi stampati, `endorsement`) e al verificatore. |

## Come si aggiorna
Quando un agente consegna un rapporto: aggiungere una sezione datata nel suo file (cosa ha fatto, misure, non verificato, file da pulire),
aggiornare la tabella "Stato" qui sopra e una riga nello storico.
