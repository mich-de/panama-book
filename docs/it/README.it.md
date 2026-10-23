> **Documento storico (2026-10-22, in italiano).** Parte dello stato qui descritto e' superato (es. WIA/TWAIN, OCR, GUI). Per lo stato attuale vedi `README.md` e `AI_CONTEXT.md` nella radice.

# libretto

Programma da riga di comando che **scansiona una sola facciata** di una *Libreta de embarque* (Seaman's Book) di Panama con lo scanner
**Fujitsu/PFU/Ricoh fi-800R** e ne estrae i dati: la **MRZ ICAO TD3** (con check digit) e i **14 campi stampati** della pagina dati.
Uscita: un JSON su standard output, con nomi di campo in inglese uguali alle etichette stampate sul documento (`schema_version` 1).

Module Go: `scanlibretto`. Solo Windows. Due modi per pilotare lo scanner: **WIA** (COM, predefinito oggi) e **TWAIN** (driver PaperStream IP,
tramite un programma ausiliario a 32 bit). OCR integrato di Windows (`Windows.Media.Ocr`), chiamato direttamente dal programma (backend nativo; ripiego su PowerShell).

Stato (2026-10-22, sera):

- MRZ e campi stampati verificati su facsimili AMP e su scansioni reali (stampe e libretti) in `scans\vecchi test WIA\`.
- Con WIA il driver **ignora l'inizio dell'area** (`-from`): per un libretto si legge quindi un tratto lungo dal bordo (190 mm, vedi "Prove con lo scanner").
  Misure sul libretto vero in `docs\agenti\03-wia.md`, sezione "Misure sul libretto vero".
- Con TWAIN il driver **accetta l'alto del riquadro** (`ICAP_FRAMES`, verificato con `twainprobe32.exe -try-frame`, senza scansione). **Non e' ancora
  stato provato con un libretto vero** se il motore salta davvero i primi mm. Il driver predefinito diventera' `twain` solo dopo quella prova.
- Non verificato: TWAIN con documento, `-width`, effetto di *Document Handling Select = 1* sul percorso fisico, libretti di modelli diversi dal 2026.

## Requisiti

- Windows 10 o 11 (non ARM: il fi-800R non e' supportato da PFU su Windows 11 ARM).
- **Go 1.22 o superiore** per compilare (`go.mod` dichiara `go 1.22`; il codice usa i builtin `min`/`max`, da Go 1.21). Il toolchain locale e' 1.27: il requisito 1.22 non e' mai stato verificato.
- **Driver PaperStream IP (TWAIN) 3.40.2 del fi-800R** installato, con lo scanner riconosciuto da Windows.
  Vedi `drivers\LEGGIMI.md` e `drivers\PaperStream_IP_TWAIN_3.40.2_solo-driver\Installa-Solo-Driver.cmd`
  (installa solo il driver; chiede i diritti di amministratore; poi ricollegare il cavo USB).
  In alternativa, **senza installer**: `drivers\installazione_manuale\` contiene i file originali INF/CAT/CAB del solo driver USB/WIA
  (versione 2.0.0.5; `fujitsu\` e' quello per questo scanner, `ricoh\` solo di riferimento) e si installa con `pnputil` o da Gestione
  dispositivi (istruzioni in `drivers\installazione_manuale\LEGGIMI.md`; non verificato su un PC senza PaperStream IP).
  Per `-driver twain` serve il driver PaperStream IP, che e' **solo a 32 bit** (data source in `C:\Windows\twain_32\Fjicube`, DSM in `SysWOW64`).
- **Servizio WIA attivo** ("Acquisizione di immagini di Windows", `stisvc`) per `-driver wia` (predefinito).
- **`twainscan32.exe`** (programma a 32 bit) accanto a `libretto.exe`, nella cartella corrente o in `_out\`, solo per `-driver twain`.
- **Motore OCR di Windows** con la lingua del profilo utente (`OcrEngine.TryCreateFromUserProfileLanguages`): serve almeno una lingua
  con il riconoscimento del testo installato (Impostazioni > Ora e lingua > Lingua e area geografica). Senza, il programma
  fallisce con "Nessun motore OCR disponibile per le lingue del profilo utente".
- Nessuna dipendenza esterna a runtime: l'OCR e' nativo (nessun processo `powershell.exe`). `powershell.exe` (Windows PowerShell 5.1) serve solo come ripiego se l'OCR nativo non parte, o se lo si forza con la variabile d'ambiente `LIBRETTO_OCR=powershell`; `LIBRETTO_OCR=native` esclude il ripiego. In compilazione `go-ole` e `golang.org/x/sys`.

## Compilazione e test

```powershell
cd C:\path\to\panama-book
go build -o libretto.exe ./cmd/libretto
$env:GOARCH='386'; go build -o twainscan32.exe ./cmd/twainscan; go build -o _out\twainprobe32.exe ./cmd/twainprobe; Remove-Item Env:GOARCH
go vet ./...                                    # anche: $env:GOARCH='386'; go vet ./...  (pulito il 2026-10-22)
go test ./...                                   # test unitari (176 + 1 saltato): non usano ne' scanner ne' OCR
go test -tags integration -v ./internal/libreta ./internal/imgproc # integrazione (10): serve l'OCR di Windows (non il ritaglio) e le immagini in facsimili\panama e scans
```

- Copertura dei test unitari (2026-10-22): `mrz` 91%, `imgproc` 78%, `twain` 74%, `libreta` 71%, `wia` 35%, `cmd/libretto` 14%, `ocr`, `cmd/twainscan` e `cmd/twainprobe` senza test.
- I test di integrazione (file `*_integration_test.go`, build tag `integration`) leggono le immagini di `facsimili\panama` e di `scans\`
  (anche `scans\vecchi test WIA\`, ma **non tutti** cercano nella sottocartella: vedi `REGISTRO.md`; i test saltano se manca il file),
  avviano PowerShell per l'OCR (tranne il test del ritaglio della pagina, in `imgproc`) e scrivono in `_out\viz_t`. Riportano **solo conteggi**, mai i valori dei campi
  (sono dati personali). Per ispezionare i valori in locale: `VIZ_SHOW=1`; per confrontare con una verita' di terreno tenuta fuori dal
  progetto: `VIZ_TRUTH=percorso.json` (vengono riportati solo i nomi dei campi sbagliati).
- Nessun test tocca lo scanner: `wia.Scan`, `wia.Describe` e `twain.Scan` non hanno test automatici sul dispositivo (i test TWAIN usano un data source finto).

## Uso

Cinque comandi (piu' `summary risultato.json`, che riassume in una riga un JSON gia' prodotto). Il PNG di scansione e' la sola cosa scritta su disco (piu' le cartelle di debug, il file di `-page-out` e quello di `-raw`, se richiesti).

```powershell
# 1. diagnostica dello scanner WIA: non scansiona, non serve documento
.\libretto.exe info
.\libretto.exe info -device fi-800R

# 2. scansiona una facciata (chiede conferma), scarta le scansioni vuote, ritaglia al documento e salva un PNG
.\libretto.exe scan                               # salva scans\scan-AAAAMMGG-hhmmss.png
.\libretto.exe scan -o scan.png -dpi 400 -gray
.\libretto.exe scan -o scan.png -len 190          # legge solo i primi 190 mm dal bordo d'ingresso (libretto aperto)

# 3. estrae i dati da un'immagine gia' acquisita (PNG o JPEG): utile per riprovare senza riscansionare
.\libretto.exe extract -i scan.png
.\libretto.exe extract -i scan.png -page-out pagina.png -debug .\_out\prova1

# 4. scansiona ed estrae in un colpo solo (JSON su stdout, messaggi su stderr)
.\libretto.exe read -o scan.png -dpi 300
.\libretto.exe read -o scan.png > risultato.json         # la conferma va su stderr/stdin: resta interattiva
.\libretto.exe read -yes -o scan.png > risultato.json    # negli script (stdin non interattivo) serve -yes
.\libretto.exe read -yes -o pagina.png -len 190 -page-only > risultato.json   # il file e' la sola pagina dati 125x88 mm
.\libretto.exe read -driver twain -yes -o pagina.png -from 90 -len 100 -page-only > risultato.json   # TWAIN: da provare

# 5. PDF di una pagina entro 1 MB (il gestionale accetta allegati fino a 1 MB): non usa lo scanner
.\libretto.exe pdf -i scans\scan.png                       # crea scans\scan.pdf (rifiuta se esiste: serve -force)
.\libretto.exe pdf -i scan.png -o scan.pdf -max 1000000 -dpi 300 -force
```

`pdf` incorpora l'immagine come JPEG senza altre ricodifiche e sceglie la qualita' piu' alta (95-80, ricerca binaria) che tiene il PDF entro `-max`; se anche a 80 non basta riduce l'immagine a passi del 7% (mai sotto la meta'). Il PDF **contiene dati personali** (il programma lo ricorda su stderr). Per l'utente c'e' anche `tools\pdf.cmd NOME`.

Opzioni:

| Opzione | Comandi | Significato |
|---|---|---|
| `-device` | info, scan, read | parte del nome dello scanner (default `fi-800R`) |
| `-driver wia\|twain` | scan, read | `wia` (predefinito) o `twain`. Con `twain` l'inizio dell'area (`-from`) e' rispettato dal driver (da verificare con documento); serve `twainscan32.exe` (programma a 32 bit, lanciato da `libretto`) |
| `-dpi` | scan, read | 75-600, default 300 |
| `-gray` | scan, read | scala di grigi (default colore) |
| `-o` | scan, read | PNG di uscita; default `scans\scan-AAAAMMGG-hhmmss.png` (un nome diverso a ogni scansione); la cartella si crea se manca |
| `-from N` | scan, read | inizia a leggere dopo N mm dal bordo d'ingresso. **Il driver WIA lo accetta ma lo ignora** (misurato: spostamento 0,0 mm); con TWAIN il driver accetta l'alto del riquadro |
| `-len N` | scan, read | lunghezza letta in mm a partire da `-from` (0 = tutto il campo, 356 mm). Con WIA funziona: `-len 100` da' 2550x1181 px a 300 dpi |
| `-width N` | scan, read | legge solo N mm di larghezza, centrati sull'apertura (0 = 216 mm). **Mai provato con documento**; se il driver ignorasse anche l'inizio orizzontale taglierebbe la parte destra |
| `-raw file.png` | scan, read | salva anche la scansione **grezza**, prima di ritaglio e controllo del vuoto (serve a misurare dove cade il documento); **contiene dati personali** |
| `-yes` | scan, read | non chiede conferma prima di avviare lo scanner |
| `-force` | scan, read, extract, pdf | consente di **sostituire** il file di uscita (`-o`, `-page-out`, `-raw`) se esiste; senza, un file esistente di `-o` e' rifiutato **prima** di usare lo scanner |
| `-page-only` | read | il file di uscita (`-o`) contiene **solo la pagina dati ritagliata** (125x88 mm), non la scansione intera; se le righe MRZ non si trovano resta la scansione intera. `scan` accetta il flag ma lo ignora. **Contiene dati personali** |
| `-i` | extract, pdf | immagine da leggere (PNG o JPEG) |
| `-o file.pdf` | pdf | PDF di uscita; default: come l'immagine, con estensione `.pdf`; un file esistente e' rifiutato senza `-force`; **contiene dati personali** |
| `-max N` | pdf | peso massimo del PDF in byte (default 1.000.000, per stare sotto 1 MB anche se il gestionale conta in megabyte decimali; 0 = nessun limite, qualita' 92) |
| `-dpi N` | pdf | risoluzione dell'immagine: decide la misura fisica della pagina (default 300) |
| `-page-out file.png` | extract, read | salva la sola **pagina dati ritagliata** (125x88 mm, trovata dalla posizione dell'MRZ) in un file a parte; **contiene dati personali** |
| `-debug cartella` | extract, read | salva le immagini intermedie e il testo OCR: **contengono dati personali**; all'avvio il programma lo ricorda con un avviso su stderr |

Esiste anche il sottocomando nascosto `libretto area-probe` (solo proprieta' WIA dell'item, nessuna acquisizione), usato per studiare i limiti del driver.
Il tempo massimo della scansione WIA e' 2 minuti e non si imposta da riga di comando.

**Regole di sicurezza del CLI** (vedi anche "Sicurezza"):

- **Conferma.** Prima di avviare lo scanner `scan` e `read` chiedono "Documento inserito? Premi Invio per scansionare". Senza un terminale
  interattivo (input rediretto, script) **rifiutano di procedere**: serve `-yes`. Con `-driver twain` la conferma resta a `libretto`: il programma ausiliario e' lanciato con `-yes`.
- **Nessuna sovrascrittura.** Se il file di `-o` esiste, `scan` e `read` si fermano subito, senza toccare lo scanner; si procede solo con `-force`.
  Un file di `-page-out` o `-raw` esistente non e' sostituito senza `-force` (sono accessori: il JSON esce comunque e si stampa un avviso).
- **Ctrl+C.** `extract` e `read` raccolgono Ctrl+C in un contesto annullabile e, se interrotti, escono con codice 130. **Non verificato:** la
  scansione e la conferma non ricevono quel contesto, quindi Ctrl+C in quei momenti potrebbe non interromperle (vedi `REGISTRO.md`).

Codici di uscita: 0 ok, 1 errore (messaggio su stderr, prefisso `errore:`), 2 uso sbagliato, **3 carta assente**, **4 inceppamento**, **130 interrotto con Ctrl+C**
(con TWAIN, `twainscan32.exe` usa gli stessi codici 3 e 4 e `libretto` li ripropone).

`scan` e `read` fanno anche questo (funzione `scanAndPost`):

- se il driver dice "carta assente" (WIA `PAPER_EMPTY`, o TWAIN) l'errore e' tradotto in italiano e accompagnato da un suggerimento (uscita 3). Con `-driver twain` `libretto` passa sempre `-ignora-sensore` al programma ausiliario, quindi il sensore dell'alimentatore non produce questo errore (un libretto dal percorso frontale non e' visto dal sensore);
- se la scansione e' vuota (nessun documento rilevato) esce con errore e **non** ritaglia (il PNG grezzo resta in `-o`); un foglio bianco
  su sfondo bianco non e' distinguibile dallo sfondo e viene trattato come vuoto;
- ritaglia al solo documento (`imgproc.AutoCropInfo`: rilevamento a **texture** e contrasto, quindi ombre e fondi lisci non contano) e lo raddrizza se inclinato oltre circa 0,3 gradi (entro +-6 gradi).

Gli errori WIA sono in italiano e riportano l'HRESULT (carta inceppata, scanner occupato o in riscaldamento, cavo USB, ecc.).
`extract` non fa il ritaglio automatico del documento: lavora sull'immagine con un semplice ritaglio dello sfondo bianco (`CropContent`) e un raddrizzamento
(`imgproc.Deskew`, oltre 2 gradi, entro +-12 gradi), poi prova le rotazioni.

**`-page-out` e `-page-only`: ritaglio della pagina dati.** `libreta.DataPage(path, rotation)` applica lo stesso raddrizzamento e la rotazione trovata da `Extract` e
`imgproc.CropDataPage` ricava il rettangolo 125x88 mm dalle due righe MRZ (44 caratteri a 2,54 mm: la larghezza dell'MRZ da' la scala in px/mm;
il fondo pagina sta `td3BottomMM` = 7,5 mm sotto la seconda riga, con affinamento sui bordi), senza OCR. Calibrato su 5 immagini (aspetto 1,417-1,423 contro 1,420 teorico);
la distanza dal fondo e' tarata su una sola scansione reale (stampa di un foglio): da riverificare su altri libretti veri. Se le righe MRZ non si
trovano il ritaglio non viene salvato (avviso su stderr) e il JSON esce comunque.

Debug (`-debug cartella`): `page.png` (l'immagine inviata all'OCR: il "foglio" dei ritagli, o la pagina ingrandita nel ripiego),
`page.txt` (testo OCR con le posizioni), e una sottocartella `rot0`, `rot180`, ... per ogni rotazione in cui e' stata trovata una coppia di
righe MRZ, con le righe ritagliate (`mrz_*.png`) e il testo letto (`mrz_ocr.txt`).

## Driver: WIA e TWAIN

| | WIA (`-driver wia`, predefinito) | TWAIN (`-driver twain`) |
|---|---|---|
| Come | `internal\wia`, COM in processo (go-ole) | `internal\twain` dentro `twainscan32.exe`, programma **a 32 bit** lanciato da `libretto` (il driver PaperStream IP e il suo DSM sono solo a 32 bit); testo del programma su stderr, il JSON resta su stdout di `libretto` |
| Inizio area (`-from`) | **ignorato** dal driver (misurato) | **accettato** dal driver (`ICAP_FRAMES`, riletto identico, senza scansione); che il motore salti davvero i primi mm e' **da provare con un libretto vero** |
| Area minima per la pagina dati | `-len 190` dal bordo (la pagina dati sta tra 90 e 180 mm) | `-from 90 -len 100`, se il motore salta (altrimenti `-len 190 -from 0`) |
| Una facciata | *Document Handling Select = 1*, controllato | duplex spento e riletto, `CAP_XFERCOUNT=1`; controllati prima di avviare |
| Ritentare | mai su `Transfer` | mai |
| Stato | usato con documenti reali | solo sonde e prove senza documento |

`twainprobe32.exe` (`cmd\twainprobe`, compilato in `_out\`) **non scansiona mai**: elenca capability e prova a scrivere/rileggere `ICAP_FRAMES` (`-try-frame`).
`twainprobe64.exe` stampa solo cosa manca per il driver a 64 bit (non installato). `twainscan32.exe` (`cmd\twainscan`) **avvia lo scanner**: chiede di digitare `SI` (a meno di `-yes`),
salva il PNG grezzo in `scans\twain-AAAAMMGG-hhmmss.png` senza mai sostituire un file esistente (salvo `-force`) e non ritenta. Dettagli e misure in `docs\agenti\03-wia.md`.

## Prove con lo scanner (script in `tools\`)

Tutti gli script **avviano lo scanner** (li lancia l'utente, mai gli agenti di documentazione): chiedono conferma con `pause`, si fermano se `scans\NOME.png` o `.json`
esistono gia', e a fine prova **la finestra resta aperta** e chiede il nome della prova successiva (Invio vuoto per uscire). Richiedono `libretto.exe` nella radice e Python (per `summary.py`).

| Script | Uso | Cosa fa |
|---|---|---|
| `tools\prova-libretto.cmd [NOME] [DPI] [LUNGH_MM] [DA_MM]` | **libretto aperto** | `read -yes -len 190 -page-only` (default 190 mm, 300 dpi): `scans\NOME.png` e' la sola pagina dati 125x88 mm, `NOME.json` il risultato, `NOME.err` i messaggi |
| `tools\prova-twain.cmd [NOME] [DPI] [LUNGH_MM] [DA_MM]` | libretto aperto, driver TWAIN | `read -driver twain -len 100 -from 90 -page-only`. Se l'immagine mostra ancora l'informativa il motore non salta: riprovare con `LUNGH 190`, `DA 0` |
| `tools\prova-pagina.cmd [NOME] [DPI]` | **foglio stampato** | `read -len 110 -page-only` (110 mm: bastano 105 per un foglio ingrandito del 10%). **Non adatto a un libretto vero**: con 110 mm si leggerebbe l'informativa (vedi sotto) |
| `tools\prova.cmd NOME [DPI]` | prova semplice | `read -yes -o scans\NOME.png -page-out scans\NOME-pagina.png` e riassunto di `tools\summary.py` |

**Come inserire il libretto** (`prova-libretto.cmd`, `prova-twain.cmd`): apri il libretto sulla pagina con la **foto**; inserisci **per primo il bordo con le due righe di MRZ** (`<<<<`),
faccia verso l'alto, al centro, dritto, nell'apertura **frontale**; un solo libretto alla volta; spessore della meta' inserita al massimo 5 mm. Conseguenza misurata: la scansione parte dal
**bordo che resta in mano**, quindi **in cima all'immagine finisce l'informativa** (l'altra pagina) e la pagina dati sta tra **90 e 180 mm** dal bordo; il bordo MRZ finisce in fondo.
Il driver WIA ignora l'inizio dell'area, quindi non si puo' saltare l'informativa: per questo il default e' 190 mm (180 mm + 10 di margine). Se si inceppa **non tirare con forza**: apri prima il coperchio dello scanner, poi estrai.

**Prima prova con un foglio di carta** (storica, prima dei libretti veri): chi non ha mai usato lo scanner puo' stampare la pagina dati di un facsimile con la MRZ leggibile
(`facsimili\panama\libreta_2020_pagina-dati_QR.jpg` o una delle due `libreta_2026_anexo8_pagina-dati_*.jpg`, o meglio una pagina con lo stesso layout e **dati inventati**) e scansionarla
dal caricatore dall'alto, **solo per fogli di carta**. I facsimili AMP potrebbero mostrare una persona reale (vedi `facsimili\panama\LEGGIMI.md`): stampa per uso locale e **distruggi il foglio** dopo la prova.
Passi: `.\libretto.exe info` (nome, driver, 75-600 dpi, *Document Handling*; lo stato "alimentatore pronto" **non** dice che c'e' carta); `.\libretto.exe scan -o prova.png -dpi 300`
(controllare: pagina intera, nessun bordo nero, dritta, nitida); `.\libretto.exe extract -i prova.png -debug .\_out\prova1` (tutti i `mrz.checks` a `true`, `warnings` vuoto o spiegabile);
ripetere con il foglio girato di 180 gradi e con una leggera inclinazione; poi cancellare `prova.png`, `_out\prova1` e distruggere il foglio. Con un libretto vero la prima volta usa `scan` (non `read`),
guarda l'immagine, poi `extract -i` con `-page-out`: una lettura sbagliata non costringe a riscansionare.

Cosa le prove **non** verificano ancora: TWAIN con un libretto vero, `-width`, e l'effetto di *Document Handling Select = 1* sul percorso fisico scelto.

## Come funziona

```
scansione 1 facciata (WIA o TWAIN, solo alimentatore senza duplex; area limitata con -len)
   -> scarto delle scansioni vuote e ritaglio automatico a texture (solo scan/read)
   -> (extract e read) CropContent + raddrizzamento Deskew
   -> orientamento: si provano 0, 180, 90, 270 gradi cercando l'MRZ; si tiene la prima rotazione con tutti i check digit validi
      (o, in mancanza, quella con il punteggio piu' alto)
   -> MRZ: righe ritagliate, binarizzate, lette dall'OCR a 3 scale (e, se il tratto e' spesso, in varianti assottigliate), riparate con i
      check digit; se una riga resta vuota, lettura a pezzi
   -> campi stampati: ritagli di ogni campo (15 ritagli x 3 varianti, piu' 11 varianti binarizzate e ritagli "jitter" dei testi liberi se il tratto e' spesso)
      letti dall'OCR, valore deciso a maggioranza (per carattere)
   -> confronto zona visiva / MRZ (l'MRZ prevale) e avvisi con codice
   -> JSON
```

- **Scansione (`internal/wia`, `internal/twain`).** WIA 2.0 via COM: il driver non ha un flag "solo fronte", quindi si imposta *Document Handling Select = 1*
  (alimentatore senza il bit duplex 4) e si rifiuta di scansionare se il duplex risulta ancora attivo. Una passata, un lato, `Pages = 1`.
  Ogni operazione fissa il goroutine a un thread del sistema (`LockOSThread`, apartment STA) e ha un timeout. Se il driver restituisce BMP
  lo converte in PNG. L'area (`-from`, `-len`, `-width`) si imposta nell'ordine che il driver richiede (prima l'inizio, poi l'estensione) e si rilegge: se il driver non accetta,
  la scansione e' annullata prima di avviare lo scanner. TWAIN: stesso criterio (area riletta, annullo prima di avviare), "ricette" di `ICAP_FRAMES` provate in ordine finche' il driver ne accetta una.
- **Ritaglio del documento (`imgproc.AutoCropInfo`).** Si riduce l'immagine a circa 800 px, lo sfondo e' la mediana di una cornice; e' "documento" cio' che si scosta dallo sfondo **e** ha texture
  (deviazione standard locale) o un forte contrasto; componenti connesse, scarto della polvere, stima dell'inclinazione (+-6 gradi) e raddrizzamento. Un documento bianco su sfondo bianco non e' rilevabile.
- **Orientamento.** Il libretto aperto puo' arrivare capovolto di 180 gradi o ruotato di 90: la traccia piu' affidabile sono le due righe
  MRZ. Per ogni rotazione si cerca una coppia di righe di testo compatibile con una MRZ (stessa altezza, stesso margine, larghezza simile);
  **senza coppia non si avvia l'OCR** (costerebbe circa 1 s).
- **MRZ (`internal/libreta/mrzband.go`, `internal/mrz`).**
  1. Binarizzazione (sparisce il fondo guilloche chiaro) e rimozione di bordi e ombre scuri.
  2. Le due righe si ritagliano a tre scale (altezza 32, 45 e 60 px) e si leggono con **una sola chiamata OCR**.
  3. Si provano tutte le combinazioni riga 1 x riga 2; `mrz.Repair` corregge i caratteri confondibili (O/0, I/1, ...) e accetta la correzione
     solo se e' l'unica che fa tornare anche il controllo finale. Il punteggio e' 1, piu' 2 per ogni check digit valido, piu' 2 se l'emittente e' `PAN`.
  4. **Tratti spessi.** Sulle stampe laser a 300 dpi il tratto riempie i vuoti tra i simboli (`<`) e il motore OCR di Windows non legge nulla.
     Si misura lo spessore dell'asta (`imgproc.StrokeWidth`) e si preparano varianti assottigliate: erosione di 1 o 2 px e soglia di
     binarizzazione scelta in modo da lasciare una data frazione di inchiostro (`imgproc.ThinToInk`, 7 varianti). Con tratto spesso le varianti
     normali e quelle assottigliate vanno nella stessa chiamata OCR (si risparmia l'avvio di PowerShell), altrimenti in due chiamate.
  5. Se una riga e' letta per intero e l'altra no, la si rilegge **a pezzi tagliati sulle 44 celle** dei caratteri (tagli diversi, con prefisso per i
     pezzi fatti di soli `<`) e si decide cella per cella a maggioranza.
  6. Ripiego: se nessuna rotazione trova la MRZ, si cercano due righe da circa 44 caratteri nell'OCR della pagina intera.
  Le correzioni fatte sono elencate in `mrz_corrections`.
- **Campi stampati (`internal/libreta/viz*.go`).** Il layout della pagina dati e' fisso: le posizioni dei campi sono costanti in rapporto
  alla larghezza della MRZ. Individuata la MRZ (e la sua inclinazione), ogni campo (etichetta e valore) si ritaglia in 3 varianti di scala e
  finestra, si impila tutto in un "foglio" e lo si legge con l'OCR, **piu'** una seconda lettura con un ritaglio per immagine (piu' affidabile in
  isolamento). Il valore finale e' quello su cui concorda la maggioranza delle letture (voto per carattere con pesi misurati sulle cifre note dall'MRZ), dopo il riconoscimento per formato (data, numero,
  livello, ...); se una lettura e' incompleta (manca l'ultima parola) sostiene quelle piu' lunghe che la contengono.
  **Assottigliamento adattivo:** sulle righe MRZ si misura il rapporto spessore/altezza del tratto e se ne ricava una soglia di
  binarizzazione; con tratto spesso (scansione a 300 dpi) si aggiungono **11 varianti binarizzate**, solo come ritagli singoli; con tratto sottile (facsimili) non si aggiunge nulla.
  **Jitter (`viz_jitter.go`):** sui soli campi di testo libero (`luogo_nascita`, `cargo`, `regionale`) delle scansioni a tratto spesso si aggiungono circa 24 ritagli spostati di 3-5 px, perche' l'OCR del grassetto su
  ologramma e' fragile (misurato su un libretto vero: esatti in 3 spostamenti su 7) e il voto per carattere corregge gli errori quasi indipendenti.
  **Nazionalita' (`viz_nationality.go`):** tabella ISO 3166-1 alpha-3 -> aggettivi inglesi (199 codici); l'aggettivo atteso per il codice dell'MRZ prevale se la lettura gli somiglia (>= 0,5).
  Costo complessivo: circa 11-13 s per immagine a 300 dpi. Il **numero libretto** e' letto anche con le sole sette cifre e ricomposto con il prefisso (`P` o `PA`). Se il `type` stampato non si legge (sta su un ologramma) lo si ricava dall'MRZ (`IS` -> `SI`) con avviso `UNCERTAIN_FIELD`.
- **Ripiego sulla pagina intera.** Se la MRZ non si trova, i campi stampati si cercano sull'OCR della pagina intera (tre varianti: base, contrasto stirato, rampa morbida) ancorandosi ai pattern dei
  valori e a etichette confrontate in modo approssimato. **Questo modo e' debole**: usarlo solo come segnalazione, non fidarsi dei campi.
- **Confronto.** Cognome, nomi, sesso, nazionalita' (ISO), date e numero libretto vengono presi dall'MRZ; se la zona visiva non coincide si
  aggiunge un avviso. Le regole sono in `docs\campi.md`.

## Uscita JSON

Schema versione 1. I nomi dei campi sono **in inglese** e coincidono con le etichette stampate sul documento (snake_case): chi legge il JSON ritrova le parole della pagina dati.

```json
{
  "schema_version": 1,
  "rotation_deg": 180,
  "mrz": {
    "doc_code": "IS", "issuer": "PAN",
    "surname": "ROSSI", "given_names": "MARIO",
    "doc_number": "P1234567", "nationality": "ITA",
    "birth_date": "1990-01-01", "sex": "M", "expiry_date": "2030-12-31",
    "optional": "PANAMA",
    "line1": "ISPANROSSI<<MARIO<<<<<<<<<<<<<<<<<<<<<<<<<<<",
    "line2": "P1234567<1ITA9001011M3012316PANAMA<<<<<<<<44",
    "checks": { "doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true }
  },
  "fields": {
    "capacity": "ORDINARY SEAMAN",
    "country_code": "PAN",
    "date_of_birth": "01-01-1990",
    "date_of_birth_iso": "1990-01-01",
    "date_of_birth_mrz": "1990-01-01",
    "date_of_expiry": "31-12-2030",
    "date_of_expiry_iso": "2030-12-31",
    "date_of_expiry_mrz": "2030-12-31",
    "date_of_issue": "31-12-2025",
    "date_of_issue_iso": "2025-12-31",
    "given_names": "MARIO",
    "id_number": "C12345678",
    "level": "SUPPORT",
    "nationality": "ITALIAN",
    "nationality_iso": "ITA",
    "place_of_birth": "GENOVA ITA",
    "regional": "PANAMA",
    "regional_mrz": "PANAMA",
    "seafarers_id": "987654321",
    "seamans_book": "P1234567",
    "seamans_book_mrz": "P1234567",
    "sex": "M",
    "surname": "ROSSI",
    "type": "SI"
  },
  "warnings": [
    { "code": "UNCERTAIN_FIELD", "field": "capacity" },
    { "code": "PRINTED_FIELDS_NOT_READ", "fields": ["certificate", "regional"] }
  ]
}
```

Tutti i valori di questo esempio sono **inventati** (le chiavi di `fields` sono in ordine alfabetico, come le scrive Go; `warnings` e' un esempio di forma, non coerente col resto dell'esempio).

- `schema_version`: versione dello schema (oggi 1).
- `rotation_deg`: rotazione applicata (0, 180, 90, 270).
- `mrz`: assente se non trovato. `checks` dice quali check digit tornano; `AllOK` = tutti `true`. Le chiavi interne di `mrz` sono quelle del pacchetto `mrz` (`doc_code`, `surname`, ...).
- `mrz_corrections`: sostituzioni di caratteri fatte dalla riparazione OCR, in inglese (`line2 pos 14: S->5 (check digit)`), da mostrare all'operatore; assente se non ce ne sono.
- `fields`: campi estratti (stampati e derivati dall'MRZ: `surname`, `given_names`, `nationality_iso`, `sex`, `*_mrz`). Le date stampate hanno due forme: `DD-MM-YYYY` e `..._iso`. L'elenco completo con
  etichette, formati e validazione e' in `docs\campi.md`. Un campo non letto e' semplicemente **assente** (`certificate` esiste solo nel modello 2026).
- `warnings`: assente se vuoto. Ogni avviso e' un oggetto con **codice stabile** (`code`) piu' `field` o `fields`, senza testo libero ne' valori personali. Codici: `MRZ_NOT_FOUND`, `MRZ_CHECK_DIGITS_INVALID`,
  `PRINTED_FIELDS_NOT_READ` (`fields`), `UNCERTAIN_FIELD` (`field`), `PRINTED_FIELD_DIFFERS_FROM_MRZ` (`field`: nascita, scadenza, numero libretto, sesso, nazionalita', regionale),
  `DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE`, `DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE`, `UNKNOWN_WARNING` (testo non riconosciuto). **Con `warnings` non vuoto i dati vanno verificati a mano.**
  Vedi `docs\campi.md` per le condizioni.
- Le chiavi sono cambiate rispetto alle prime versioni (italiano/spagnolo, senza `schema_version`): la tabella di corrispondenza e' in `docs\campi.md`.

## Prestazioni e misure

Misure del 2026-10-22 su 5 immagini (ultime riportate in `docs\agenti\`): facsimile 2020 (QR), facsimili 2026 A e B, `panama.png` (scansione reale) e una stampa scansionata a 300 dpi, ruotata di 180 gradi:

| Misura | Risultato |
|---|---|
| MRZ, check digit validi | 5 su 5 su tutte e 5 le immagini |
| Campi stampati *trovati* | da 13 a 14 su 14 (il `certificate` manca sui modelli che non lo hanno) |
| Campi stampati *esatti* | non sempre tutti: sulle scansioni reali 13-14 su 14 (`cargo` con sigla `II/2` sbagliato, l'OCR non la legge); verita' di terreno fatta a mano |
| Tempo per immagine | da 5,5 s a 13,3 s (le 300 dpi sono le piu' lente; ogni chiamata OCR costa circa 1 s di avvio di PowerShell) |
| Ripiego sulla pagina intera | debole: pochi campi, non affidabili |

"Trovato" non vuol dire "esatto": con `warnings` vuoto i dati sono coerenti con l'MRZ, ma i campi stampati senza un dato corrispondente nell'MRZ
(`capacity`, `place_of_birth`, `id_number`, `level`, `seafarers_id`, `certificate`) non hanno altra verifica.
Dopo quelle misure il VIZ ha aggiunto la nazionalita' da tabella e il jitter dei testi liberi (l'agente VIZ e' ancora al lavoro): le misure aggiornate non sono ancora in `docs\agenti\02-viz.md`.

## Limiti noti

- **Una sola facciata**, niente duplex, niente doppia pagina: serve la pagina dati con l'MRZ.
- **WIA ignora l'inizio dell'area**: per un libretto si leggono i primi 190 mm dal bordo (circa meta' immagine vuota o con l'informativa). TWAIN dovrebbe risolverlo, ma non e' provato con un documento.
- **TWAIN solo a 32 bit**: serve `twainscan32.exe`; il driver a 64 bit non e' installato. `libretto` passa sempre `-ignora-sensore`, quindi "carta assente" da sensore non esiste su quel percorso.
- Il motore OCR di Windows non e' addestrato sull'OCR-B e si confonde col fondo guilloche e l'ologramma: per questo l'MRZ e' letto
  riga per riga e validato dai check digit; i campi stampati hanno **nessuna validazione** oltre al formato e al confronto con l'MRZ
  (`level`, `capacity`, `place_of_birth`, `id_number`, `certificate`, `seafarers_id`, `regional`).
- Se l'MRZ non e' leggibile si usa il ripiego sulla pagina intera, debole, e i campi stampati non sono verificabili (avviso nel JSON).
- Il layout dei ritagli e' stato misurato sui modelli 2020 e 2026 e su scansioni reali; i modelli 2017 e 2019 sono stati visti solo con dati
  oscurati. Per il 2017 (senza Id Marino, Identificazione e Regional) l'avviso `PRINTED_FIELDS_NOT_READ` compare comunque.
- Il livello (`level`) e' riconosciuto solo se e' `SUPPORT`, `OPERATIONAL` o `MANAGEMENT`.
- Barcode (QR/PDF417) non decodificati; foto e firma non estratte; verifica ufficiale del documento fuori ambito (sito AMP / Panama Ship
  Registry).
- La validita' del libretto e' variabile (5 anni nei facsimili, circa 5 mesi in una scansione reale): il programma non la presume.
- Documentati solo i libretti dei marittimi, non il passaporto di Panama.
- Solo Windows; l'OCR e' nativo (circa 3,7 s per una scansione contro 13,1 s col vecchio PowerShell, stesso JSON; 90 s di timeout per chiamata). Il vecchio percorso (`powershell.exe`, circa 1 s di costo fisso per chiamata) resta come ripiego e con `LIBRETTO_OCR=powershell`. Motivo del cambio: un controllo di sicurezza del PC termina in modo intermittente i processi PowerShell (codice `0xC0400065`) e con loro l'albero di `PanamaBook.exe`.
- `info` mostra *Document Handling Status*, che indica "alimentatore pronto" e **non** la presenza di carta.
- Il programma non puo' scegliere il percorso fisico della carta (caricatore o frontale): dipende da dove si inserisce il documento.
- Le calibrazioni (ritaglio `-page-out`, soglie dello spessore del tratto, `IsBlank`/`AutoCrop`, jitter) sono state fatte su poche scansioni reali: possono non generalizzare.
- Ctrl+C: il contesto annullabile arriva a `extract`/`read` ma non alla scansione (vedi `REGISTRO.md`).
- `internal/libreta/viz.go` e' troppo grande (circa 2400 righe, 72 KB, ancora in crescita: l'agente VIZ ci lavora) e il progetto **non e' un repository git**: non ci sono snapshot (vedi `REGISTRO.md`).

## Sicurezza

- **Non usare il caricatore dall'alto (ADF) per i libretti.** Il libretto e' rigido (cartonato, fino a 5 mm di spessore) e il percorso
  curvo dell'alimentatore lo rovina e puo' inceppare. Usare il **percorso frontale "Return Scan"** del fi-800R, che accetta
  documenti spessi fino a 5 mm, verificando le istruzioni nel manuale PFU. Il programma non puo' scegliere il percorso: lo decide
  la posizione fisica del documento e della leva, mentre WIA espone un solo "alimentatore". La skill `libretto-panama` consiglia
  il "vassoio piatto": il fi-800R non ha il piano di lettura, quindi vale la regola del percorso frontale. Il caricatore dall'alto
  si usa **solo per fogli di carta** (per esempio la prova stampata).
- **Conferma e nessuna sovrascrittura.** `scan` e `read` chiedono conferma prima di avviare lo scanner (senza terminale serve `-yes`), perche' un
  documento gia' nel percorso frontale viene tirato dentro; un file di uscita esistente non viene sostituito senza `-force`, e il rifiuto avviene
  prima dello scanner. Con `-yes` e' il chiamante a rispondere del documento inserito. `twainscan32.exe` lanciato a mano chiede di digitare `SI`.
- **Inceppamento.** Non tirare il libretto con forza: aprire prima il coperchio dello scanner, poi estrarre.
- **Dati personali.** Scansioni, JSON, cartelle `-debug`, file di `-page-out`, `-page-only` e `-raw`, `scans\` (circa 380 MB, anche `scans\vecchi test WIA\`), `_out\` e `_da_eliminare\` contengono nome, foto, data di nascita e
  numeri di documento di persone reali: non committarli (il `.gitignore` li esclude, come `facsimili\`), non inviarli a servizi esterni, cancellarli
  quando non servono (il programma avvisa su stderr quando `-debug`, `-page-out` o `-raw` salvano dati personali), e controllare che la cartella non sia sincronizzata su un servizio cloud. Il programma non invia nulla in rete:
  OCR e scansione sono locali, ma `read > risultato.json` scrive i dati su disco.
- Le prove con persone reali vanno fatte solo con il consenso del titolare; per i test usare dati sintetici (come negli esempi).
- I test di integrazione e gli strumenti di sviluppo scrivono immagini e testo OCR in `_out\` (con dati personali): vedi `REGISTRO.md`,
  "Pulizia proposta". I resti delle prove precedenti sono in `_da_eliminare\` (quarantena): vanno cancellati appena possibile.

## Struttura

| Percorso | Contenuto |
|---|---|
| `cmd\libretto\main.go` | CLI: `info`, `scan`, `extract`, `read`, `summary`, `pdf` (+ `area-probe` nascosto); `scanAndPost`, `confirmScan`, `saveRaw`, `savePage`, codici di uscita |
| `cmd\libretto\pdf.go`, `internal\pdfout` | comando `pdf`: PDF di una pagina entro un peso massimo (`cmdPDF`); `pdfout.Encode` incorpora un JPEG (DCT) alla qualita' piu' alta che sta nel limite, poi riduce l'immagine a passi del 7%; test in `pdfout_test.go` |
| `tools\pdf.cmd` | crea `scans\NOME.pdf` da `scans\NOME.png` per il gestionale (**non usa lo scanner**; ripiega su `_out\libretto_pdf.exe` finche' `libretto.exe` non ha il comando) |
| `cmd\libretto\output.go` | JSON in uscita: `output`, `fieldNames` (nomi inglesi), `toWarning` (codici stabili), `correction`; test in `output_test.go` |
| `cmd\libretto\twaindrv.go` | scelta del driver (`acquire`), ricerca e lancio di `twainscan32.exe` (`scanTwain`), `defaultDriver` |
| `cmd\twainscan\main.go` | programma di scansione TWAIN (da compilare a 32 bit): **avvia lo scanner** |
| `cmd\twainprobe\main.go` | sonda TWAIN: capability e prova di `ICAP_FRAMES`, **non scansiona** |
| `internal\wia` | acquisizione WIA/COM (solo Windows): `wia.go`, `options.go` (con `Force`, area), `area.go`, `probe.go`, `errors.go`, `bmp.go`, `outpath.go` (verifica della destinazione prima dello scanner, temporanei casuali) |
| `internal\twain` | client TWAIN indipendente da 32/64 bit: `session.go`, `frames.go` (ricette di `ICAP_FRAMES`), `area.go`, `container.go`, `layout.go`, `win.go` (DSM), `dib.go`, `probe.go`, `capnames.go`, `errors.go` |
| `internal\imgproc` | caricamento, rotazione, ritaglio (`CropContent`, `AutoCrop` a texture, `IsBlank`), raddrizzamento (`deskew.go`), scala, binarizzazione, righe di testo, spessore del tratto e assottigliamento (`StrokeWidth`, `Erode`, `ThinToInk`), ritaglio della pagina dati TD3 dall'MRZ (`pagecrop.go`: `CropDataPage`, `DataPageGeometry`) |
| `internal\ocr` | OCR integrato di Windows: backend nativo (COM/WinRT, `ocr_native_windows.go`) e ripiego PowerShell (`ocr_powershell.go`, `winocr.ps1` incorporato); scelta con `LIBRETTO_OCR` |
| `internal\mrz` | parser TD3, check digit, riparazione errori OCR (orologio `now` iniettabile per i test) |
| `internal\libreta` | orchestrazione (`extract.go`, `DataPage`), righe MRZ (`mrzband.go`), campi stampati (`viz.go`, `viz_vote.go`, `viz_page.go`, `viz_jitter.go`, `viz_nationality.go`) |
| `docs\campi.md` | tabella dei campi, codici degli avvisi e regole |
| `REGISTRO.md` | registro della codebase e proposta di pulizia |
| `drivers\` | installazione del driver del fi-800R (vedi `drivers\LEGGIMI.md`) |
| `facsimili\panama\` | facsimili AMP (contengono dati personali, non versionati; vedi `LEGGIMI.md`) |
| `docs\agenti\` | un rapporto per ogni agente (MRZ, campi stampati, WIA/TWAIN, documentazione, sette revisioni di architettura), con stato, misure e decisioni aperte; parte da `docs\agenti\README.md` |
| `tools\summary.py` | riassunto in una riga (rotazione, MRZ, check digit, campi trovati, avvisi) di un'immagine (lancia `libretto.exe extract`) o di un JSON gia' prodotto; senza dati personali |
| `tools\prova-libretto.cmd`, `prova-twain.cmd`, `prova-pagina.cmd`, `prova.cmd` | prove con lo scanner (**lo avviano**; vedi "Prove con lo scanner") |
| `drivers\installazione_manuale\` | INF/CAT/CAB del solo driver USB/WIA per installarlo a mano (vedi "Requisiti") |
| `drivers\_work\`, `drivers\_extract\` | intermedi dell'installer PFU, tenuti su richiesta (non versionati) |
| `scans\` | scansioni di prova e risultati (dati personali, non versionati); le prove WIA precedenti sono in `scans\vecchi test WIA\` (circa 380 MB), le uscite di `twainprobe32.exe` in `scans\twain-probe*.txt` |
| `_out\` | cartella di scratch (circa 100 MB): test di integrazione, esempi di `-debug`, eseguibili di sviluppo, `twainprobe32.exe` (non versionata) |
| `_da_eliminare\` | **quarantena** dei resti di prova degli agenti (circa 366 MB, con immagini con dati personali). Nulla e' stato cancellato: l'elenco con i percorsi originali e' in `_da_eliminare\ELENCO.md` (non aggiornato per `round4\` e `round5\`); la cancellera' l'utente |
| `libretto.exe`, `twainscan32.exe` | eseguibili ricostruiti dal codice attuale (si rigenerano con `go build`) |
| `.claude\agents\` | definizioni dei 4 agenti di progetto: `libretto-docs`, `libretto-mrz`, `libretto-viz`, `libretto-wia` (non e' codice del programma) |
