# 14 · Verifica dei JSON (`libretto-verifica`)

**Ruolo:** dire se i campi che `libretto` estrae sono **giusti**, non solo presenti. La metrica "campi 14/14" conta i campi non vuoti: un cargo
con tre lettere sbagliate vale come trovato. Qui ci sono due controlli: la **verita' letta a occhio** sull'immagine e le **regole di coerenza**
che non hanno bisogno della verita'.
**File suoi:** `internal/verify/**`, `cmd/verifyjson/**`, `scans\verita\**` (verita': contiene dati personali, `scans/` e' nel `.gitignore`),
questo file. **Eseguibile:** `_out\verifyjson.exe` (`go build -o _out\verifyjson.exe .\cmd\verifyjson`).
**Nessun valore personale in questa pagina:** solo nomi di campo, conteggi e classi di errore. I valori stanno solo in `scans\verita\`.
Ultimo aggiornamento: **2026-10-23**.

## 1. Verita' a occhio (`scans\verita\`)

- **75 file** `NOME.json`, uno per immagine del corpus (NOME = nome dell'immagine senza estensione; nessun nome doppio tra le cartelle).
  **60** immagini mostrano una pagina dati leggibile (5 solo in parte: ritaglio senza la riga alta, oppure pagina tagliata in basso con la sola
  riga alta); **15** non ne mostrano (strisce MRZ, pagine interne dell'avviso, copertine): tutti i campi vuoti, con nota. Un campo vuoto e' "non
  stampato, fuori immagine o illeggibile" e non si confronta.
- **9 libretti distinti.** Due sono i facsimili AMP "Anexo 8" 2026, presenti sia come immagine dell'AMP sia come scansioni della loro stampa (il primo in 32
  immagini, perche' e' stato scansionato molte volte con cambi di ritaglio e di driver; il secondo in 3), quattro sono scansioni di altri libretti 2026
  (22 immagini), uno e' `panama.png`, uno e' il facsimile 2020 (QR) e uno il facsimile 2017 (PDF417, oscurato da AMP: solo 6 campi visibili). Di
  conseguenza il corpus e' sbilanciato: delle 35 scansioni distinte con pagina leggibile, 20 sono dello stesso libretto.
- **Metodo.** Ogni pagina e' stata letta con `Read` e, dove la stampa e' piccola o sta sulla filigrana (cargo, luogo di nascita, Id Marino,
  identificazione, certificato), su ritagli ingranditi (Python/PIL su copie nello scratchpad). Il valore e' quello stampato, preso dall'immagine piu'
  nitida dello stesso libretto (facsimile AMP o scansione TWAIN) e riletto sulle altre. Dove l'immagine non lo mostra (ritaglio) il campo e' vuoto
  con nota in `_note`; dove e' solo sfocato ma leggibile (identificazione su `a4-1`) vale il valore noto, con nota.
- **Riscontro indipendente.** Sulle immagini in cui il programma ha letto l'MRZ (**45** con la build delle 10:16, **55** con quella delle 11:15), nascita,
  scadenza e numero libretto della verita' coincidono con l'MRZ ogni volta che il check digit del campo torna: 0 discrepanze, in entrambe le build.
  Gli altri campi non hanno un secondo lettore, ma i valori che il programma legge uguali in
  piu' immagini indipendenti coincidono con la verita'. I due casi di dubbio sono stati ingranditi (identificazione del facsimile 2020 e di `a4-1`).
- **Formato.** I 14 nomi inglesi al livello superiore (come l'uscita di `libretto`); le chiavi che cominciano per `_` sono metadati e le ignora ogni
  lettore: `_tipo` (pagina, documento, documento_180, parziale, striscia_MRZ, ...), `_libretto` (A-I), `_note` (motivo di un campo vuoto o difficile),
  `_riga_nome` (la riga del nome stampata: serve solo a riconoscere un campo che ha letto quella riga). `cmd/regress -truth` legge gli stessi file.
- **Non c'e'** verita' per `libreta_2019_appendice6_pagina-dati` (layout diverso e oscurato: fuori dal modello letto dal programma). Nomi e cognomi
  non sono tra i 14 campi (il programma li prende dall'MRZ).

## 2. Lo strumento: `verifyjson`

```
verifyjson -json FILE.json [-truth VERITA.json | CARTELLA_VERITA] [-strict]
verifyjson -dir CARTELLA_JSON [-truth scans\verita] [-only REGEX] [-exclude REGEX] [-strict] [-v]
```

- **Esiti per campo** (mai i valori): `OK`, `SBAGLIATO` (con la classe di errore), `MANCANTE`, `NON_STAMPATO` (verita' vuota e nulla letto) e
  `IN_PIU` (verita' vuota ma il programma ha letto un valore: dato da guardare, non fa fallire). Per un campo sbagliato o mancante dice anche se
  **l'MRZ trovato lo avrebbe corretto** (`MRZ corregge`, `MRZ non corregge`, `MRZ assente`: il campo sta nell'MRZ ma l'MRZ non c'e' o non e' valido).
- **Confronto normalizzato** (`verify.Equal(field, got, want)`): maiuscole, spazi, trattini, punteggiatura e accenti non contano; le tre date si
  confrontano come date (`GG-MM-AAAA` e ISO sono uguali). Non si ignora altro: `II/2` e `11/2` sono diversi.
- **Classi di errore** (senza valori): `data diversa` (giorno/mese/anno, o giorno e mese scambiati), `caratteri sostituiti` (N), `due caratteri
  scambiati`, `troncato` (N caratteri invece di M), `coda in piu'`, `lunghezza diversa`, `testo non correlato` (piu' del 60% da cambiare),
  `testo della riga del nome` (anche storpiato), `valore di un altro campo` (dice quale). Il dettaglio riporta solo conteggi (caratteri, parole).
- **`-dir`:** una riga per immagine (campi letti, giusti, campi sbagliati e mancanti per nome), poi la tabella di accuratezza per campo, le famiglie
  di errore, le incoerenze piu' frequenti e la tabella **con e senza MRZ valido**. `-exclude "(-grezza|-pagina[0-9]*)$"` tiene solo le scansioni
  distinte. Senza `-truth` usa `scans\verita` se esiste.
- **Uscita:** `0` tutto in ordine; `1` almeno un `SBAGLIATO`, un `MANCANTE` o un'incoerenza `ERRORE` (con `-strict` anche un `AVVISO`); `2` uso
  errato, file illeggibile o JSON che non e' un'uscita di `libretto` (manca `schema_version`).
- **Riga 1 dell'MRZ:** non ha check digit, quindi una lettura storpiata passa inosservata (5 check digit su 5 e nomi sbagliati). `verify.NameMatchesMRZ(surname,
  givenNames, nameRow)` confronta cognome e nomi con la riga stampata (`Truth.NameRow`, da `_riga_nome`): parole senza ordine, accenti e punteggiatura
  ignorati, un prefisso vale solo come troncatura credibile a 39 caratteri. `verifyjson -dir` conta i nomi giusti, sbagliati e senza MRZ.
- **Il campo `certificate` (prima `endorsement`).** Il nome del JSON segue l'etichetta stampata `Nº Título / Certificate` dell'Anexo 8; `panama.png` la stampa
  come `Nº de Refrendo / Endorsement` (altra etichetta, stesso campo). Il vecchio nome `endorsement` si legge ancora come **alias**, sia nell'uscita di
  `libretto` sia nella verita' (`verify.Canonical`, `lookup`, `Truth.Field`): un `libretto.exe` vecchio resta verificabile e una verita' vecchia resta
  valida; qualunque nome sia in `outsum.FieldKeys` i test passano (provato anche con l'elenco vecchio). I report stampano sempre `certificate`.
  **Il campo c'e' solo su alcuni titolari:** visto su 2 libretti su 9, entrambi con una qualifica di grado (cargo con sigla), mai sugli altri 7
  (compresi i facsimili 2017 e 2020). Quindi **assente = `NON_STAMPATO`, non un errore**; la regola di formato `FORMAT_CERTIFICATE` (9 cifre) scatta solo
  quando il campo c'e' ed e' un `AVVISO` (due esemplari non bastano per un errore). Sulle 60 immagini con pagina il programma non da' mai l'avviso
  "campo non letto" dove il certificato non e' stampato (31 su 31), ma non lo da' nemmeno nei 4 casi in cui e' stampato e non letto.
- **API usata da altri:** `verify.LoadTruth`, `verify.Compare(sum, truth)`, `verify.Status*`, `verify.Equal` (le usa `cmd/regress`: non cambiano
  senza avviso). Riusa `outsum.FieldKeys`, `outsum.Parse` e `outsum.NFields` dell'integratore.
- **Test:** `go test ./internal/verify ./cmd/verifyjson`, solo dati sintetici (nomi e numeri inventati), compresa la prova che nessun report
  contiene un valore. `gofmt`, `go vet ./...` e `go test ./...` puliti alla consegna.

### Regole di coerenza (senza verita')

Gravita': `ERRORE` contraddice un formato o un'altra parte dello stesso JSON, `AVVISO` e' un sospetto, `INFO` una nota. Il testo e' fisso e non contiene valori.

| Codice | Gravita' | Cosa controlla |
|---|---|---|
| `FORMAT_TYPE`, `FORMAT_COUNTRY_CODE` | ERRORE | `type` = `SI`, `country_code` = `PAN` |
| `FORMAT_SEAMANS_BOOK` | ERRORE | `PA` + 7 cifre (2026), `P` + 7 cifre (2017 e 2020) |
| `FORMAT_SEAFARERS_ID` | ERRORE | 9 cifre |
| `FORMAT_CERTIFICATE` | AVVISO | 9 cifre, **solo se il campo c'e'** (vedi sotto) |
| `FORMAT_ID_NUMBER` | ERRORE | 5-12 lettere o cifre, con trattini |
| `FORMAT_LEVEL` | ERRORE | `SUPPORT`, `OPERATIONAL` o `MANAGEMENT` |
| `FORMAT_DATE` | ERRORE | data di calendario `GG-MM-AAAA` |
| `DATE_BIRTH_NOT_BEFORE_ISSUE`, `DATE_EXPIRY_NOT_AFTER_ISSUE`, `DATE_EXPIRY_NOT_AFTER_BIRTH` | ERRORE | nascita < emissione < scadenza |
| `DATE_VALIDITY_IMPLAUSIBLE` | AVVISO | validita' fuori da 30 giorni - 5 anni e un mese (osservato: da 5 mesi a 5 anni) |
| `DATE_YEAR_RANGE`, `DATE_AGE_IMPLAUSIBLE`, `DATE_ISO_MISMATCH` | AVVISO | anni plausibili, eta' 16-75 all'emissione, forma ISO uguale a quella stampata |
| `NATIONALITY_LOOKS_LIKE_NAME` | ERRORE | la nazionalita' contiene parole della riga del nome (MRZ), anche storpiate |
| `NATIONALITY_ISO_MISMATCH` | ERRORE | aggettivo di un altro paese rispetto a `nationality_iso` |
| `NATIONALITY_NOT_IN_TABLE` | ERRORE (con ISO) o AVVISO | non e' un aggettivo noto (tabella di 199 codici, copia di `viz_nationality.go`) |
| `DATE_OF_BIRTH_DIFFERS_FROM_MRZ`, `DATE_OF_EXPIRY_DIFFERS_FROM_MRZ`, `SEAMANS_BOOK_DIFFERS_FROM_MRZ` | ERRORE | stampato diverso dall'MRZ |
| `REGIONAL_DIFFERS_FROM_MRZ` | AVVISO | il regionale non comincia come il campo opzionale MRZ (o viceversa: `PANAMA` per `PANAMA DR` e' coerente) |
| `CHAR_NON_PRINTABLE` | ERRORE | caratteri di controllo o simboli non ASCII |
| `CHAR_ACCENT`, `CHAR_SYMBOLS`, `CHAR_LOWERCASE`, `FIELD_SPACES` | AVVISO | lettere accentate spurie, simboli, minuscole, spazi iniziali/finali/doppi |
| `FIELD_TOO_SHORT`, `FIELD_HAS_DIGITS`, `FIELD_NOISE_TOKEN`, `CAPACITY_SUSPICIOUS_TOKEN` | AVVISO | campo troppo corto, cifre dove non vanno, parola di una lettera, cifre fuori da un grado (`II/2`) nel cargo |
| `MRZ_CHECK_DIGITS_INVALID`, `SEX_UNKNOWN_VALUE`, `NATIONALITY_ISO_UNKNOWN`, `MRZ_NOT_FOUND` | AVVISO o INFO | check digit non validi, sesso fuori da M/F/X, codice ISO fuori tabella, MRZ assente |

## 3. Misura su `libretto.exe`

**Build misurata nelle sezioni 3.1-3.4:** `libretto.exe` delle **10:16** (quella in uso quando e' partito il lavoro; copia in `scratchpad\bin`). 76 immagini
del corpus (`scans\`, `scans\vecchi test WIA\`, `facsimili\panama\`), `extract -i` su copie, 75 con verita'. Alle 11:15 l'integratore ha installato una build
nuova: la sezione 3.5 la misura allo stesso modo e la confronta. Nessun valore in questa sezione.

### 3.1 Accuratezza per campo (tutte le 75 immagini: 60 con pagina leggibile, 15 senza)

"Con verita'" = giusti + sbagliati + mancanti (il denominatore: campi stampati e leggibili). "Letti" = campi con un valore scritto dal programma (la
vecchia metrica "campi trovati"). Il programma legge 703 campi e ne azzecca 638: **55 dei 693 campi letti dove la verita' c'e' (8%) sono sbagliati**.

| campo | giusti | sbagliati | mancanti | con verita' | giusti % | letti | non stampati | in piu' |
|---|---|---|---|---|---|---|---|---|
| `type` | 46 | 0 | 13 | 59 | 78% | 47 | 15 | 1 |
| `country_code` | 53 | 0 | 6 | 59 | 90% | 54 | 15 | 1 |
| `nationality` | 51 | 3 | 1 | 55 | 93% | 56 | 18 | 2 |
| `place_of_birth` | 44 | 9 | 2 | 55 | 80% | 55 | 18 | 2 |
| `date_of_issue` | 49 | 2 | 5 | 56 | 88% | 51 | 19 | 0 |
| `date_of_expiry` | 49 | 4 | 3 | 56 | 88% | 53 | 19 | 0 |
| `date_of_birth` | 51 | 0 | 4 | 55 | 93% | 51 | 20 | 0 |
| `level` | 52 | 0 | 4 | 56 | 93% | 52 | 19 | 0 |
| **`capacity`** | **23** | **30** | 3 | 56 | **41%** | 53 | 19 | 0 |
| `seamans_book` | 47 | 0 | 11 | 58 | 81% | 47 | 17 | 0 |
| `id_number` | 48 | 5 | 2 | 55 | 87% | 56 | 17 | 3 |
| `certificate` | 24 | 1 | 4 | 29 | 83% | 25 | 46 | 0 |
| `seafarers_id` | 51 | 0 | 4 | 55 | 93% | 52 | 19 | 1 |
| `regional` | 50 | 1 | 4 | 55 | 91% | 51 | 20 | 0 |
| **totale** | **638** | **55** | **66** | **759** | **84%** | 703 | 281 | 10 |

Il conteggio e' stato rifatto in modo indipendente (script Python con la stessa normalizzazione): stessi numeri, campo per campo.

**Solo scansioni distinte** (senza i `-grezza` e `-pagina`, che ripetono la stessa scansione: 35 con pagina leggibile, 12 senza):
`type` 27/34, `country_code` 31/34, `nationality` 29/32, `place_of_birth` 27/32, `date_of_issue` 28/33, `date_of_expiry` 29/33,
`date_of_birth` 30/32, `level` 31/33, **`capacity` 12/33**, `seamans_book` 28/33, `id_number` 28/32, `certificate` 16/19, `seafarers_id` 29/32,
`regional` 30/32; totale **375/444 (84%)**.

### 3.2 Con e senza MRZ valido (giusti / con verita', tutte le immagini con pagina leggibile)

| campo | con MRZ valido (43 immagini) | senza MRZ valido (17 immagini) |
|---|---|---|
| `type` | 43/43 100% | 3/16 19% |
| `country_code` | 43/43 100% | 10/16 62% |
| `nationality` | 43/43 100% | 8/12 67% |
| `place_of_birth` | 39/43 91% | 5/12 42% |
| `date_of_issue` | 41/43 95% | 8/13 62% |
| `date_of_expiry` | 43/43 100% | 6/13 46% |
| `date_of_birth` | 43/43 100% | 8/12 67% |
| `level` | 43/43 100% | 9/13 69% |
| `capacity` | 17/43 40% | 6/13 46% |
| `seamans_book` | 42/43 98% | 5/15 33% |
| `id_number` | 40/43 93% | 8/12 67% |
| `certificate` | 21/24 88% | 3/5 60% |
| `seafarers_id` | 43/43 100% | 8/12 67% |
| `regional` | 43/43 100% | 7/12 58% |
| **totale** | **544/583 93%** | **94/176 53%** |

Con l'MRZ valido il programma e' al 93% e quasi tutto l'errore e' il cargo; senza, scende al 53%. **Sei campi stanno anche nell'MRZ** (`type`,
`country_code`, `nationality` tramite il codice ISO, `date_of_birth`, `date_of_expiry`, `seamans_book`): dei 121 campi sbagliati o mancanti, **45** sono di
questi sei. **41** sono "MRZ assente" (l'MRZ non e' stato letto o il suo check digit non torna) e **4** avevano un MRZ valido che avrebbe dato il
valore giusto ma il programma non lo ha usato (nascita 1, scadenza 1, numero libretto 2). La verita' coincide con l'MRZ in tutte le immagini con MRZ
letto (sezione 1), quindi riempire questi campi dall'MRZ non ha rischi.

### 3.3 Per immagine (scansioni distinte con pagina leggibile)

`letti` e' la vecchia metrica, `giusti` e' la nuova (giusti / campi con verita'). **Le 15 immagini con 14/14 campi letti hanno tutte almeno un campo
sbagliato** (il cargo, o la data di emissione): le 7 immagini davvero perfette hanno tutte 13/14 "letti", perche' quel libretto non ha il certificato.

| immagine | MRZ | letti | giusti | sbagliati | mancanti |
|---|---|---|---|---|---|
| `DEFINITIVO_spero` | no | 11/14 | 7/14 | nationality, place_of_birth, date_of_expiry, capacity | type, date_of_issue, seafarers_id |
| `ScanTest00000001` | si 5/5 | 14/14 | 12/14 | capacity, certificate |  |
| `a4-1` | si 5/5 | 13/14 | 13/13 |  |  |
| `dimensioni_libretto_panama` | si 5/5 | 13/14 | 11/13 | place_of_birth, capacity |  |
| `libreta_2017_pagina-dati_PDF417_oscurata` | no | 6/14 | 5/6 |  | type |
| `libreta_2020_pagina-dati_QR` | si 5/5 | 13/14 | 11/13 | place_of_birth, id_number |  |
| `libreta_2026_anexo8_pagina-dati_A` | si 5/5 | 14/14 | 12/14 | date_of_issue, capacity |  |
| `libreta_2026_anexo8_pagina-dati_B` | si 5/5 | 13/14 | 13/13 |  |  |
| `libretto-2` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `libretto-vero1` | si 5/5 | 13/14 | 12/13 | id_number |  |
| `libretto-vero2` | si 5/5 | 13/14 | 13/13 |  |  |
| `libretto-vero3` | si 5/5 | 13/14 | 13/13 |  |  |
| `libretto-vero4` | si 5/5 | 13/14 | 13/13 |  |  |
| `libretto-vero5` | no | 11/14 | 10/13 | date_of_expiry | type, seamans_book |
| `libretto-vero6` | si 5/5 | 13/14 | 13/13 |  |  |
| `libretto-vero7` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `librettook1` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `librettook10` | no | 0/14 | 0/3 |  | type, country_code, seamans_book |
| `librettook11` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `librettook4` | si 5/5 | 13/14 | 12/14 | capacity | certificate |
| `librettook7` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `librettook8` | no | 1/14 | 0/3 |  | type, country_code, seamans_book |
| `scana1` | no | 2/14 | 0/11 | nationality, id_number | place_of_birth, date_of_issue, date_of_expiry, date_of_birth, level, capacity, certificate, seafarers_id, regional |
| `scana2` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `panama` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `prova1` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `provamrz` | no | 14/14 | 13/14 | capacity |  |
| `vero` | si 5/5 | 13/14 | 13/13 |  |  |
| `vero12` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `vero13` | si 5/5 | 14/14 | 12/14 | date_of_issue, capacity |  |
| `vero14` | no | 12/14 | 12/14 |  | capacity, seamans_book |
| `vero15` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `vero16` | si 5/5 | 14/14 | 13/14 | capacity |  |
| `vero6` | no | 12/14 | 12/13 |  | type |
| `vero9` | no | 0/14 | 0/13 |  | type, country_code, nationality, place_of_birth, date_of_issue, date_of_expiry, date_of_birth, level, capacity, seamans_book, id_number, seafarers_id, regional |

### 3.4 Stessa scansione, tre inquadrature (ritagliata / grezza con margini bianchi / solo pagina)

Giusti / campi con verita', e se l'MRZ e' valido (MRZ) o no (noMRZ).

| scansione | ritagliata | grezza | pagina |
|---|---|---|---|
| libretto-2 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| libretto-vero1 | 12/13 MRZ | 13/13 MRZ | 12/13 MRZ |
| libretto-vero2 | 13/13 MRZ | **3/13 noMRZ** | 12/13 MRZ |
| libretto-vero3 | 13/13 MRZ | 13/13 MRZ | 12/13 MRZ |
| libretto-vero4 | 13/13 MRZ | **10/13 noMRZ** | 12/13 MRZ |
| libretto-vero5 | 10/13 noMRZ | 9/13 noMRZ | 12/13 noMRZ |
| libretto-vero6 | 13/13 MRZ | 13/13 MRZ | - |
| librettook1 | 13/14 MRZ | **1/14 noMRZ** | 13/14 MRZ |
| librettook4 | 12/14 MRZ | 13/14 MRZ | 12/14 MRZ |
| librettook7 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| librettook11 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| vero | 13/13 MRZ | - | 12/13 MRZ |

Gli stessi pixel con margini bianchi diversi danno risultati molto diversi: 3 grezze su 11 perdono l'MRZ e con lui quasi tutto il resto.

### 3.5 La build delle 11:15 (installata dall'integratore) contro quella delle 10:16

Stessa verita', stesse copie delle immagini; l'eseguibile delle 11:15 (il `libretto.exe` installato dall'integratore, build dei sorgenti di allora) e' stato
copiato in `scratchpad\bin` e misurato come l'altro. Giusti / con verita':

| campo | con verita' | 10:16 | 11:15 |
|---|---|---|---|
| `type` | 59 | 46 | **53** |
| `country_code` | 59 | 53 | **54** |
| `nationality` | 55 | 51 | **52** |
| `place_of_birth` | 55 | 44 | 44 |
| `date_of_issue` | 56 | 49 | 49 |
| `date_of_expiry` | 56 | 49 | **52** |
| `date_of_birth` | 55 | 51 | 51 |
| `level` | 56 | 52 | 52 |
| `capacity` | 56 | 23 | 23 |
| `seamans_book` | 58 | 47 | 47 |
| `id_number` | 55 | 48 | 48 |
| `certificate` | 29 | 24 | 22 |
| `seafarers_id` | 55 | 51 | 51 |
| `regional` | 55 | 50 | 50 |
| **totale** | **759** | **638 (84%)** | **648 (85%)** |

- **MRZ valido** su 53 delle 60 immagini con pagina leggibile (prima 43): +10 immagini. Con MRZ valido l'accuratezza e' 642/716 (90%, prima 93%: le
  10 immagini nuove sono le piu' difficili), senza 6/43 (14%). Sulle 35 scansioni distinte: **381/444 (86%)** (prima 375/444).
- **12 campi migliorati** (`type` 7, `date_of_expiry` 3, `nationality` 1, `country_code` 1), **2 regressioni**: `certificate` da giusto a sbagliato su
  `DEFINITIVO_spero` e `vero14` (secondo l'integratore succede quando l'MRZ si trova e il VIZ cambia modo di lettura). Sbagliati 55 -> 56, mancanti 66 -> 55.
- **Il difetto principale non si e' mosso:** `capacity` 23/56 (con sigla ancora 0 su 29), `place_of_birth` 44/55, `id_number` 48/55, `seamans_book` 47/58.
- **Riga 1 dell'MRZ** (cognome e nomi contro la riga stampata, 59 immagini): 44 giusti prima, **51 giusti e 2 sbagliati** ora (`libretto-vero5` e
  `libretto-vero5-pagina`: riga 1 storpiata con i 5 check digit della riga 2 validi: la riga 1 non ha check digit, il confronto con la riga stampata e' l'unico
  modo di accorgersene; la `-grezza` e' giusta). `verifyjson` e `verify.NameMatchesMRZ` lo contano.
- **Recupero dall'MRZ:** con piu' MRZ validi, **11 campi** sono sbagliati o vuoti pur avendo l'MRZ giusto (`seamans_book` 6, `date_of_birth` 2,
  `date_of_expiry` 2, `nationality` 1; piu' 3 `regional` dove il campo opzionale e' il nome intero), altri 22 lo diventerebbero con l'MRZ letto.
  Riempirli dall'MRZ e' il recupero piu' economico.
- Stessa scansione in tre inquadrature (giusti / con verita'): `libretto-vero2` grezza **3/13 con MRZ valido** (l'MRZ si legge, la zona visiva no:
  lavoro per il VIZ), `libretto-vero4` grezza 11/13, `libretto-vero5` 12/13, 11/13, 13/13; `librettook1` grezza ancora 1/14 senza MRZ.

| scansione | ritagliata | grezza | pagina |
|---|---|---|---|
| libretto-2 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| libretto-vero1 | 12/13 MRZ | 13/13 MRZ | 12/13 MRZ |
| libretto-vero2 | 13/13 MRZ | **3/13 MRZ** | 12/13 MRZ |
| libretto-vero3 | 13/13 MRZ | 13/13 MRZ | 12/13 MRZ |
| libretto-vero4 | 13/13 MRZ | 11/13 MRZ | 12/13 MRZ |
| libretto-vero5 | 12/13 MRZ | 11/13 MRZ | 13/13 MRZ |
| libretto-vero6 | 13/13 MRZ | 13/13 MRZ | - |
| librettook1 | 13/14 MRZ | **1/14 noMRZ** | 13/14 MRZ |
| librettook4 | 12/14 MRZ | 13/14 MRZ | 12/14 MRZ |
| librettook7 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| librettook11 | 13/14 MRZ | 13/14 MRZ | 13/14 MRZ |
| vero | 13/13 MRZ | - | 12/13 MRZ |

## 4. Difetti ricorrenti e priorita' per il VIZ

Ordine: campi sbagliati o mancanti (**difetti**) per campo, tutte le 75 immagini. "MRZ" dice se l'MRZ trovato avrebbe corretto il campo.

| # | Campo | Difetti | Tipo di errore (senza valori) | L'MRZ lo corregge? |
|---|---|---|---|---|
| 1 | **`capacity`** | **33 su 56** (30 sbagliati, 3 mancanti) | **Con una sigla di grado (numero romano + barra + cifra) il cargo e' giusto 0 volte su 29; senza sigla 23 su 27.** Esce un prefisso di 3 lettere invece di una qualifica di due parole (9 casi), un testo breve non correlato (13), una prima parola quasi giusta con lettere spurie o accentate (6), il livello al posto del cargo (2, modo pagina senza MRZ). Su una qualifica di 4 parole le lettere si raddoppiano e gli spazi si perdono (voto per carattere che allinea male letture di lunghezza diversa). A occhio il testo e' nitido (nero spesso): non e' un problema di qualita' dell'immagine | no (non e' nell'MRZ) |
| 2 | `type` | 13 (tutti mancanti) | Non letto quando manca l'MRZ (3 giusti su 16); con l'MRZ viene ricavato da li' (`IS` -> `SI`) e il programma lo segna incerto in 12 casi su 43. `SI` sta sull'ologramma | si, 13 su 13 |
| 3 | `place_of_birth` | 11 (9 + 2) | Testo non correlato di 3-6 lettere (5: il campo sta accanto alla colonna di filigrana `REPUBLICA / REPUBLIC OF PANAMA` e in 3 dei 5 casi il testo letto le assomiglia, a occhio), ultime lettere o sigla del paese perse (2), lettere storpiate (2). 4 su 9 sbagliati senza avviso | no |
| 4 | `seamans_book` | 11 mancanti | 9 senza MRZ, 2 con MRZ che da' il valore giusto: riempire dal campo MRZ quando la lettura stampata manca | si, 11 su 11 |
| 5 | `date_of_expiry` | 7 (4 + 3) | Uguale alla data di emissione (2), mese diverso (2), 3 non lette; 6 su 7 senza MRZ | si, 7 su 7 |
| 6 | `id_number` | 7 (5 + 2) | Una cifra scambiata (9 letto come `S` o `5`: 3 casi), il certificato letto al posto dell'identificazione (1), testo non correlato (1), 2 non letti. Nessuna regola di coerenza lo vede | no |
| 7 | `date_of_issue` | 7 (2 + 5) | Un anno sbagliato di una cifra (6 letto 3, 2 casi: le due "validita' oltre 5 anni" le segnala `DATE_VALIDITY_IMPLAUSIBLE`), 5 non lette | no |
| 8 | `regional` | 5 | 1 lettera sbagliata, 4 mancanti. Tutti senza MRZ | in parte: l'MRZ ha il prefisso (`PANAMA` per `PANAMA DR`) o il nome intero |
| 9 | `country_code` | 6 mancanti | senza MRZ | si |
| 10 | `certificate`, `level`, `seafarers_id`, `date_of_birth`, `nationality` | 5, 4, 4, 4, 4 | mancanti quasi tutti nelle immagini senza MRZ o con pagina tagliata; `nationality` sbagliata 3 volte (testo della riga del nome 2, testo del luogo di nascita 1, tutte senza MRZ) | nascita, nazionalita': si |

**Il programma sa di sbagliare solo in parte.** Dei 55 campi sbagliati, 39 (71%) hanno l'avviso `UNCERTAIN_FIELD` e **16 sono silenziosi** (cargo 5,
luogo di nascita 4, identificazione 2, nazionalita' 2, scadenza 2, emissione 1). L'avviso del cargo e' affidabile (25 su 30 sbagliati, 0 su 23
giusti); quello del certificato no (17 su 24 giusti). Le regole di coerenza di `verifyjson` segnalano 27 dei 55 sbagliati (49%): cargo 20/30,
nazionalita' 3/3, emissione 2/2, luogo 2/9; non vedono scadenza (0/4), identificazione (0/5), certificato e regionale.

**Campi inventati (`IN_PIU`, 10):** una pagina di osservazioni e un certificato letti come pagina dati (4 campi), il numero libretto della riga alta finito in
`id_number` su due pagine tagliate in basso (2: **e `seamans_book` resta vuoto**), un pezzo di riga MRZ letto come identificazione (1), tipo, paese
e nazionalita' ricavati dall'MRZ su una striscia (3: sono derivati, non un errore).

### Cosa chiedo, in ordine

1. **VIZ, cargo con sigla di grado** (`II/2`, `IV/2`): la qualifica e' stampata in nero spesso, ma esce un prefisso. Provare una finestra piu' larga e una
   lettura a parole, e non far prevalere il voto per carattere quando le varianti hanno lunghezze diverse. E' il difetto piu' grosso (33 dei 121 difetti, il 27%). Verificare con `verifyjson -dir ... -exclude "(-grezza|-pagina[0-9]*)$"`: oggi `capacity` e' 12/33.
2. **VIZ / integratore, riempire dall'MRZ i campi che l'MRZ valido ha e la VIZ no**: `seamans_book`, `date_of_birth`, `date_of_expiry`,
   `nationality` (con la tabella ISO) e, se l'MRZ e' valido, `type` e `country_code`. La verita' coincide con l'MRZ in tutte le immagini con MRZ letto:
   nessun rischio: sulla build delle 10:16 sono 4 campi sbagliati o mancanti subito (41 in piu' se l'MRZ viene letto); sulla build delle 11:15
   sono 11 subito (22 in piu' se l'MRZ viene letto).
3. **VIZ, luogo di nascita e identificazione:** mascherare la colonna di filigrana `REPU...` accanto al luogo; l'identificazione scambia 9 con `S` o `5`
   (nei numeri con una sola lettera iniziale e poi solo cifre, una lettera in mezzo alle cifre e' sospetta).
4. **VIZ, pagina tagliata o ritaglio senza riga alta:** con la sola riga alta il numero libretto finisce in `id_number`.
5. **MRZ / integratore, inquadratura:** 3 grezze su 11 perdono l'MRZ con gli stessi pixel (tabella 3.4).
6. **Integratore:** una regola in piu' sulla validita' (emissione piu' di 5 anni e un mese prima della scadenza) coglie le date di emissione sbagliate.

## 5. Limiti e cose da sapere

- Il corpus ha **9 libretti** e 20 delle 35 scansioni distinte sono dello stesso: i giusti per campo non sono una stima per i libretti in generale.
  Mancano libretti con qualifiche diverse (il cargo con sigla e' di soli 2 libretti), altri uffici regionali e altri paesi di nascita.
- `-grezza` e `-pagina` ripetono la stessa scansione: tenerli nella tabella pesa troppo i libretti con piu' versioni (usare `-exclude`).
- L'MRZ e' il secondo lettore solo per sei campi. Gli altri dipendono dalla mia lettura (e dall'accordo con il programma dove coincide).
- `IN_PIU` non e' sempre un errore: la verita' vuota puo' significare "illeggibile".
- **Immagini del corpus senza pagina dati** (verita' tutta vuota, 15): `scana`, `vero10`, `vero11` sono **strisce con le sole righe MRZ** (i 3 campi
  che il programma scrive su `scana` e `vero10` sono derivati dall'MRZ, non c'e' altro da leggere: non sono un lavoro per il VIZ);
  `vero8`, `vero5`, `librettook`, `librettook2`, `librettook6` (e le loro `-grezza`) mostrano solo la pagina interna dell'avviso; le copertine e
  le pagine di osservazioni dei facsimili non hanno campi. `librettook8` e `librettook10` (con le `-grezza`) mostrano solo la riga alta della pagina
  dati: i campi della riga alta (tipo, paese, numero libretto) sono in verita', gli altri vuoti.
- Le verita' non vanno nel repository (`scans/` e' ignorata); se cambiano, la baseline di `cmd/regress` con `-truth` va rigenerata.

## 6. Storico

| Data | Cosa |
|---|---|
| 2026-10-23 | Verita' per 75 immagini, `internal/verify`, `cmd/verifyjson`, misura su `libretto.exe` delle 10:16. Risposto all'integratore: `verify.Equal`, verita' pronte. |
| 2026-10-23 | Il campo `endorsement` diventa **`certificate`** (richiesta dell'utente: i nomi del JSON come le etichette stampate): rinominato in tutti i 75 file di `scans\verita\` (chiavi e `_note`), in `internal/verify` (`FORMAT_CERTIFICATE`, `Canonical`, alias in lettura) e `cmd/verifyjson`; i numeri di questo rapporto non cambiano (stesse misure, nome nuovo). |
