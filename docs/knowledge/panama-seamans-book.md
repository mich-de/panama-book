> Nota sul contenuto: documento di conoscenza (italiano) usato dal progetto. Contiene solo esempi sintetici. Aggiornato con la scoperta sull'ordine dei nomi nell'MRZ di Panama.

# Libreta de embarque (Seaman's Book) di Panama

Documento d'identita' e di servizio del marittimo, emesso dalla **Autoridad Marítima de Panamá (AMP)**, Direzione Generale della Gente de Mar.
Se l'utente intendeva il *passaporto* panamense, questa skill non basta: ha la pagina dati diversa (vedi "Non coperto").

## Cosa e' verificato e cosa no
Fonti: circolare AMP **MMC-331** (dic. 2020, Annex 1 con facsimile) e **DGGM-UCYC-020-2017** (facsimile 2017). Il resto e' marcato *(non verificato)*.

## Formato fisico
- Misure e forma **secondo ICAO** (circolare MMC-331 p.4.1.3): formato libretto **TD3**, 125 x 88 mm.
- Copertina **blu navy** (uguale per ufficiali e comuni), stemma nazionale in **oro**. Sulla copertina, dall'alto:
  `REPÚBLICA DE PANAMÁ / REPUBLIC OF PANAMA` - stemma con arco di stelle e motto *PRO MUNDI BENEFICIO* - `MARINA MERCANTE DE PANAMÁ / MERCHANT MARINE OF PANAMA`.
- Carta di sicurezza dal 2020 (carta di sicurezza o formato elettronico). I libretti gia' emessi restano validi fino alla scadenza.
- Numero di pagine *(non verificato)*.

## Doppia pagina dati (apertura iniziale)
Aperto e ruotato di 90 gradi, come ogni TD3, mostra due fogli:

**1. Pagina interna di copertina (avviso legale)**
- Testo bilingue ES/EN sull'obbligo di tenere il documento a bordo di una nave panamense, con la sanzione prevista.
- Sfondo guilloche con la scritta ripetuta `AUTORIDAD MARÍTIMA DE PANAMÁ - DIRECCIÓN GENERAL DE LA GENTE DE MAR`, stemma in filigrana.
- **Numero di serie** (formato `P` + 7 cifre) ripetuto in verticale, piu' la **perforazione laser** del numero.
- Un riquadro bianco vuoto *(funzione non verificata)*.

**2. Pagina dati** - zona visiva (VIZ) sopra, MRZ in basso.

| Zona | Etichetta stampata (ES/EN) | Esempio del formato |
|---|---|---|
| Intestazione | `REPÚBLICA DE PANAMÁ / REPUBLIC OF PANAMA`; a sinistra `Libreta de embarque / Seaman's Book` | |
| Tipo | `Tipo/Type` | `SI` |
| Stato emittente | `Código del país/Country code` | `PAN` |
| Nome | `Nombres/Given Names`, `Apellidos/Surname` | in maiuscolo, nomi e cognome sulla stessa riga |
| Nazionalita' | `Nacionalidad/Nationality` | **in inglese**, per esteso (es. `NICARAGUAN`), non codice ISO |
| Sesso | `Sexo/Sex` | `M` o `F` |
| Luogo di nascita | `Lugar de nacimiento/Place of birth` | citta' + sigla paese |
| Date | `Fecha de emisión/Date of issue`, `Fecha de expiración/Date of expiry`, `Fecha de nacimiento/Date of birth` | **`DD-MM-YYYY`** |
| Livello | `Nivel/Level` | es. `SUPPORT` |
| Cargo | `Cargo/Capacity` | es. `ORDINARY SEAMAN`, `ENGINE CADET` |
| N. libretto | `Libreta de embarque/Seaman's Book` | `P` + 7 cifre, stesso numero della pagina interna |
| Identificazione | `Identificación/ID number` | `C` + 8 cifre (modello 2020) |
| ID marittimo | `Id Marino/Seafarers Id` | 9 cifre (modello 2020) |
| Regionale | `Regional/Regional` | ufficio regionale emittente |
| Foto + firma | `Firma del titular/Holder's signature` | foto a sinistra, firma sotto |
| Codice | **QR (modello 2020)** oppure **PDF417 (modello 2017)**, a destra o sotto i campi | |

Validita': **variabile**. I facsimili 2017 e 2020 coprono circa 5 anni (08-06-2017 -> 06-06-2022; 02-12-2020 -> 24-11-2025), ma una scansione reale del 2026 ne
copre circa **5 mesi**. Non assumere una durata: leggere sempre la scadenza.

## Differenza tra i due modelli
| | 2017 | dal 2020 |
|---|---|---|
| Codice | **PDF417** largo quasi tutta la pagina, sotto i campi | **QR** in basso a destra |
| Campi | Tipo, paese, nomi, nazionalita', sesso, luogo, date, livello, cargo | in piu' **Id Marino** e **Identificazione**, e `Regional` |
| Altro | | ologramma `PANAMA MARITIME AUTHORITY` sulla pagina dati |

## Variante vista su scan reale 2026 (osservata, un solo esemplare)
Layout uguale al modello 2020 (QR, foto, firma, ologramma, MRZ), con queste differenze:
- **N. libretto** = `PA` + 7 cifre (9 caratteri, senza riempimento `<`), non `P` + 7 cifre. Lo stesso numero e' stampato in verticale sulla pagina interna.
- **Identificazione/ID number**: valore alfanumerico di 9 caratteri (2 lettere + 7 cifre), non `C` + 8 cifre.
- Campo in piu' **solo su alcuni titolari** (negli esempi AMP compare per un ufficiale con qualifica di grado, non per un marittimo di bassa fascia): etichetta stampata **`Nº Título/Certificate`** (9 cifre, tra `Identificación/ID number` e `Fecha de nacimiento/Date of birth`). Verificato sullo zoom del facsimile AMP Anexo 8 (esempio A); l'esempio B non ha il campo. Nel JSON si chiama `certificate`, come l'etichetta inglese; non e' un `Endorsement`/`Refrendo` (nome usato per errore nelle prime versioni).
- **`Regional`** e' il nome dell'ufficio in maiuscolo e compare anche nel campo opzionale dell'MRZ (qui al posto di `PANAMA`).
- **Testo di restrizione** sovrastampato nell'area dell'ologramma, del tipo `ONLY VALID ON BOARD <compagnia>`: non e' un errore di OCR, non va scartato come rumore.
- `Cargo` puo' essere una qualifica lunga su piu' parole (es. operatore GMDSS) e `Nivel` assume valori come `OPERATIONAL`, `SUPPORT`.
- Sullo scan l'MRZ risulta valido ICAO (check digit di numero, nascita, scadenza coerenti con le date della VIZ).

## Orientamento nella scansione
Il libretto aperto puo' arrivare **capovolto di 180 gradi** (pagina dati in alto con il testo a testa in giu', pagina dell'avviso in basso).
Prima dell'OCR rilevare l'orientamento, per esempio cercando le righe MRZ in alto o in basso: sono la traccia piu' affidabile.
Il numero di serie perforato a laser sta in un riquadro bianco della pagina interna e si legge ruotato di 90 gradi.

## MRZ (due righe da 44 caratteri, ICAO TD3)
Il facsimile 2020 ha check digit **validi** secondo ICAO (pesi 7-3-1), quindi la validazione automatica funziona.

```
Riga 1:  IS PAN <COGNOME>< <NOMI> <<<<...      (pos. 1-2 codice, 3-5 stato, 6-44 nome)
Riga 2:  <num libretto 9><cd> <naz 3> <nascita YYMMDD><cd> <sesso> <scadenza YYMMDD><cd> <dati opzionali 14><cd> <cd finale>
```
Esempio **sintetico** (persona e numeri inventati, check digit calcolati):
```
ISPANMARIO<<ROSSI<<<<<<<<<<<<<<<<<<<<<<<<<<<<
P1234567<1ITA9001011M3012316PANAMA<<<<<<<<44
```
Particolarita' osservate nei facsimili:
- **Ordine dei nomi invertito rispetto a ICAO**: nella riga 1 dei libretti di Panama il primo gruppo (prima di `<<`) contiene i **NOMI** e il secondo i **COGNOMI**: `ISPAN` + `NOMI<<COGNOMI` (verificato su modelli 2020 e 2026 e su scan reali), lo stesso ordine della riga stampata `Nombres/Given Names Apellidos/Surname`. Un parser ICAO li scambia: per `IS`+`PAN` invertire. Il campo opzionale della riga 2 e' l'ufficio regionale, non un numero personale. Questo vale anche per un esempio sintetico: costruirlo con i nomi prima.
- **Codice documento `IS`** nell'MRZ, mentre la VIZ stampa `SI` (inversione): non e' un errore di lettura, non "correggerlo".
- Il campo **nazionalita'** dell'MRZ e' quella del titolare (es. `NIC`), **non** `PAN`; `PAN` e' lo stato emittente.
- I **dati opzionali** contengono la parola `PANAMA`.
- Le date MRZ sono `YYMMDD`, quelle della VIZ `DD-MM-YYYY`: usarle per controllarsi a vicenda.

## Altre pagine viste
- `OBSERVACIONES / OBSERVATIONS`: pagina con testo bilingue (rilascio conforme alle leggi di Panama, Convenzione **ILO 108** del 1958 e art. VI **STCW'78**). Per i libretti emessi con il sistema **SAA** il **sigillo ILO 108 / STCW** si applica sull'**ultima pagina** di questa sezione (circolare 2017). Il libretto senza sigillo non e' per forza invalido.
- `CAUSALES DE CANCELACIÓN DEL CARNÉ DE IDENTIFICACIÓN / REASONS FOR THE CANCELLATION OF THE IDENTITY CARD`: elenco a)-h) bilingue su due colonne (ES a sinistra, EN a destra).
- Pagine degli imbarchi (nave, grado, date di imbarco e sbarco) *(non viste, da una fonte non ufficiale)*.

## Autenticita'
Verifica ufficiale: **QR**, ricerca online sul sito AMP/Panama Ship Registry, oppure e-mail `verification@amp.gob.pa`.

## Consigli per scansione e OCR
1. Con lo scanner **fi-800R** (che non ha vassoio piatto) usare il percorso frontale **Return Scan**: il libretto entra e torna fuori dal fronte, fino a 5 mm di spessore, senza foglio porta-documento. **Mai** dal caricatore dall'alto (percorso a U): piega e rovina il libretto. Con altri scanner usare il vassoio piatto.
2. Il fondo guilloche e l'ologramma abbassano il contrasto del testo: provare **300-400 dpi**, a colori, e valutare un filtro che toglie il verde/azzurro del fondo.
3. Leggere prima l'**MRZ** (carattere OCR-B, il piu' affidabile), poi usarlo per validare i campi della VIZ:
   numero libretto, date, sesso, nazionalita'.
4. Non fidarsi solo della VIZ per la nazionalita': e' in inglese per esteso; usare il codice ISO a 3 lettere dell'MRZ.
5. Mettere il QR/PDF417 in un campo a parte: il modello 2017 e 2020 hanno posizioni diverse.

## Non coperto
- Passaporto di Panama (Dirección Nacional de Pasaportes), per il quale PRADO ha `PAN-AO-01001` e `PAN-AO-02001`.
- Libretti di altri Stati e il documento ILO 185 (SID): Panama non risulta averlo ratificato, il suo libretto segue la Convenzione 108.

## Facsimili
Le pagine dati dei facsimili AMP possono mostrare persone reali non oscurate: non vanno pubblicate. Per addestrare un OCR usare dati sintetici.

## Fonti
- AMP, circolare MMC-331 (12/2020), Annex 1: https://www.panamashipregistry.com/wp-content/uploads/2020/12/MMC-331-DGGM-14-12-2020-1.pdf
- AMP, circolare DGGM-UCYC-020-2017 (sigillo ILO 108/STCW): https://www.panamashipregistry.com/wp-content/uploads/2017/01/Circular-DGGM-UCYC-020-2017.pdf
- ICAO Doc 9303 parte 4 (TD3 e MRZ).
