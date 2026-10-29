# 03 · Agente WIA (`libretto-wia`)

**Ruolo:** acquisizione da scanner Fujitsu/Ricoh fi-800R via WIA (COM con go-ole), una sola facciata, e post-elaborazione dell'immagine.
**File suoi:** `internal/wia/**` (`wia.go`, `errors.go`, `bmp.go`, `options.go`, test) e `internal/imgproc/autocrop.go`.

## Fatti accertati sul driver (WIA 2.0, driver 2.0.0.2)
- Document Handling Capabilities = 5 (alimentatore 1 + duplex 4), nessun flag "solo fronte". Select = 1 equivale a una sola facciata.
- Document Handling Status = 5 anche a scanner vuoto: il bit 1 vuol dire "alimentatore pronto", **non** "carta presente".
- Un solo item "Scan"; 75-600 dpi; area massima 8,5x14 pollici; formato preferito BMP.
- `Item` nelle collezioni WIA e' una propget: va chiamato con `oleutil.GetProperty(..., "Item", idx)`, non come metodo (difetto trovato e corretto dal committente).

## Revisione critica di `wia.go` — difetti corretti
- doppio rilascio dell'immagine dopo `Transfer` (`Clear` e `Release`): poteva far crashare il processo;
- mancava `LockOSThread` con l'apartment STA; ora `CoInitializeEx` gestisce `S_FALSE` e `RPC_E_CHANGED_MODE`;
- tolti tutti i `MustGetProperty` (andavano in panico); gestiti `ToIDispatch` nullo e VARIANT liberati;
- il controllo "solo una facciata" passava se la proprieta' non era leggibile: ora annulla la scansione, e la rifiuta anche se manca il bit alimentatore;
- `Pages=1` e' obbligatorio (intervallo 0..100): prima l'errore era ignorato;
- errori WIA tradotti in italiano con codice HRESULT (0x80210001..0F, 15..18, 1A: carta assente, inceppamento, coperchio aperto, occupato, USB); i codici da 0x15 in poi non sono verificati; il codice vero si legge da `EXCEPINFO.scode`;
- nuova opzione `Options.Timeout` (default 2 minuti); riprova (3 tentativi) solo su `Connect`, **mai su `Transfer`** (evita doppie scansioni o carta consumata);
- il file temporaneo usa l'estensione del driver; se e' BMP lo converte in PNG con un decoder proprio (non provato con il driver reale).

## Aggiunte
`wia.List()`, `wia.Dump()`, `IsPaperEmpty`, `IsPaperJam` · `imgproc.AutoCrop`, `AutoCropInfo`, `IsBlank` (media a blocchi, sfondo dalla cornice, maschera con
componenti connesse, inclinazione fino a ±6°, circa 0,2 s su 2550x4200).
Il committente ha collegato `IsBlank` e `AutoCrop` nella CLI (`scanAndPost`): `scan` e `read` scartano le scansioni vuote e ritagliano.

## Risultato reale
**Prima prova con lo scanner** (foglio con una pagina dati stampata, inserito dal fronte): una passata, una facciata, nessun errore, 11 s, PNG a colori 1679x2263.
L'avanti e indietro del foglio e' il comportamento del percorso frontale (manuale p. 12 e 45), non un'anomalia.

## Non verificato
`Transfer` con carta per il **libretto vero** (spessore fino a 5 mm); soglie di `IsBlank` e `AutoCrop` su scansioni reali (provate su sintetiche e su una reale);
l'effetto di `Select=1` sul percorso fisico scelto. Un foglio bianco senza stampa su sfondo bianco non e' rilevabile come vuoto.
Copertura dei test del pacchetto `wia`: 29%.

## Idea in lavorazione (non implementata)
Ritaglio esatto della pagina dati a partire dalla geometria dell'MRZ (125x88 mm; 44 caratteri a 2,54 mm). Prototipo fatto fuori dal programma sulla scansione reale:
scala stimata 309 dpi, ritaglio 1515x1071 px con la pagina intera; margine inferiore assunto a 5 mm da calibrare; richiede che `readMRZBand` restituisca le posizioni delle righe.

## Round 3 — sicurezza dei file di uscita (concluso)
- **`Options.Force`**: `Scan` chiama `checkOutPath(path, force)` subito dopo `normalized()`, **prima** di `withDevice`: un file esistente senza `Force` viene rifiutato con un errore che cita il file e `-force`, senza toccare lo scanner. Una cartella come destinazione e' sempre rifiutata; la cartella deve esistere; `finalizeImage` ricontrolla prima del rename.
- **Temporaneo sicuro** (`outpath.go`, `newTempName`, `tempFiles`): `os.CreateTemp` nella cartella di `OutPath`, nome casuale, rimosso sempre con `defer`; controllo di scrivibilita' della cartella prima dello scanner.
- Test in `outpath_test.go` (compresa unicita' dei nomi con 50 goroutine e `finalizeImage` con immagini sintetiche).
**Verificato dal committente:** `libretto scan -yes -o scans/prova1.png` risponde che il file esiste e di usare `-force`, senza avviare lo scanner.
**Non verificato:** comportamento reale con carta; c'e' una finestra minima tra la riserva del nome e `SaveFile` di WIA, che non sovrascrive.

## CLI collegata (committente, `cmd/libretto/main.go`)
Conferma prima di scansionare (senza terminale interattivo rifiuta: serve `-yes`); `-force`; Ctrl+C con `signal.NotifyContext`; avviso su stderr quando `-debug` salva dati personali; nome predefinito `scans\scan-AAAAMMGG-hhmmss.png`; codici di uscita 3 (carta assente), 4 (inceppamento), 130 (Ctrl+C). `tools\prova.cmd NOME [DPI]` per una scansione di prova con esito in una riga.

## Round 4 — area di scansione limitata (concluso, con un limite del driver)
**Aggiunto:** `Options.WidthMM`, `HeightMM`, `StartMM` (`options.go`, `area.go`, `probe.go`); funzioni pure `checkAreaMM` e `areaPixels`; flag `-len`, `-width`, `-from` nella CLI; sottocomando nascosto `area-probe` (solo proprieta', nessuna acquisizione).
**Verificato con carta:** `-len 100` produce un'immagine grezza di 2550x1181 px = 216x100 mm: lo scanner legge solo quella lunghezza dal bordo d'ingresso.
**Scoperto con `area-probe` (a 300 dpi, V Start 0..3600, V Extent 600..4200):** ogni modifica di `Vertical Start Position` fa **ricalcolare dal driver l'extent verticale** (valore non prevedibile) e il massimo dell'extent diventa 4200 meno lo start. L'extent impostato dopo lo start viene accettato. Correzione: impostare sempre prima lo start e poi l'extent, e rileggere (`applyArea`).
**Limite reale:** con `-from 90` il driver accetta l'area ma **l'inizio non cambia**: confronto tra `librettook` (start 0) e `librettook6` (start 90 mm) = spostamento 0,0 mm. L'area utile e' quindi sempre "i primi N mm" del documento.
**Cosa significa:** per un libretto vero (entra solo la meta' con la foto) basta `-len 130`; per il foglio stampato taglia il foglio alla piega e inserisci la meta' con la foto. Il movimento fisico non cambia: il manuale non ha impostazioni su quanto il documento viene tirato dentro; esiste `Paper Protection (Return)` (stop se il documento entra storto).
**Non verificato:** `-width`; il vero effetto di `Paper Protection (Return)` sul tuo scanner.
**CLI:** `tools/prova-libretto.cmd NOME [DPI] [LUNGH_MM] [DA_MM]`; scrive anche la riga "area richiesta" nel file `.err`.

## Lettura della sola pagina dati (2026-10-22)
Il driver ignora l'inizio dell'area, quindi la pagina dati va fatta entrare per prima e si limita la lunghezza. **Misurato per simulazione** (primi N mm delle scansioni con la pagina dati davanti, stampa ingrandita del 10%): 95 mm falliscono, 100 mm una su due, **105 mm leggono tutte e due** (MRZ 5/5, pagina dati intera, aspetto 1,420). Comando: `tools/prova-pagina.cmd NOME [DPI]` (110 mm, con la regola "entra per primo il bordo con le due righe di MRZ"). Non ancora provato con lo scanner.

## Misure sul libretto vero (2026-10-22)
Nel campo grezzo (216x356 mm) la pagina dati sta tra **90 e 180 mm** dal bordo d'ingresso e tra 46 e 173 mm in larghezza (127x90 mm); i primi 90 mm sono l'altra pagina. Il driver ignora l'inizio dell'area, quindi la lunghezza minima che contiene la pagina dati e' 180 mm: `tools/prova-libretto.cmd` legge **190 mm** di default (10 mm di margine). Senza limite il driver restituisce tutto il campo (2550x4200 px, meta' vuota). `-width` non e' stato provato: se il driver ignora anche lo start orizzontale taglierebbe la parte destra della pagina.

## TWAIN (2026-10-22) — studio statico del driver e client `internal/twain`
**Perche':** con WIA il driver ignora l'inizio verticale dell'area. Si verifica se il data source TWAIN PaperStream IP (3.40.2.1815, `C:\Windows\twain_32\Fjicube\Fjic*.ds`, DSM solo in SysWOW64) lo rispetta. **Nessuno dei file del driver e' stato eseguito**: solo letture di stringhe, ini, xml e tabelle `{id, nome}` nelle DLL (`DeviceCtl\DeviceCtl.dll`, `fjictwin.dll`, `DeviceCtl\fi800R.dll`, `DeviceCtl\fjtw0409.dll`).

**Cosa dice il driver sull'offset (non verificato con carta):**
- `ICAP_FRAMES` (0x1114) e `ICAP_MAXFRAMES` (0x111A) esistono, e la finestra delle impostazioni del driver ha un'area "Scanning Area" con **Left / Top / Width / Length** (anteprima, "relocating the scanning area"): l'alto e' quindi un parametro del driver. Il comando SCSI "Set Window" di `fi800R.dll` porta le coordinate al motore. **Non si sa se per l'alimentatore il motore salti davvero i primi mm**: lo dira' `twainprobe32.exe -try-frame` (il driver accetta l'alto?) e poi una scansione di prova.
- "Y Offset" nelle stringhe e' l'offset di stampa dell'imprinter/endorser, **non** l'inizio dell'immagine (corretto un possibile equivoco). Non esiste una capability "Return Scan": "Return path opening" compare solo come punto dell'inceppamento.
- Capability proprie utili (id letti dalle tabelle): `ICAP_EXT_FRAMEMODE` 0xA069, `ICAP_EXT_FRAMES_INDUNIT` 0xA020 e `ICAP_EXT_FRAMEUNIT` 0xA01F (riquadro in unita' indipendenti), `ICAP_EXT_TRIMSIZE` 0xA006 / `..UNIT` 0xA007 (taglio dei margini), `CAP_EXT_FRAME` 0x80CE, `ICAP_OVERSCAN` 0x1149 / `ICAP_EXT_OVERSCAN` 0x80F0, `ICAP_EXT_ADFPHYSICALHEIGHT/WIDTH` 0xA010/0xA011, `ICAP_EXT_PAPERLENGTH/WIDTH` 0x8003/0x8002 (+ `_INDUNIT` 0xA018/0xA017), `ICAP_EXT_LONGPAGE` 0x80D0, `ICAP_EXT_EXTENDEDLONGPAGE_ENABLED` 0xA100.
- Alimentazione e protezione: `CAP_EXT_PAPERPROTECTION` 0x80FE, `CAP_EXT_MANUALFEEDERENABLED` 0xA063, `CAP_EXT_SYNCHRONOUSEJECTION` 0x8103, `CAP_DOUBLEFEEDDETECTION` 0x103F (+ `LENGTH` 0x1040, `RESPONSE` 0x1042), `CAP_EXT_DOUBLEFEEDCONTROL` 0x8031, `CAP_EXT_MULTIFEEDUI` 0x80DB, `ICAP_EXT_MULTIFEEDERRORMODE` 0x8111, `CAP_EXT_PREPICK` 0x80FD, `CAP_EXT_PAUSESCANNING` 0x8106. Il testo del driver avverte che con Overscan o rilevamento automatico del formato il controllo spessore dell'alimentazione multipla non funziona.
- Senza finestre: `CAP_INDICATORS` 0x100B, `CAP_UICONTROLLABLE` 0x100E, e `MSG_ENABLEDS` con `ShowUI=false`.
- Ritaglio/raddrizzamento automatici (da spegnere per non alterare la geometria): `ICAP_AUTOMATICBORDERDETECTION` 0x1150, `ICAP_AUTOMATICDESKEW` 0x1151, `ICAP_AUTOMATICROTATE` 0x1152.
- L'intera tabella dei nomi e' in `internal/twain/capnames.go` (352 voci).

**`internal/twain` (nuovo, indipendente dall'architettura 32/64 bit):**
- `Scan(Options{DeviceMatch, DPI, Gray, StartMM, HeightMM, WidthMM, Timeout, IgnoreFeederLoaded}) (image.Image, error)`: apre DSM e data source senza finestre, **spegne il duplex e lo rilegge (altrimenti annulla)**, una sola pagina (`CAP_XFERCOUNT=1`), pollici, risoluzione verificata contro l'intervallo del driver, imposta `ICAP_FRAMES` e **lo rilegge: se il driver cambia l'alto, annulla prima di avviare lo scanner**. Controlla `CAP_FEEDERLOADED` (errore "carta assente" se vuoto; `IgnoreFeederLoaded` lo salta, utile se il sensore non vede un libretto dal percorso frontale). Trasferimento nativo DIB -> `image.Image` (`decodeDIB`), **mai ritentato**; scarta le pagine in coda; chiude DS e DSM anche in caso di errore. L'avvio (`MSG_ENABLEDS`) e' in un solo punto (`enableAndTransfer`).
- `Probe(match, ProbeOptions{TrySetFrame})`: apre DSM e data source, stampa elenco dei data source, tutte le capability con nome, flag GET/SET/RESET e valore corrente, il dettaglio di quelle su area/riquadro/alimentazione e `DAT_IMAGELAYOUT`. **Non chiama mai `MSG_ENABLEDS`** (test dedicato). Con `TrySetFrame` scrive solo `ICAP_FRAMES` (alto 90 mm, lunghezza 100 mm), rilegge, poi ripristina: e' una proprieta', non una scansione.
- Aree: `frameForArea` (pura) come in `wia`: origine in alto, larghezza centrata, offset+altezza <= campo (letto da `ICAP_PHYSICALWIDTH/HEIGHT`, altrimenti 8,5x14"), tolleranza 0,5 mm sul campo.
- 32 e 64 bit: strutture TW_* a 2 byte (come `twain.h`), costruite a mano in `[]byte` con `layout{ptr: 4|8}`: TW_CAPABILITY 8/12 byte, TW_USERINTERFACE 8/12, TW_EVENT 6/10; TW_IDENTITY 156, TW_STATUS 4, TW_PENDINGXFERS 6, TW_IMAGEINFO 42, TW_FRAME 16 uguali. Verificate da test per entrambe le dimensioni; `GOARCH=386 go vet/test` passano. Il DSM si carica dallo stesso numero di bit del processo.
- Esiste un solo driver, a 32 bit: **`_out\twainprobe32.exe` e' quello che funziona oggi**. `_out\twainprobe64.exe` stampa i percorsi cercati (`System32\TWAINDSM.dll`, `C:\Windows\twain_64`) e dice che serve 'PaperStream IP (TWAIN x64)' (installer PFU separato, non presente in `drivers\download`).
- Duplicazioni note (file di altri): `decodeDIB` ripete la logica di `wia.decodeBMP`, e `frameForArea` quella di `wia.areaPixels`: da portare in un pacchetto comune quando si potranno toccare.

**Come lanciare (lo lancia l'utente; non serve alcun documento, nessuna scansione):** `_out\twainprobe32.exe` e, per provare la scrittura dell'alto, `_out\twainprobe32.exe -try-frame > scans\twain-probe.txt`. **Non verificato:** l'apertura reale del data source, i valori delle capability, se il motore rispetta l'alto, il formato esatto con cui il driver accetta `ICAP_FRAMES` (OneValue di TW_FRAME; se rifiuta, provare `DAT_IMAGELAYOUT` o `ICAP_EXT_FRAMES_INDUNIT`), e tutta la parte a 64 bit.

### Esito della prova di scrittura di ICAP_FRAMES (2026-10-22, `twainprobe32.exe -try-frame [-all]`, file `scans\twain-probe2.txt` e `scans\twain-probe4.txt`)
**Il driver ACCETTA l'alto del riquadro** (alto 90 mm = 3,543 pollici, lunghezza 100 mm) e lo rilegge identico. Nessuna scansione, solo proprieta' (riportate al predefinito a fine prova; il predefinito non persiste tra una sessione e l'altra: verificato con una seconda apertura).
- Causa del primo rifiuto (CC=10 BADVALUE): il riquadro era piu' largo del formato carta corrente (A4 = 8,268 x 11,693 pollici, `ICAP_SUPPORTEDSIZES`=1). Non e' l'alto.
- Accettato e riletto: riquadro dentro l'A4 predefinito; con `ICAP_SUPPORTEDSIZES`=NONE (0) solo dentro la larghezza A4 (con 8,5 pollici: rifiutato); con USLEGAL (4) larghezza piena 8,5 pollici; con formati propri 32769/32770/32801/32802/32803 piu' `ICAP_EXT_PAPERWIDTH/LENGTH` = 8,5 x 14; riquadro in centimetri e in pixel (a 300 dpi: alto 1063 px); riquadro piu' stretto e centrato (190 mm); `ICAP_EXT_FRAMES_INDUNIT`, `ICAP_EXT_FRAMEMODE`=1 e `CAP_EXT_FRAME`=1 non cambiano l'esito. Rifiutato: MAXSIZE (53) con 8,5 pollici, e NONE con 8,5 pollici (anche in cm).
- Unita' di `ICAP_UNITS`/`ICAP_EXT_FRAMEUNIT`: 0 pollici, 1 centimetri, 5 pixel (nessun millimetro).
- Altro dalla prova: `CAP_FEEDERLOADED` = falso a scanner vuoto (affidabile?), `CAP_FEEDERALIGNMENT` = 2 (centrato), `ICAP_XRESOLUTION` 50..600, `ICAP_EXT_PAPERWIDTH` 8,268, `ICAP_EXT_ADFPHYSICALHEIGHT/WIDTH` 16800/10200 (centesimi di pollice?), `ICAP_MAXFRAMES` 1. `CAP_EXT_PAPERPROTECTION`, `CAP_EXT_MANUALFEEDERENABLED` e `CAP_EXT_SYNCHRONOUSEJECTION` **non sono supportate** da questo driver. `MSG_RESET` di `ICAP_FRAMES` non ripristina il predefinito: `internal/twain` lo reimposta esplicitamente.
- DAT_IMAGELAYOUT (DG_IMAGE) non e' stato usato (opzione `-layout`, spenta): `ICAP_FRAMES` e il layout risultavano gia' uguali nella prima prova.
- **Bug corretto durante la prova:** il campo fisico (`ICAP_PHYSICALWIDTH`) e' espresso nell'unita' corrente, non sempre in pollici: ora si converte (`unitsToMM`).

**`internal/twain.Scan` ora imposta l'area con le "ricette"** (`frames.go`): prova in ordine il riquadro nel formato predefinito (la piu' semplice), poi NONE, MAXSIZE, USLEGAL, carta personalizzata, centimetri, pixel, capability EXT, riquadro stretto; tiene la prima che il driver accetta e rilegge identica; se nessuna funziona annulla prima di avviare lo scanner, elencando l'esito di ognuna. `Options` invariato; `ScanInfo` restituisce anche la ricetta usata. Con il formato A4 l'immagine e' larga 210 mm (2480 px a 300 dpi), non 216.

**Comando di prova con carta (da lanciare a mano, AVVIA LO SCANNER):** `_out\twainscan32.exe -from 90 -len 100 -dpi 300` — chiede conferma (SI), scrive il PNG grezzo `scans\twain-AAAAMMGG-hhmmss.png` (mai sovrascrive), una pagina, una facciata, nessun nuovo tentativo. `-ignora-sensore` salta il controllo `CAP_FEEDERLOADED` se il sensore non vede un libretto inserito dal percorso frontale. **Non verificato:** che il MOTORE salti davvero i primi 90 mm (la differenza si misura confrontando il contenuto con `-from 0 -len 190`), la scansione TWAIN in se' (DIB nativo, evento `XFERREADY`), `CAP_AUTOFEED`/avvio.
