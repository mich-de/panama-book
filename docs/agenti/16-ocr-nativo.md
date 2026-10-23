# 16 - OCR nativo (agente `libretto-ocr`)

Ultimo aggiornamento: 2026-10-23. Nessun dato personale: solo esiti numerici.

## Perche'

L'OCR girava lanciando `powershell.exe -File winocr.ps1`. Sul PC aziendale un controllo di sicurezza termina in modo intermittente
i processi PowerShell (codice di uscita `0xC0400065`) e, quando succede, l'albero `PanamaBook.exe -> libretto.exe -> powershell`
viene ucciso per intero. Il backend nativo toglie PowerShell dal percorso dell'OCR: il motore e' lo stesso
(`Windows.Media.Ocr`), ma si chiama dal processo di `libretto.exe`, senza processi figli.

## Cosa e' stato fatto

Pacchetto `internal/ocr` (tutti file dell'agente `libretto-ocr`; l'API pubblica non cambia):

| File | Contenuto |
|---|---|
| `ocr.go` | tipi `Word`, `Line`, `Result`; `Recognize`, `RecognizeMany`; scelta del backend; `Backend()`; `errTooLarge` |
| `ocr_native_windows.go` | backend nativo: COM/WinRT a mano (vtable) con `syscall` e `golang.org/x/sys/windows`, niente cgo |
| `ocr_bitmap.go` | lettura PNG/JPEG/GIF con la libreria standard, conversione in Bgra8 "Straight" (`fillBGRA`) |
| `ocr_powershell.go` + `winocr.ps1` | il vecchio backend, invariato (ripiego) |
| `ocr_native_other.go` | segnaposto per i sistemi non Windows |
| `ocr_test.go`, `ocr_native_windows_test.go` | test (i test del motore saltano con `t.Skip` se manca) |

`go.mod`: `golang.org/x/sys` passa da indiretta a diretta (stessa versione `v0.1.0`, gia' nella cache dei moduli).

### Come lavora il backend nativo

1. `combase.dll`: `RoInitialize(MTA)`, `RoGetActivationFactory`, `WindowsCreateString` / `WindowsDeleteString` / `WindowsGetStringRawBuffer`.
2. Un thread dedicato, mai terminato, tiene vivo il MTA per tutta la vita del processo. Senza, quando l'ultimo thread di lavoro fa
   `RoUninitialize` l'apartment viene smontato e la chiamata dopo fallisce (`RoGetActivationFactory` -> `0x80080008`).
3. Ogni chiamata a `RecognizeMany` gira in una goroutine propria con il thread bloccato (`runtime.LockOSThread`, mai sbloccato: il
   thread muore con la goroutine e non lascia stato COM a nessuno).
4. Factory `Windows.Media.Ocr.OcrEngine` (`IOcrEngineStatics`: `MaxImageDimension`, `TryCreateFromUserProfileLanguages`), `IOcrEngine`
   (`RecognizeAsync`, `RecognizerLanguage`), `IOcrResult.Lines`, `IOcrLine` (`Words`, `Text`), `IOcrWord` (`Text`, `BoundingRect`).
   Gli `IVectorView<T>` si leggono con `Size` e `GetAt` (stessa vtable per ogni `T`); gli `IAsyncOperation<T>` si usano senza IID
   parametrizzato: si interroga `IAsyncInfo` (`Status`, `ErrorCode`, `Cancel`, `Close`) e si chiama `GetResults` (indice 8).
5. Bitmap: l'immagine non passa da `BitmapDecoder`. Si decodifica con `image/png` e si crea un `SoftwareBitmap` Bgra8 con alfa `Straight`
   da un `Windows.Storage.Streams.Buffer` (`IBufferFactory::Create`, scrittura con `IBufferByteAccess::Buffer`, `put_Length`,
   `ISoftwareBitmapStatics::CreateCopyWithAlphaFromBuffer`).
6. L'attesa e' a polling su `IAsyncInfo::get_Status`; se il contesto scade o viene annullato si chiama `IAsyncInfo::Cancel` e si
   attende al massimo 1 s che il motore si fermi. Il tetto resta 90 s a chiamata.
7. Velocita': la decodifica PNG delle immagini successive va in parallelo al lavoro del motore (3 in anticipo) e fino a 3 immagini sono
   nel motore insieme, ciascuna con un `OcrEngine` suo (un solo `OcrEngine` rifiuta due `RecognizeAsync` sovrapposte: `E_ABORT`
   `0x80004004`). I risultati escono nell'ordine delle immagini.
8. I riquadri sono `float32` del motore: si riportano al `float64` "corto" (12.3 e non 12.300000190734863), cioe' il valore che il JSON
   di PowerShell dava.

Indici delle vtable e IID (verificati su Windows 11 26300: se fossero sbagliati si avrebbe `E_NOINTERFACE` o risultati assurdi):
`IOcrEngineStatics` `{5BFFA85A-3384-3540-9940-699120D428A8}` (MaxImageDimension 6, TryCreateFromUserProfileLanguages 10),
`ISoftwareBitmapStatics` `{DF0385DB-672F-4A9D-806E-C2442F343E86}` (CreateCopyWithAlphaFromBuffer 10),
`IBufferFactory` `{71AF914D-C10F-484B-BC50-14BC623B3A27}` (Create 6), `IBufferByteAccess` `{905A0FEF-BC53-11DF-8C49-001E4FC686DA}` (Buffer 3),
`IAsyncInfo` `{00000036-0000-0000-C000-000000000046}`.

## Scelta del backend

| `LIBRETTO_OCR` | Comportamento |
|---|---|
| vuota o altro valore (automatico, predefinito) | nativo. Se all'avvio non parte (API assente, nessuna lingua OCR installata, `combase.dll` mancante) o se una chiamata nativa fallisce (non per contesto annullato e non per immagine troppo grande) ripiega su PowerShell e lo scrive su stderr **una sola volta** per processo: `libretto: OCR nativo non disponibile (...): uso PowerShell` |
| `native` | solo nativo, nessun ripiego: l'errore viene restituito |
| `powershell` | solo il vecchio percorso con `winocr.ps1` |

`ocr.Backend()` dice quale backend userebbe ora `RecognizeMany` (`"native"` o `"powershell"`).

Per riavere subito il vecchio comportamento: impostare `LIBRETTO_OCR=powershell` nell'ambiente del processo (per `PanamaBook.exe`
basta impostarla prima di avviarlo: il processo figlio `libretto.exe` la eredita).

## Limiti noti

- Formati: il nativo legge PNG, JPEG e GIF con la libreria standard (BMP, TIFF e altri: errore, e in modalita' automatica si ripiega su
  PowerShell). Il programma passa sempre PNG scritti da `imgproc.SavePNG`. Per i PNG i pixel sono identici byte per byte a quelli di
  `BitmapDecoder` (grigio 8 bit, RGB, RGBA, 1560x1093 e 1368x216 verificati); per i JPEG il decodificatore di Go e quello di Windows
  possono differire di qualche livello (campionamento del colore), quindi non si garantisce l'identita'.
- PNG a 16 bit: si riducono a 8 bit con il troncamento di `image/color`; WIC potrebbe arrotondare diversamente. Non usati dal programma.
- Il motore e' quello delle lingue del profilo utente (oggi solo `it-IT`): se il profilo non ne ha, il nativo ripiega su PowerShell,
  che darebbe lo stesso errore ("nessun motore OCR"). Nessuna novita' rispetto a prima.
- La decodifica PNG di Go (circa 80 ms per una pagina intera) e' piu' lenta di quella di Windows; per una sola pagina intera e' il costo
  che resta, ma si paga una sola volta e niente avvio di PowerShell (circa 1 s).
- Non e' stato provato il comportamento sotto l'antivirus/EDR dell'utente: qui si e' eliminato il processo PowerShell, non si puo'
  escludere che altro venga fermato. Al posto di un processo figlio c'e' solo il carico di `Windows.Media.Ocr.dll` nel processo di `libretto.exe`.

## Verifica (2026-10-23, candidato `_out\libretto_ocr.exe`, 7.427.584 byte, sha256 `66192ad74871`)

Tutto su copie o in sola lettura; nessuno scanner; nessun processo terminato.

**1. Pixel.** Il `SoftwareBitmap` che `BitmapDecoder` produce per ogni PNG e' sempre Bgra8 con alfa Straight (anche per il grigio 8 bit:
`BitmapDecoder.BitmapPixelFormat` dice Rgba8 o Bgra8, ma `GetSoftwareBitmapAsync()` converte). I byte scaricati con `CopyToBuffer` da PowerShell
e quelli di `fillBGRA` coincidono **byte per byte** su 5 PNG (grigio 8 bit 64x32 e 1368x216, RGB 64x32, RGBA 64x32 e 1560x1093).

**2. Testo e riquadri, OCR diretto (stesse immagini, un backend alla volta).**

| Insieme | Immagini | Testo righe uguale | Parole uguali | Riquadri identici | Scarto massimo |
|---|---|---|---|---|---|
| pagine intere e scansioni (`scans\` e `scans\vecchi test WIA\`, in sola lettura) | 72 | 72 | 72 (2160 parole) | 2160/2160 | 0 px |
| ritagli di righe MRZ, celle e pagine di `-debug` (9 immagini) | 309 | 309 | 309 (786 parole; 162 immagini senza testo, uguali anche queste) | 786/786 | 0 px |

Anche `Width`, `Height` e `Language` coincidono. Lo scarto di riquadro e' zero perche' lo stesso motore riceve gli stessi pixel.

**3. `libretto extract` completo** su 10 copie (pagine intere, grezze, strisce MRZ): il JSON in uscita e' **identico byte per byte**
con `LIBRETTO_OCR=powershell` e `LIBRETTO_OCR=native`, e stderr vuoto.

**4. Harness** `_out\regress.exe -exe _out\libretto_ocr.exe -baseline docs\baseline-regressione.tsv -truth scans\verita` (3 processi, nessun'altra
prova dell'harness in corso da parte mia): con `LIBRETTO_OCR=native` MRZ 43/70, check 5/5 43/70, check 215/350, campi 586/980, **giusti 540/598**, nomi MRZ 41/41,
**0 regressioni**, 0 miglioramenti, 50 invariate (le altre 20 "nuove" e 14 "assenti" sono solo il percorso: le scansioni stanno ora in `scans\vecchi test WIA\`,
la baseline le ha con il vecchio percorso). Per accoppiare tutte le 70 immagini lo stesso eseguibile e' stato provato anche con
`LIBRETTO_OCR=powershell` e le due tabelle confrontate con `regress -baseline PS.tsv -against NATIVO.tsv`: **70 invariate su 70, nessuna differenza in nessuna colonna**
(rot, mrz, check, campi, avvisi, giusti, nomi e le tre mappe di simboli).

**5. Tempi** (stessa macchina, un processo alla volta salvo l'harness):

| Misura | PowerShell | Nativo |
|---|---|---|
| una chiamata OCR, 1 immagine piccola (ritaglio MRZ) | 946 ms | 25 ms |
| una chiamata OCR, 5 ritagli MRZ | 943-969 ms | 39 ms |
| 309 ritagli e pagine in un'unica chiamata | 6,1 s | 1,9 s |
| 72 pagine intere in un'unica chiamata (nativo senza parallelismo: 21,3 s; con la sola decodifica in anticipo: 15,2 s) | 18,2 s | 7,9 s |
| `libretto extract` su 10 copie, somma dei tempi | 136 s | 36 s (3,8 volte) |
| harness, 70 immagini, 3 processi | 320 s | 87 s |

Il tempo che resta nell'estrazione nativa (3-4 s a immagine) e' elaborazione d'immagine e lettura a celle in Go, non OCR.

**6. Test.** `go test ./internal/ocr` (15 test piu' 2 opzionali): conversione dei pixel, lettura di un'immagine sintetica disegnata con un font a punti
(testo e riquadri), ordine dei risultati con piu' immagini in volo, immagine bianca, immagine oltre `MaxImageDimension` (`errTooLarge`, nessun ripiego), file mancante,
contesto annullato, **annullamento a meta' lavoro** (`IAsyncInfo::Cancel`, ritorno rapido, backend ancora utilizzabile), chiamate concorrenti, scelta del backend.
Se il motore OCR manca (`nativeProbe` fallisce) i test del motore saltano con `t.Skip`. Con `OCR_TEST_PS=1` (avvia PowerShell, quindi non di default) si provano anche
l'uguaglianza nativo/PowerShell su un'immagine sintetica e il ripiego con il nativo guasto (avviso su stderr una sola volta; con `LIBRETTO_OCR=native` nessun ripiego).
`gofmt`, `go vet ./...` e `go test ./...` puliti su tutti i pacchetti (compreso `cmd/panamabook`).

## Resti

File temporanei di lavoro spostati in `_da_eliminare\ocr_prima_nativo\` (da cancellare a cura dell'utente): `ocr.go.bak` e `winocr.ps1.bak` (copie dello stato precedente),
`probe_tmp_test.go.txt`, `prof_tmp_test.go.txt`, `pix_tmp_test.go.txt` (sonde di laboratorio `[TEMPORANEO]`). In `%LOCALAPPDATA%\Temp\ocrnat_probe\` ci sono copie di immagini e
immagini di debug con dati personali create per il confronto: da cancellare a mano.
