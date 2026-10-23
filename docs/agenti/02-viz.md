# 02 · Agente VIZ (`libretto-viz`)

**Ruolo:** leggere i 14 campi stampati nella zona visiva della pagina dati e confrontarli con l'MRZ.
**File suoi:** `internal/libreta/viz.go`, `viz_test.go`, `viz_integration_test.go`.
**Campi:** tipo, paese, nacionalidad, luogo_nascita, data_emissione, data_scadenza, data_nascita, livello, cargo, numero_libretto, identificazione, refrendo, id_marino, regionale (date anche in ISO come `<chiave>_iso`).

## Round 1 — campi stampati
**Implementato**
- `prepareVIZImage`: localizza le 2 righe MRZ (soglia locale, stima dell'inclinazione). Il layout e' fisso in unita' di larghezza MRZ, quindi ritaglia ogni campo in 3 varianti (scala, finestra, assottigliamento) e le impila in un "foglio" 1500xN, riconosciuto da `extractVIZ` dalle dimensioni. Senza MRZ ripiega sulla pagina ingrandita.
- `extractVIZ`: in modo foglio fa un OCR separato dei ritagli (una sola chiamata `RecognizeMany`, +2-3 s); legge ogni campo per formato (date con confusioni S/9 e O/0, numero libretto `P` o `PA` + 7 cifre, 9 cifre, livello approssimato) e vota tra le varianti. Il modo pagina (ripiego) usa pattern, distanza di Levenshtein sulle etichette e ordine cronologico delle date.
- `finalizeVIZ`: date `DD-MM-YYYY` e `_iso`, campi non letti, coerenza delle date, confronto con l'MRZ (nascita, scadenza, numero, sesso, nazionalita', regionale) con avvisi senza valori personali.

**Misure** (verita' fatta a mano, estrazione completa con MRZ attiva):

| Immagine | Trovati | Esatti | Discrepanze con MRZ |
|---|---|---|---|
| `panama.png` | 14/14 | 13 (cargo parziale) | 0 |
| facsimile 2020 | 13/13 (manca refrendo, assente sul modello) | 11 | 1 (numero) |
| Anexo 8 A | 12/14 (mancano cargo, numero libretto) | 12 | 0 |
| Anexo 8 B | 13/13 | 12 | 1 (nascita) |
| 2019 e 2017 oscurati | 1/14 e 6/14 | — | nessun crash |

Il modo pagina di ripiego e' debole (circa 23/56 campi esatti).
**Segnalazioni dell'agente:** `extractVIZ` non ha `context` (usa `Background` con timeout 60 s); il foglio passa da `prepareVIZImage` a `extractVIZ` tramite una variabile globale con mutex.
**Non verificato:** scansioni a 300 dpi, pagine molto storte (oltre 3 gradi), altre varianti del modello. Cargo e luogo di nascita (testo libero su filigrana) sono i campi piu' instabili.

## Round 2 — scansione reale a 300 dpi (concluso)
**Incarico:** far funzionare l'estrazione dei campi stampati sulla prima stampa scansionata (`scans/prova1.png`, rotazione 180), senza regressioni sui facsimili.
**Problema di partenza:** il "foglio" di ritagli non dava nessuna riga di testo OCR sulla stampa, per lo stesso motivo dei tratti spessi scoperto dall'agente MRZ.
**Cosa ha fatto (tutto in `viz.go`)**
- **assottigliamento adattivo:** sulle righe MRZ misura il rapporto spessore/altezza (media dei tratti neri tra il 25° e il 75° percentile) e ne ricava una soglia di binarizzazione: 45 sulla scansione, 120 su `panama.png`, 170 sul QR 2020 (sottile, nessuna variante in piu'). Sul foglio la soglia da sola non basta: aggiunge 11 varianti binarizzate, solo come ritagli (il foglio resta di dimensioni fisse);
- **voto dei testi:** una lettura incompleta (manca l'ultima parola) sostiene quelle piu' lunghe che la contengono;
- test: unitari su spessore, soglia, varianti e voto; integrazione su `prova1.png` (salta se manca) e `viz_stroke_integration_test.go`.

**Campi esatti su 14 (prima -> dopo, verita' fatta a mano):** `prova1` 10 -> 13 · Anexo 8 A 12 -> 13 · Anexo 8 B 12 -> 13 · `panama.png` 13 -> 13 · facsimile QR 2020 11 -> 11.
Restano sbagliati: `cargo` su prova1, A e `panama.png` (l'OCR non legge la sigla "II/2"); sul QR 2020 `luogo_nascita`, `numero_libretto`, `identificazione`.

**Verifica del committente (estrazione completa; `go vet` e `go test ./...` verdi):**

| Immagine | MRZ | Campi trovati | Tempo (prima del round 2) |
|---|---|---|---|
| `prova1.png` (rotazione 180) | 5/5 | 14/14, 0 avvisi | 13,3 s (8,4 s) |
| Anexo 8 A | 5/5 | 14/14 | 11,9 s (6,3 s) |
| Anexo 8 B | 5/5 | 13/14 (manca refrendo) | 11,3 s (6,0 s) |
| QR 2020 | 5/5 | 13/14 (manca refrendo) | 5,5 s (5,8 s) |
| `panama.png` | 5/5 | 14/14 | 10,9 s (6,9 s) |

**Prezzo:** i tempi sono quasi raddoppiati (da 6-8 s a 11-13 s) per le 11 varianti in piu'. Nessuna regressione sui campi.

**Non ha fatto (serve `extract.go`):** togliere la variabile globale con mutex e dare un `context` a `extractVIZ`. `extractVIZ` riceve solo un `ocr.Result`, che non contiene l'immagine; servirebbe cambiare la firma, ad esempio `extractVIZ(ctx, page, scaled, best)`. E' il passo 4-5 del [piano di rifattorizzazione](08-piano-rifattorizzazione.md).
**Non verificato:** il modo pagina di ripiego su scansioni spesse. L'agente ha aggiunto una nota temporanea a `REGISTRO.md`.

## File temporanei
Spostati in `_da_eliminare/viz/` (non cancellati, vedi `_da_eliminare/ELENCO.md`): `_viz_exp*`, `_out/viz_t`, `_out/dbg_viz`, gli eseguibili `viz*.exe`, gli script `e*.py`, `exp*.py`, `patch1.py`, le immagini `p1_*.png` e `prova1_rot*.png`, `zz_test.go`.

## Round 3 — generalizzazione su tre scansioni reali (concluso)
**Incarico:** far funzionare i campi su due stampe diverse (prova1 e a4-1) senza tarare su una sola, correggere `id_marino` (prima cifra 6 invece di 5 su a4-1) e `luogo_nascita` (Q letta come O, parola mancante su libretto-2).
**Cause trovate e correzioni** (`viz.go`, `viz_vote.go`, `viz_page.go`):
1. **Geometria MRZ:** i bordi della pagina allargavano le righe MRZ del 7-12%, quindi i ritagli finivano circa 50 px fuori posto. Ora usa le righe di `imgproc` (che cancella i bordi lunghi), con la ricerca vecchia come ripiego. Era anche la causa di libretto-2: andava in modo foglio con geometria sbagliata.
2. **Voto per carattere** su numeri, date, identificazione e testi liberi. Le varianti pesano per la loro accuratezza sulle cifre note dall'MRZ, rimisurata su ogni scansione.
3. Le varianti binarizzate usano una rampa morbida (180 livelli) al posto del taglio netto: accuratezza media dei caratteri da 0,56 a 0,74.
4. L'MRZ prevale su date e numero libretto, con avviso senza valori.
5. Avvisi "campo incerto: X" sui campi con letture scarse o discordanti.
6. Regionale: il suffisso OR/0R diventa DR. Cargo: allineato a un elenco chiuso di qualifiche (nessun toponimo).

**Esatti contro verita' fatta a mano:** a4-1 14/14 · prova1 13/14 (sbagliato: cargo) · libretto-2 13/14 (sbagliato: cargo) · 4 facsimili valori identici a prima. Con la geometria spostata di 1-2 px (5 prove, 3 scansioni): 12-14 esatti, media 13,2. 11 nuovi test unitari, piu' libretto-2 e soglie nei test di integrazione.
**Verifica del committente (estrazione completa):** a4-1 rot 180, MRZ 5/5, 13/14 trovati (refrendo non stampato sul modello B) in 12,2 s · prova1 14/14 in 10,8 s · libretto-2 14/14 in 9,0 s · librettook7 13/14 in 13,1 s.
**Non risolto:** il cargo "MASTER II/2" resta sbagliato (segnalato come incerto); il modo pagina (senza MRZ) resta debole (5-6/14 sulle pagine ritagliate); avviso spurio "refrendo non letto" sul modello B.
**Incidente:** per le sue prove l'agente ha salvato `scans/a4-1.png` capovolto (la scansione originale e' a 180 gradi). Il file e' tornato come l'originale (rotazione 180 riconosciuta). Da ricordare: gli agenti non devono riscrivere i file in `scans/`.
**File temporanei:** spostati in `_da_eliminare/round4/`, compreso `truth.json` (verita' fatta a mano, dati personali).

## Round 5 — qualifica (capacity), MRZ come fonte, nome stampato (23 ottobre 2026)
**Incarico:** alzare l'accuratezza VERA dei 14 campi contro `scans\verita\*.json` (priorita': qualifica con sigla di grado "II/2", riempimento/correzione da MRZ valida, luogo di nascita, avvisi `campo incerto`), senza regressioni di campo.

**Risultato (harness `regress -truth`, 64 immagini, candidata `_out\libretto_viz.exe`):** 0 regressioni, 13 miglioramenti, 51 invariate. Campi giusti 527 -> 540 su 598 con verita'. Cambia solo la qualifica (`capacity`): 23/44 -> 36/44; gli altri 13 campi sono uguali alla baseline.

| Campo | Prima | Dopo | Con verita' |
|---|---|---|---|
| type | 41 | 41 | 47 |
| country_code | 42 | 42 | 47 |
| nationality | 41 | 41 | 43 |
| place_of_birth | 35 | 35 | 43 |
| date_of_issue | 38 | 38 | 44 |
| date_of_expiry | 42 | 42 | 44 |
| date_of_birth | 41 | 41 | 43 |
| level | 42 | 42 | 44 |
| **capacity** | **23** | **36** | 44 |
| seamans_book | 41 | 41 | 46 |
| id_number | 39 | 39 | 43 |
| certificate | 20 | 20 | 24 |
| seafarers_id | 41 | 41 | 43 |
| regional | 41 | 41 | 43 |

**Cosa e' stato fatto:**
1. **Geometria MRZ** (`viz_geom.go`, `viz_pitch.go`): righe candidate di `imgproc`, passo dei caratteri dall'autocorrelazione del profilo di inchiostro, scelta della coppia di righe; larghezza = 43,5 passi. Ripara le scansioni con bordi o righe MRZ allargate.
2. **Lettura per parola** (`viz_words.go`) e **grammatica della sigla di grado** (`viz_rank.go`: numerale romano I..X o cifra, confusioni 1/I/l, "/" ~ 1/t/T, N ~ IV, distanza pesata; nessun elenco chiuso di qualifiche). Le parole si leggono in ritagli isolati nella stessa chiamata OCR degli altri ritagli. Una parola letta in modo concorde da due elaborazioni pesa 1,5 (`vizWordWeight`).
3. **Consenso per allineamento** (`viz_consensus.go`) per la sola qualifica: allineamento semi-globale a un pivot, voto per colonna. Per gli altri campi di testo resta il voto per lunghezza (il consenso faceva regredire luogo di nascita e regionale, dove l'apostrofo al posto della I si somma alle letture con la I).
4. **Riconciliazione con l'MRZ** (`viz_finalize.go`): numero libretto, date, paese, tipo, aggettivo di nazionalita' (`viz_nationality.go`), regionale dal campo opzionale; le discrepanze vanno in `Result.Warnings` senza valori.
5. **Nome stampato:** `vizPrintedName(r *Result) (name string, conf float64, ok bool)` (`viz_name.go`), non serializzato in `Fields`; la confidenza e' la frazione di peso del gruppo per l'accordo medio per carattere. `extract.go` non lo usa ancora.
6. Chiave `refrendo` interna invariata; il JSON dice `certificate` (a carico di `extract`/`cmd`).

**Calibrazione verticale** (`viz_calib.go`): misura lo scostamento delle due colonne dalle ancore (date, livello, id marittimo, numero libretto) e sposta le finestre. Misurata: con la calibrazione 6 regressioni (le finestre spostate danno letture di riga intera vuote), quindi e' **spenta** (`vizNoCalib = true`); resta per riprovarla (`LAB_CALIB=1` nel laboratorio).

**Laboratorio** (tag `vizlab`, `viz_lab2_test.go`): `TestLabFull` esegue il motore sul corpus con cache su disco dell'OCR (chiave sha256 dell'immagine, MRZ presa dai JSON di una prova), in ~100 s invece di ~9 minuti. La cache contiene testo OCR di documenti reali: sta fuori dal repo (cartella temporanea della sessione).

**Non risolto / non verificato:**
- Qualifica ancora sbagliata su 7 immagini (scana2, vero15, dimensioni, A facsimile, vero2-grezza e altre); luogo di nascita 35/43 (filigrana); data di emissione 38/44; cifre confuse 9/S/5 in `id_number` (QR, vero1).
- Pagine ritagliate (librettook8/10): `type`, `country_code`, `seamans_book` mancanti; nessun controllo formato/etichetta per evitare il numero libretto in `id_number`.
- Immagini senza MRZ (scana1, librettook1-grezza) dipendono dal lettore MRZ.
- Copertura `campo incerto`: restano ~10 campi sbagliati senza avviso (sul corpus).
- Valutazione leave-one-booklet-out: l'unico parametro nuovo, `vizWordWeight`, e' stato provato a 1, 1,5, 2 (qualifica 35/36/36); non c'e' una taratura su una sola scansione.

**File temporanei (non cancellati):** `_out\wip\` (copia di lavoro del modulo, con giunzioni a scans/docs/facsimili: rimuovere le giunzioni con `rmdir`, mai `rm -rf`), `_out\viz_wip\`, `_out\viz_frozen_1242\` e `_out\libretto_viz_frozen.exe` (stato 12:42), `_out\j_base\` (JSON del corpus: dati personali), `_out\reg_*.tsv|txt`, script `_out\labrun.sh`, `_out\runset.py`, `_out\tsvfields.py`, `_out\eval\`; cache OCR nella cartella scratchpad della sessione.
