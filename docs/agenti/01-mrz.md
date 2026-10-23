# 01 · Agente MRZ (`libretto-mrz`)

**Ruolo:** leggere le due righe MRZ (ICAO TD3, 44 caratteri, codice `IS`, emittente `PAN`) dalla pagina dati e validarle con i check digit.
**File suoi:** `internal/libreta/mrzband.go`, `mrzband_test.go`, `mrzband_integration_test.go`, `internal/imgproc/rows.go`, `deskew.go`, `pagecrop.go`.

## Round 1 — lettura delle righe MRZ
**Metodo** (verificato prima di delegare): l'OCR di Windows sulla pagina intera perde i simboli `<` e a volte la prima riga. Funziona invece
l'OCR riga per riga su ritagli binarizzati.
- binarizza, ripulisce i bordi, trova la coppia di righe MRZ in basso per proiezione dell'inchiostro;
- ritaglia le righe a 3 scale (altezza 32/45/60 px) e le legge con **una sola** chiamata `ocr.RecognizeMany`;
- `mrz.Repair` prova le combinazioni; `fitLength` ripara i `<` persi;
- se l'OCR lascia vuota una riga (capita quasi sempre alla riga 2) la rilegge a pezzi sulle 44 celle e assembla per voto;
- se non ci sono due righe adatte ritorna subito score 0 senza chiamare l'OCR (le rotazioni sbagliate costano poco).
**Aggiunte in `rows.go`:** `MRZPairs`, `MainSpan`, `ClearLines` (toglie le ombre nere dello scanner che fondevano le righe); `TextRows` ignora i segni isolati.

**Misure:** facsimile QR 5/5 in 1,4 s · Anexo 8 A 5/5 in 3,3 s · Anexo 8 B 5/5 in 3,1 s · `panama.png` 5/5 (3 correzioni) in 2,9 s ·
pagina capovolta di 180° 5/5, rotazione trovata, 6,2 s in totale · facsimili 2017 e 2019 con dati oscurati: MRZ non trovata, score 0, nessun crash.
**Non verificato:** scansioni a 300 dpi vere (poi toccato nel round 2); righe a 90°/270° oltre al caso "nessuna coppia".

## Round 2 — tratti spessi sulla prima scansione reale
**Problema:** sulla stampa a 300 dpi l'OCR di Windows restituiva 0 righe su tutti i ritagli, anche se i ritagli erano perfetti.
**Causa (misurata):** la stampante laser lascia i tratti piu' spessi dei facsimili. Assottigliando il nero di 1 px il motore legge la riga.
**Scoperta dell'agente:** il rapporto spessore/altezza **non** distingue le stampe leggibili da quelle illeggibili (prova1 0,19; A e B 0,18).
Il motore legge solo in una finestra stretta di inchiostro residuo dopo l'erosione (circa 25-30% dell'area della riga), con esito caotico per piccole variazioni.
**Cosa ha fatto:**
- nuove funzioni in `rows.go`: `StrokeWidth`, `Erode`, `GrayCrop`, `InkFraction`, `ThinToInk` (soglia scelta per ottenere una frazione di inchiostro), `PadWhite`;
- `MRZPairs` scarta le righe piu' larghe di 12 volte la propria altezza (prima, a rotazione 0, nasceva una coppia assurda alta 749 px che sprecava due chiamate OCR);
- le varianti normali restano le prime; se entrambe le righe restano vuote parte la variante assottigliata (14 immagini, 1 o 2 px, piu' soglie) e si ferma ai check validi;
- se il rapporto e' >= 0,185 i due gruppi vanno in una sola chiamata (risparmio di circa 1 s di avvio di PowerShell);
- test: unitari senza OCR (spessore, erosione, `ThinToInk`, scarto righe alte) e `TestIntegrationScansioneProva1` (salta se il file manca).

**Misure (`readMRZBand`, rotazioni 0 e 180, check 5/5 ovunque):** prova1 4,1 s · QR 1,5 s · A 3,4 s · B 3,5 s · panama 3,1 s · B capovolta 3,2 s.
**Verifica del committente (estrazione completa):** prova1 rotazione 180, MRZ 5/5, 11/14 campi, 8,4 s; nessuna regressione sulle altre tre immagini.

**Non verificato / fragile**
- la finestra di inchiostro e' tarata su **una sola scansione reale**: una stampa un po' diversa potrebbe non leggersi;
- il caso "B capovolta" e' passato da 1,3 s a 3,2 s (seconda chiamata OCR).

## File da cancellare (serve il tuo ok)
`internal/libreta/zz_time_test.go` (temporaneo, solo tag integration) · `_out/exp`, `_out/exp2`, `_out/exp3`, `_out/exp4`, `_out/dbg_t`, `_out/dbg_mrz` · `scans/dbg3`, `scans/dbg4`.
Le cartelle di debug contengono immagini con dati personali.

## Round 3 — ritaglio della pagina dati e test del pacchetto `mrz` (concluso)
**Ritaglio** (`internal/imgproc/pagecrop.go`, test unitari e di integrazione): `CropDataPage(img) (image.Image, bool)` e `DataPageGeometry` (rettangolo, scala, righe MRZ). Nessun OCR: usa `Binarize`, `ClearLines`, `TextRows`, `MRZPairs`.
Geometria TD3: 125x88 mm, 44 caratteri a passo 2,54 mm, MRZ centrata. **Calibrazione:** il margine inferiore passa da 5 a **7,5 mm** (costante `td3BottomMM`): il bordo reale sta a 7,0-8,4 mm sotto la riga 2; l'altezza resta 88 mm.

| Immagine | Ritaglio (px) | Aspetto (atteso 1,420) | MRZ intere |
|---|---|---|---|
| `prova1.png` (ruotata 180) | 1523x1072 | 1,421 | si |
| facsimile QR 2020 | 737x518 | 1,423 | si |
| Anexo 8 A | 739x520 | 1,421 | si |
| Anexo 8 B | 740x521 | 1,420 | si |
| `panama.png` | 727x513 | 1,417 | si |

Scarto massimo 0,2%. Sulla scansione reale resta solo una sottile striscia della pagina accanto in alto. Il bordo alto sui facsimili varia di circa ±3 mm perche' sono ritagli diversi tra loro.
**Test `mrz`:** correzione con 2 candidati validi non applicata; riga 2 di lunghezza diversa da 44 non toccata da `Normalize`; campo opzionale tutto `<` valido con check `<` o `0`; `var now = time.Now` in `mrz.go` (comportamento invariato) per testare il secolo della data di nascita.
**Collegamento (fatto dal committente):** `libreta.DataPage(path, rotation)` in `extract.go` e opzione `-page-out` in `libretto extract` e `read`. Verificato su `prova1.png`: pagina dati 1523x1072, aspetto 1,421, MRZ 5/5, 14/14 campi.
**Non verificato:** calibrazione basata su **una sola scansione reale**.

## Round 4 — scansioni TWAIN pulite dello stesso libretto (concluso)
**Problema:** tre scansioni nitide e dritte dello stesso libretto 2026 (`scana2`, `DEFINITIVO_spero`, `provamrz`) davano "MRZ non trovato" mentre `vero16` si leggeva. Le cause sono **tre e indipendenti**:

| Scansione | Causa (misurata) | Correzione |
|---|---|---|
| `scana2` | `ClearLines` cancellava le **colonne intere** con piu' del 35% di inchiostro: la foto scura del titolare le rende "scure" e quelle colonne attraversano anche le righe MRZ, che perdevano "ISPAN" e "PA012" a sinistra (righe da x=328 invece che da x=107) | `ClearLines` cancella solo le **corse verticali lunghe** (>= max(40 px, 8% dell'altezza)) delle colonne scure: le aste dei caratteri (36-40 px) restano |
| `DEFINITIVO_spero` | righe MRZ inclinate di **1,2 gradi** (29 px di dislivello su 1350, quasi un'altezza di carattere), sotto `DeskewMinAngle` (2 gradi) quindi non raddrizzate; con quell'inclinazione il motore OCR restituisce testo vuoto anche su ritagli perfetti | `imgproc.EstimateRowsSkew` misura l'inclinazione sulle sole righe MRZ (picco netto anche a 0,5 gradi); `imgproc.ShearRows` raddrizza la banda facendo scorrere in verticale le colonne di pixel interi (**niente interpolazione**: `RotateAngle` sfoca e ingrossa il tratto, spessore/altezza 0,125 -> 0,212, e leggeva peggio). La banda raddrizzata si legge prima di quella originale (`straightenBand`, soglia 0,4 gradi) |
| `provamrz` | il motore OCR restituisce testo vuoto su **tutte** le varianti della riga intera (anche a mano, con altre scale, sfocata, invertita: verificato dal committente), ma legge bene i pezzi da 5-15 caratteri; il passo a pezzi pero' partiva solo se una riga era stata letta per intero | il passo a pezzi (passo 3) parte anche con **entrambe le righe vuote** |

**Altre correzioni emerse dalle misure** (tutte in `mrzband.go`, con test unitari senza OCR):
- **Riga 1 senza check digit:** una lettura intera di 41-47 caratteri veniva "aggiustata" da `fitLength` (un `<` in piu' o in meno) e accettata se la riga 2 era giusta: visto un cognome mutilato (`CORDERO<SALINAS` letto `TEJEII`) con 5/5 check. Ora la riga 1 conta come letta per intero solo se ha **44 caratteri esatti e comincia con `ISPAN`** (altrimenti si rilegge a pezzi), `bestReading` preferisce le righe 1 che cominciano con `ISPAN` e di lunghezza esatta, e `readingDone` pretende check 5/5, **emittente `PAN`** e riga 1 esatta. Il punteggio di `readMRZBand` arriva a **14** (`scoreSure`: 13 + 1 se la riga 1 e' esatta); 0 resta "non trovato".
- **`fillFiller`:** il motore non restituisce nulla per i pezzi fatti di soli `<` (riga 1 con nome corto e tanti `<`: `libretto-vero5`). Le celle senza voti che somigliano (forma sulla griglia 6x10 piu' larghezza) ai `<` gia' letti diventano `<`. Misurato: `<` a distanza 0,08-0,21 dal modello, ogni lettera >= 0,36; soglia adattiva 0,18-0,30.
- **`mrzUpsideDown`:** nella riga 1 il riempimento `<` sta a destra; se le 8 celle a sinistra sono uniformi e quelle a destra no, il testo e' capovolto e `readMRZBand` ritorna subito (prima: 2 chiamate OCR, ora 0,2 s). Misurato su 22 scansioni/facsimili nei due versi: dritte sinistra >= 0,52 e destra <= 0,20, capovolte sinistra <= 0,38 e destra >= 0,45; nei casi dubbi non salta.
- **Nessuna MRZ:** con entrambe le righe intere vuote i pezzi si provano una volta sola e si prosegue solo se leggono le cifre dei campi data della riga 2 (`dateDigits`); non si provano affatto se una riga letta e' molto piu' lunga di 44 caratteri (testo normale: copertine). Una riga 2 intera conta come letta solo con almeno 10 cifre.
- **Struttura:** `readMRZRows` e' diventata la struttura `bandRead` con le fasi `whole` (righe intere + pezzi dalla variante letta) e `thinPieces` (pezzi su varianti assottigliate, con scadenza): `readMRZBand` prova prima `whole` su banda raddrizzata e originale, poi `thinPieces`. Tetto 8 s per banda, diviso tra le bande.

**Misure (`readMRZBand` sulle 63 immagini del corpus, con le rotazioni di `Extract`; prima = codice del 2026-10-22, dopo = round 4):** MRZ con check validi **29 -> 41** su 63, **0 regressioni**, impronta (hash) delle due righe **identica** su tutte le 29 gia' lette; tempo medio sulle gia' lette 3,5 -> 3,9 s (punteggio 13 -> 14 sulle esatte).

| Immagine | Prima | Dopo |
|---|---|---|
| `provamrz.png` | non trovata | 5/5, rot 0, 6,2 s |
| `DEFINITIVO_spero.png` | non trovata | 5/5, rot 0, 6,1 s |
| `scana2.png` | non trovata | 5/5, rot 0, 3,4 s |
| `vero16.png` | 5/5, 1,9 s | 5/5, 2,4 s |
| `scana.png` | non trovata | 5/5, rot 180, 4,1 s |
| `vero13`, `vero14`, `vero15` | non trovata | 5/5 (4,5 / 4,1 / 2,0 s) |
| `vecchi test WIA\vero6.png` | non trovata | 5/5, 5,5 s |
| `libretto-vero2-grezza.png` | 3/5 (12,5 s) | 5/5, 3,7 s |
| `libretto-vero4-grezza.png` | non trovata | 5/5, 4,9 s |
| `libretto-vero5.png` e la sua grezza | non trovata | 5/5 (5,5 s) |
| facsimili QR, Anexo 8 A e B, `panama.png`; `prova1`, `a4-1`, `libretto-2` e grezza, `libretto-vero1-3` (+ grezze), `libretto-vero2/4/6/7`, `librettook1/4/7/11` (+ grezze), `vero`, `vero10`, `vero12` | 5/5 | 5/5, stesse righe (`panama.png` 3,3 -> 7,2 s: la banda raddrizzata di 1,25 gradi non si legge e si ripiega sull'originale) |
| copertine, facsimili 2017/2019, `librettook*` (pagina interna di copertina, senza MRZ) | non trovata | non trovata, nessun crash, tempi invariati (7,9 s la copertina 2019, che ha due righe di testo normale) |
| `scana1` (libretto tagliato a sinistra), `vero11` (solo la striscia MRZ, capovolta e tagliata a destra) | non trovata | non trovata (manca l'inizio delle righe) |

**Fonti AMP (`facsimili\panama\fonti\`):** i quattro PDF **non descrivono l'MRZ a parole**. Le circolari MMC-331 (2019, 2020) dicono solo "misure e forma secondo ICAO" (QR integrato dal 2020); `DGGM-UCYC-020-2017` riguarda il sigillo OIT/STCW; l'Anexo 8 (2026) non ha strato di testo (solo le immagini delle pagine dati A e B). Quello che si sa dell'MRZ viene dai campioni (immagini) e dalle scansioni reali, riportato qui solo come struttura:
- TD3, 2 righe x 44 caratteri. Riga 1 = `IS` + `PAN` + COGNOME`<<`NOMI + `<` fino a 44 (solo A-Z e `<`); il codice documento `IS` non va corretto in `SI` (e' la stampa della zona visiva).
- Riga 2 = numero libretto di 9 caratteri (modello 2020: `P` + 7 cifre + `<`; modello 2026, campioni A/B e scansioni: `PA` + 7 cifre), check, nazionalita' del titolare (3 lettere), nascita AAMMGG + check, sesso, scadenza AAMMGG + check, **campo opzionale di 14 caratteri = nome dell'ufficio regionale** (lettere + `<`: `PANAMA` + 8 `<` nei campioni 2020 e 2026) + check opzionale + check composito (le ultime due cifre). Il campione 2017 (nomi e numero oscurati da AMP) mostra la stessa forma: `<` prima del check del numero, riempimento `<` e due cifre in fondo. Il campione 2019 (Appendix 6, sistema manuale) non ha MRZ.
- I campioni 2020 e 2026 tornano con i check ICAO 9303 (pesi 7-3-1); la nazionalita' non e' coperta da nessun check (nemmeno dal composito): se l'OCR la sbaglia (`NIC` letto `NUS`) non ci si accorge.
- Vincoli usati: `mrz.Normalize` (classe lettera/cifra/`<` per posizione, gia' presente), prefisso `ISPAN` e lunghezza 44 esatta della riga 1, cifre nei campi data della riga 2 (`dateDigits`), forma del `<` (`fillFiller`).

**Provato e scartato:** rotazione bilineare della banda (`RotateAngle`: sfoca, peggiora `panama.png`); riscalatura (area, bilineare, LANCZOS), grigio, sfocatura e inversione dei ritagli, due righe nella stessa immagine, pagina intera ingrandita: su `provamrz` e `DEFINITIVO_spero` il motore legge testo vuoto in ogni caso (a `DEFINITIVO_spero` serviva raddrizzare, a `provamrz` leggere a pezzi).

**Non verificato / fragile**
- il motore OCR resta capriccioso: la lettura a riga intera dipende da dettagli di pixel (le stesse righe ruotate di 0,5 o di 1 grado danno esiti opposti); il passo a pezzi e' la rete di sicurezza, misurato su due titolari (il primo in `scana2`, `provamrz`, `DEFINITIVO_spero` e `vero*`, il secondo in `libretto-vero*`) piu' i campioni AMP;
- la riga 1 non ha check digit: una riga 1 sbagliata che comincia con `ISPAN` e ha 44 caratteri passa. Conviene confrontarla con i nomi stampati (VIZ) in `extract.go` (richiesta all'integratore, nella bacheca);
- `fillFiller` e `mrzUpsideDown` sono tarati su 22 immagini; il secondo non decide con nomi lunghissimi (nessun `<` a destra);
- la striscia MRZ tagliata (`vero11`, `scana1`) costa 6-12 s e non si legge; `panama.png` e QR costano 3-5 s in piu' per il tentativo sulla banda raddrizzata.

**Misure con `libretto extract` (intero, con VIZ) e con l'harness dell'integratore** (`_out\libretto_mrz.exe`, 11:03, contro `docs\baseline-regressione.tsv` di `libretto.exe` delle 10:16, che aveva gia' la correzione di `ClearLines`): **0 regressioni, 9 miglioramenti, 54 invariate**; MRZ 41/63, check 5/5 41/63 (205/315 check validi), campi 541/882. Contro il codice originale del 2026-10-22 (62 immagini in comune): check 5/5 **29 -> 39** (+2 immagini nuove, `provamrz` e `dimensioni_libretto_panama`, entrambe 5/5), campi letti 502 -> 514; i soli campi che calano (`libreta_2017_osservazioni`, `scana1`: 4 -> 1 e 3 -> 2) sono su pagine senza dati. I tempi dell'harness (3 processi in parallelo, altri agenti attivi) non sono confrontabili con quelli delle misure sopra.

I file temporanei di questo round sono in `_da_eliminare\mrz_round4\` (vedi `_da_eliminare\ELENCO.md`): due test `zz_*` con tag `integration`, programmi di prova, copie del sorgente, candidati intermedi e **cartelle di debug con dati personali** (`out\p2`, `ds`, `pm`, `v5`, `a4`, `pn`, `cv`).
