# Campi della pagina dati (Libreta de embarque, Panama)

Elenco dei dati che `libretto` estrae da una scansione di una facciata. Fonte del layout: skill `libretto-panama`
(circolari AMP MMC-331/2020 e DGGM-UCYC-020/2017, facsimile 2026 "Anexo 8").
Tutti gli esempi sono **inventati**. Allineato al codice del 2026-10-22 (sera): `cmd/libretto/output.go` (nomi e avvisi in uscita),
`internal/libreta/viz*.go`, `extract.go`.

## Regole generali

1. **MRZ prevale sulla zona visiva (VIZ).** L'MRZ ha i check digit, l'OCR della VIZ no. Dove un dato e' presente in entrambe,
   si tiene quello dell'MRZ e la differenza diventa un avviso (`warnings`).
2. **Check digit ICAO 9303, pesi 7-3-1.** Cifre = valore, lettere A-Z = 10-35, `<` = 0; somma dei prodotti, modulo 10.
   Cinque controlli sulla riga 2: numero libretto (pos. 10), nascita (20), scadenza (28), dati opzionali (43), controllo finale (44).
   Nel campo opzionale tutto `<` il check digit puo' essere `<` o `0`.
3. **Codice documento `IS` nell'MRZ, `SI` nella VIZ.** Il campo `Tipo/Type` stampato mostra `SI` (inversione). Non e' un errore di OCR:
   non "correggere" nessuno dei due. Il codice legge `SI` dalla VIZ (un valore storpiato vicino a `SI` viene riportato a `SI`) e tiene
   `IS` in `mrz.doc_code`; i due non vengono confrontati. **Se `type` non si legge** (sta su un ologramma) il programma lo ricava
   dall'MRZ invertendo le due lettere (`IS` -> `SI`) e aggiunge l'avviso `UNCERTAIN_FIELD` con `field` = `type`.
4. **Nazionalita' = quella del titolare, non `PAN`.** `PAN` e' lo stato emittente (riga 1, pos. 3-5, `mrz.issuer`). Nella riga 2 il codice
   a 3 lettere (`mrz.nationality`, in `fields` come `nationality_iso`) e' la nazionalita' del marittimo (es. `ITA`). La VIZ la stampa
   **in inglese per esteso** (es. `ITALIAN`, campo `nationality`). Con l'MRZ presente il valore stampato e' riallineato all'aggettivo atteso per
   il codice ISO (tabella di 199 codici in `viz_nationality.go`) se la lettura OCR gli somiglia (>= 0,5).
5. **Formati data.** MRZ: `YYMMDD`. VIZ: `DD-MM-YYYY`. Nell'uscita JSON: le date MRZ sono `YYYY-MM-DD`; le date stampate hanno la forma
   stampata (`date_of_birth`) e la forma ISO (`date_of_birth_iso`). Per la nascita nell'MRZ un anno nel futuro e' del secolo scorso; la
   scadenza MRZ e' sempre letta nel 2000. Il confronto VIZ/MRZ e' fatto sulle ultime 8 cifre (`AA-MM-GG`), perche' l'MRZ ha solo due cifre per l'anno.
6. **Validita' variabile.** Facsimili 2017/2020: circa 5 anni. Uno scan reale del 2026: circa **5 mesi**. Non dedurre mai
   la scadenza dall'emissione: leggere sempre la data. Il programma non scarta un libretto "scaduto": riporta le date.
7. **Modelli.** 2017: barcode PDF417, niente Id Marino/Identificazione/Regional. 2020: QR, in piu' Id Marino, Identificazione e
   Regional. 2026: come il 2020, con numero libretto `PA`+7 cifre, Identificazione di 2 lettere + 7 cifre e il campo in piu'
   `Nº Título/Certificate` (9 cifre, tra Identificación/ID number e Fecha de nacimiento/Date of birth). Un campo assente nel modello letto resta vuoto, non e' un errore (ma vedi gli avvisi sotto: per il
   modello 2017 il programma segnala comunque `PRINTED_FIELDS_NOT_READ`).
8. **Chiavi in inglese.** L'uscita JSON ha `schema_version` 1 e nomi di campo **in inglese, uguali alle etichette stampate** (snake_case).
   Le chiavi sotto sono quelle della colonna "Chiave JSON" (dentro `fields`). Le righe MRZ complete e l'esito dei check
   digit stanno nell'oggetto `mrz`. Un campo non letto e' **assente** dal JSON.
9. **Lettura della VIZ.** Ogni campo e' letto da piu' ritagli (3 varianti di scala e finestra, ognuno sul "foglio" e come immagine singola, piu'
   varianti binarizzate e ritagli "jitter" dei testi liberi sulle scansioni a tratto spesso); vince il valore su cui concorda la maggioranza
   (voto per carattere, pesi misurati sulle cifre note dall'MRZ), a parita' la variante di riferimento. I formati sotto sono quelli
   riconosciuti dal codice: cifre confuse con lettere (O/0, I/1, ...) sono corrette dove il formato richiede una cifra.

## Tabella

| Chiave JSON | Etichetta stampata (ES / EN) | Formato | Fonte | Validazione |
|---|---|---|---|---|
| `type` | `Tipo / Type` | 2 lettere, `SI` | VIZ (MRZ: `IS`, in `mrz.doc_code`) | letto con il paese sulla stessa riga; valore vicino a `SI` (similarita' >= 0,5) riportato a `SI`; se non si legge e' ricavato dall'MRZ (`IS` -> `SI`) con avviso `UNCERTAIN_FIELD`; nessun confronto con l'MRZ |
| `country_code` | `Código del país / Country code` | 3 lettere, `PAN` | VIZ (MRZ: `mrz.issuer`, riga 1 pos. 3-5) | valore vicino a `PAN` (>= 0,6) riportato a `PAN`; il codice **non** lo confronta con `mrz.issuer` |
| `surname` | `Apellidos / Surname` | maiuscolo, senza accenti | **solo MRZ** (riga 1, prima di `<<`) | non letto dalla VIZ. Il riempimento `<` diventa spazio |
| `given_names` | `Nombres / Given Names` | maiuscolo, nomi separati da spazio | **solo MRZ** (riga 1, dopo `<<`) | non letto dalla VIZ. L'MRZ non ha accenti ne' apostrofi: la VIZ puo' averli |
| `nationality` | `Nacionalidad / Nationality` | inglese per esteso (`ITALIAN`) | VIZ | riallineato all'aggettivo atteso per `nationality_iso` (tabella ISO, 199 codici, similarita' >= 0,5); senza codice noto, vicino a un aggettivo della tabella (>= 0,8) riportato a quello; avviso se il paese e' in tabella (>= 0,75) e il codice e' diverso. Non ricavabile dal solo MRZ |
| `nationality_iso` | (nessuna: solo MRZ) | 3 lettere ISO 3166-1 alpha-3 / ICAO | MRZ (riga 2, pos. 11-13) | lettere (non cifre, riparate dalla classe di posizione); titolare non `PAN` per i libretti degli stranieri |
| `sex` | `Sexo / Sex` | `M` o `F` (MRZ ammette anche `<`) | MRZ (riga 2, pos. 21) | MRZ prevale; il sesso stampato (`M`, `F`, `X`) e' letto solo per il confronto, non compare nel JSON |
| `place_of_birth` | `Lugar de nacimiento / Place of birth` | citta' + sigla paese, maiuscolo | VIZ | nessun check; normalizzazione (maiuscolo, simboli spuri scartati, spazi singoli); accettato se ha almeno 2 lettere. Campo instabile (testo libero su filigrana): letto anche con i ritagli jitter |
| `date_of_issue` (+ `_iso`) | `Fecha de emisión / Date of issue` | `DD-MM-YYYY` e `YYYY-MM-DD` | VIZ | data di calendario valida con anno tra 2000 e anno corrente + 1; deve precedere la scadenza; non c'e' nell'MRZ |
| `date_of_expiry` (+ `_iso`, `_mrz`) | `Fecha de expiración / Date of expiry` | VIZ `DD-MM-YYYY`; MRZ `YYMMDD` | MRZ (`date_of_expiry_mrz`) e VIZ (`date_of_expiry`) | check digit pos. 28; VIZ con anno tra 2000 e anno corrente + 15, deve coincidere con l'MRZ (avviso se diversa) |
| `date_of_birth` (+ `_iso`, `_mrz`) | `Fecha de nacimiento / Date of birth` | VIZ `DD-MM-YYYY`; MRZ `YYMMDD` | MRZ (`date_of_birth_mrz`) e VIZ (`date_of_birth`) | check digit pos. 20; VIZ con anno tra 1920 e (anno corrente - 14), deve coincidere con l'MRZ e precedere l'emissione |
| `level` | `Nivel / Level` | maiuscolo | VIZ | solo `SUPPORT`, `OPERATIONAL` o `MANAGEMENT` (similarita' >= 0,65); altri valori non sono riconosciuti |
| `capacity` | `Cargo / Capacity` | maiuscolo, puo' avere piu' parole (`ORDINARY SEAMAN`, `ENGINE CADET`) | VIZ | nessun check; accettato se ha almeno 2 lettere; allineato a un elenco chiuso di qualifiche. Campo instabile (la sigla `II/2` non e' letta dall'OCR): letto anche con i ritagli jitter |
| `seamans_book` (+ `_mrz`) | `Libreta de embarque / Seaman's Book` | `P` + 7 cifre (modelli 2017/2020), `PA` + 7 cifre (2026) | MRZ (riga 2, pos. 1-9, `seamans_book_mrz`) e VIZ | check digit pos. 10; la VIZ deve coincidere con l'MRZ (avviso se diversa); letto anche con le sole 7 cifre e ricomposto col prefisso; stesso numero stampato in verticale nella pagina interna |
| `id_number` | `Identificación / ID number` | `C` + 8 cifre (2020); 2 lettere + 7 cifre (2026) | VIZ | nessun check (non e' nell'MRZ); formato: 1-2 lettere + 7-8 cifre (almeno 5 cifre), oppure sequenza con almeno 6 cifre |
| `certificate` | `Nº Título / Certificate` | 9 cifre | VIZ, **solo modello 2026** | fino al 2026-10-23 si chiamava `endorsement` (etichetta creduta "Nº de Refrendo / Endorsement"; sul facsimile 2026 Anexo 8, esempio A, e' stampato `Nº Título/Certificate`; l'esempio B non ha il campo). Formato: 9 caratteri con almeno 6 cifre; assente negli altri modelli (l'avviso di campo non letto e' dato solo se il numero libretto inizia per `PA` e l'etichetta e' stata vista) |
| `seafarers_id` | `Id Marino / Seafarers Id` | 9 cifre | VIZ (modelli 2020 e 2026) | formato: 9 caratteri con almeno 6 cifre |
| `regional` | `Regional / Regional` | nome ufficio, maiuscolo | VIZ (e `regional_mrz`: campo opzionale MRZ) | confronto approssimato col campo opzionale MRZ (similarita' < 0,7 = avviso). Nel 2020 l'opzionale dell'MRZ e' `PANAMA`; nel 2026 vi compare il nome dell'ufficio. **Da verificare**: se i due testi differiscono per legittimo motivo l'avviso e' spurio |

Il campo opzionale dell'MRZ (riga 2, pos. 29-42, 14 caratteri) ha il suo check digit (pos. 43).

## Campi derivati dall'MRZ nell'uscita

Oltre ai campi stampati, `fields` contiene i valori letti dall'MRZ, perche' sono i piu' affidabili (e sono presenti solo se l'MRZ e' stato trovato):
`surname`, `given_names`, `nationality_iso`, `sex`, `date_of_birth_mrz`, `date_of_expiry_mrz`, `seamans_book_mrz`, `regional_mrz`.
Le chiavi `date_of_*_iso` sono la forma ISO delle tre date stampate (emissione, scadenza, nascita).

## Corrispondenza con le vecchie chiavi (prima di `schema_version` 1)

Nelle prime versioni (senza `schema_version`) le chiavi erano in italiano/spagnolo. Chi ha gia' salvato JSON di allora li ritrova cosi':

| Vecchia | Nuova | | Vecchia | Nuova |
|---|---|---|---|---|
| `tipo` | `type` | | `data_nascita` (+ `_iso`, `_mrz`) | `date_of_birth` (+ `_iso`, `_mrz`) |
| `paese` | `country_code` | | `livello` | `level` |
| `nacionalidad` | `nationality` | | `cargo` | `capacity` |
| `luogo_nascita` | `place_of_birth` | | `numero_libretto` (+ `_mrz`) | `seamans_book` (+ `_mrz`) |
| `data_emissione` (+ `_iso`) | `date_of_issue` (+ `_iso`) | | `identificazione` | `id_number` |
| `data_scadenza` (+ `_iso`, `_mrz`) | `date_of_expiry` (+ `_iso`, `_mrz`) | | `refrendo` | `certificate` (fino al 2026-10-23 `endorsement`) |
| `cognome` | `surname` | | `id_marino` | `seafarers_id` |
| `nomi` | `given_names` | | `regionale` (+ `_mrz`) | `regional` (+ `_mrz`) |
| `nazionalita_iso` | `nationality_iso` | | `sesso` | `sex` |

**Ordine dei nomi nei libretti di Panama (dal 2026-10-23):** nella riga 1 dell'MRZ dei libretti di Panama (codice `IS`, stato `PAN`; verificato sui modelli 2020 e 2026 e su scansioni reali) i NOMI vengono PRIMA del cognome (`ISPAN` + nomi`<<`cognomi), all'opposto di ICAO. Il parser legge in ordine ICAO, quindi `cmd/libretto/output.go` (`toOutput`) scambia `surname` e `given_names` (e i due gruppi del blocco `mrz`) per `IS`+`PAN`; se c'e' un solo gruppo non scambia. Il dato interno (`Result`, `mrz.Data`) resta in ordine ICAO. Nell'uscita JSON `surname` e `given_names` sono quindi corretti. `libretto rename -json scans\NOME.json [-n]` rinomina immagine, JSON, PDF e log in `NOME - COGNOME - NUMERO LIBRETTO` (`internal/filename`: solo lettere e cifre ASCII e spazi, accenti traslitterati, `< > / \ : * ? " | ' .` diventano spazi, al massimo 45 caratteri per pezzo, `nome (2)` se esiste gia'), solo se l'MRZ ha tutti i check digit e non c'e' `MRZ_LINE1_UNCERTAIN`; tutto o niente, con ripristino.

**Cambio di nome del 2026-10-23:** il campo `endorsement` si chiama `certificate` (etichetta stampata `Nº Título / Certificate`). Le uscite nuove hanno solo `certificate`; `internal/outsum` (quindi `libretto summary` e l'harness di regressione) legge ancora le uscite salvate con il nome vecchio (alias in lettura, transitorio). `schema_version` resta 1: non c'e' ancora nessun lettore esterno del JSON.

Anche `mrz_corrections` e' in inglese (`riga2 pos 14: S→5` diventa `line2 pos 14: S->5`) e `warnings` non e' piu' un elenco di frasi italiane ma di oggetti con codice.
Le correzioni comprendono anche le frasi del lettore a template (`line1 pos 22: uncertain reading (O)`) e quelle della seconda lettura della riga 1 e della riga 2 senza check digit (`line1: surname and given names taken from the template reader ...`, `line2: nationality taken from ...`, `line2: sex taken from ...`): quando l'OCR e il lettore a template differiscono in quelle parti e il lettore e' sicuro delle celle diverse, si prende la sua lettura e la correzione lo dice.

## Avvisi (`warnings`)

Ogni avviso e' un oggetto `{ "code": ..., "field": ... }` o `{ "code": ..., "fields": [...] }`: codice stabile, senza testo libero e senza valori personali. I nomi in `field`/`fields`
sono quelli di `fields`. Con `warnings` non vuoto i dati vanno verificati a mano.

| `code` | Extra | Quando |
|---|---|---|
| `MRZ_NOT_FOUND` | | MRZ non trovato: i dati stampati non sono verificabili |
| `MRZ_CHECK_DIGITS_INVALID` | | uno o piu' check digit MRZ non tornano |
| `PRINTED_FIELDS_NOT_READ` | `fields` (ordine alfabetico) | campi stampati non letti. Il `certificate` e' atteso solo se il numero libretto inizia per `PA`. **Per il modello 2017 compare sempre**, perche' Id Marino, Identificazione e Regional sono sempre attesi |
| `MRZ_LINE1_UNCERTAIN` | | la riga 1 dell'MRZ (cognome e nomi, senza check digit) letta dall'OCR e dal lettore a template differisce, e il lettore a template non e' sicuro delle celle diverse: resta la lettura OCR, da controllare a occhio |
| `MRZ_FIELD_UNCERTAIN` | `field`: `nationality_iso`, `sex` | come sopra per la nazionalita' o il sesso della riga 2, che i check digit non coprono |
| `MRZ_NAME_DIFFERS_FROM_PRINTED` | | il nome stampato ("Nombres/Given Names Apellidos/Surname"), letto con confidenza almeno 0,9, ha parole diverse da cognome e nomi dell'MRZ (si confrontano le parole, senza ordine e senza accenti, perche' a Panama i nomi precedono il cognome). Nessun valore nell'avviso. Si controlla perche' la riga 1 dell'MRZ non ha check digit. Non si emette se il campo nomi dell'MRZ e' tutto occupato (possibile troncamento) o se il nome stampato non e' stato letto. Sul corpus di prova la confidenza dei nomi e' quasi sempre sotto 0,9: l'avviso e' prudente e di fatto raro |
| `UNCERTAIN_FIELD` | `field` | letture scarse o discordanti tra le varianti; oppure `type` ricavato dall'MRZ |
| `PRINTED_FIELD_DIFFERS_FROM_MRZ` | `field`: `date_of_birth`, `date_of_expiry`, `seamans_book`, `sex`, `nationality`, `regional` | il valore stampato non coincide con l'MRZ |
| `DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE` | | scadenza stampata non successiva all'emissione |
| `DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE` | | nascita stampata non precedente all'emissione |
| `UNKNOWN_WARNING` | | testo interno che `output.go` non sa tradurre. **Oggi ci finisce anche** "data stampata non interpretabile come data" (il campo viene tolto dal JSON): manca un codice dedicato |

## Non e' un campo da leggere

- **Barcode** (QR o PDF417): non decodificato dal programma.
- **Testo di restrizione** sovrastampato nell'area dell'ologramma (`ONLY VALID ON BOARD <compagnia>`) nel 2026: non e' rumore, ma non e' un
  campo estratto.
- **Foto e firma** (`Firma del titular / Holder's signature`).

## Esempio inventato (MRZ coerente, check digit calcolati)

```
ISPANROSSI<<MARIO<<<<<<<<<<<<<<<<<<<<<<<<<<<
P1234567<1ITA9001011M3012316PANAMA<<<<<<<<44
```

I cinque check digit di questo esempio (`1`, `1`, `6`, `4`, `4`) sono stati ricalcolati con i pesi 7-3-1; sono quelli del test sintetico
del pacchetto `internal/mrz`. L'esempio completo dell'uscita JSON e' nel `README.md`.
