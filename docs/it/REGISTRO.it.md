> **Documento storico (in italiano).** Registro della codebase tenuto durante lo sviluppo; puo' essere indietro rispetto al codice. Vedi `AI_CONTEXT.md`.

# Registro della Codebase - libretto (module `scanlibretto`)

Formato definito nel CLAUDE.md globale dell'utente. Compilato leggendo il codice **attuale** il 2026-10-22 (sera), dopo l'uscita JSON in inglese, le opzioni
`-page-only`/`-len`/`-from`/`-width`/`-raw`/`-driver`, il client TWAIN e il round 4 del VIZ (ancora in corso: `viz.go` cambia mentre si scrive).
`go vet ./...` pulito (anche con `GOARCH=386`); `go test ./...` ok con **176 test unitari passati e 1 saltato** (cmd/libretto 2, imgproc 20 + 1 saltato, libreta 73, mrz 10, twain 42, wia 29;
cmd/twainscan, cmd/twainprobe e ocr senza test); **10 test di integrazione** con build tag `integration` (libreta: 4 MRZ, 1 sweep, 3 campi stampati, 1 spessore del tratto; imgproc: 1 ritaglio
della pagina dati; non eseguiti in questa sessione). `go vet -tags integration ./...` **fallisce** per un file temporaneo dell'agente VIZ, `internal/libreta/viz_tmp_integration_test.go`
(`undefined: imageGrayAlias`, lavoro in corso: vedi `todo_e_criticita`). Copertura delle istruzioni: mrz 91%, imgproc 78%, twain 74%, libreta 71%, wia 35%, cmd/libretto 14%, ocr 0%.
Codice Go: circa 10700 righe non di test in `cmd/` e `internal/` (15900 con i test); `viz.go` da solo circa 2400 righe (72 KB), in crescita.
Misure attuali (8 immagini, `docs/agenti/README.md`): MRZ con tutti i check digit su tutte; campi stampati trovati 13-14 su 14; da 5,5 a 13,3 s per immagine.

```yaml
registro_codebase:
  ultimo_aggiornamento: "2026-10-23 (aggiornate solo le voci dell'integratore: extract.go, cmd/libretto/summary.go, cmd/regress, internal/outsum; il resto e' della passata del 2026-10-22 sera, 19:35)"
  ultima_pulizia: "2026-10-22 (riordino: i resti degli agenti, fino al round 5, sono in _da_eliminare/, non cancellati; le scansioni di prova sono state spostate dall'utente in scans/vecchi test WIA/; vedi Pulizia proposta)"

  moduli_esistenti:
    - nome: "CLI libretto"
      file: "cmd/libretto/main.go"
      responsabilita: "Punto d'ingresso. Comandi info (diagnostica WIA), scan (una facciata), extract (da immagine), read (scan + extract) e area-probe (nascosto: solo proprieta' WIA, nessuna acquisizione). scan e read passano da scanAndPost: crea la cartella di uscita, chiede conferma (confirmScan: senza terminale interattivo rifiuta, serve -yes), scansiona con il driver scelto (acquire), traduce 'carta assente' con suggerimento, salva la scansione grezza se -raw (saveRaw), scarta le scansioni vuote (IsBlank), ritaglia e raddrizza (AutoCropInfo). Opzioni: -device, -dpi, -gray, -o, -yes, -force, -driver wia|twain (predefinito wia, defaultDriver), -from/-len/-width (mm; area limitata, passata a wia/twain), -raw, -page-only (solo read: il file di -o diventa la sola pagina dati 125x88 mm via extract(..., force=true); scan accetta il flag ma lo ignora), -page-out (extract e read, via libreta.DataPage), -debug (warnDebug). Nome predefinito: scans/scan-AAAAMMGG-hhmmss.png (defaultOutPath). signal.NotifyContext(os.Interrupt) per extract e read. Codici di uscita: 1 errore, 2 uso, 3 carta assente (WIA o TWAIN), 4 inceppamento, 130 Ctrl+C (exitCode). JSON su stdout, messaggi su stderr."
      funzioni_chiave: ["main", "withInterrupt", "exitCode", "usage", "cmdInfo", "cmdAreaProbe", "scanConfig", "scanFlags", "defaultOutPath", "warnDebug", "confirmScan", "scanAndPost", "saveRaw", "cmdScan", "checkPageOut", "cmdExtract", "cmdRead", "extract", "savePage"]
      test: "nessuno diretto (la copertura 14% viene dai test di output.go)"

    - nome: "CLI libretto (uscita JSON)"
      file: "cmd/libretto/output.go, cmd/libretto/output_test.go"
      responsabilita: "Costruisce il JSON stampato da extract e read (toOutput): schema_version 1, rotation_deg, mrz, mrz_corrections in inglese (correction), fields con nomi INGLESI uguali alle etichette stampate (fieldNames: type, country_code, nationality, place_of_birth, date_of_issue, date_of_expiry, date_of_birth, level, capacity, seamans_book, id_number, certificate (fino al 2026-10-23 endorsement: etichetta stampata Nº Titulo/Certificate), seafarers_id, regional + derivati MRZ surname, given_names, nationality_iso, sex, *_mrz; il suffisso _iso delle date e' conservato da fieldName) e warnings come oggetti con CODICE STABILE (toWarning: MRZ_NOT_FOUND, MRZ_CHECK_DIGITS_INVALID, PRINTED_FIELDS_NOT_READ con fields ordinati, UNCERTAIN_FIELD, PRINTED_FIELD_DIFFERS_FROM_MRZ, DATE_OF_BIRTH_NOT_BEFORE_DATE_OF_ISSUE, DATE_OF_EXPIRY_NOT_AFTER_DATE_OF_ISSUE, UNKNOWN_WARNING), senza testo libero ne' valori personali. libreta.Result resta con chiavi interne italiane: la traduzione avviene solo qui."
      funzioni_chiave: ["toOutput", "fieldName", "toWarning", "correction"]
      tipi: ["output", "warning"]
      test: "output_test.go (2 test: nomi inglesi senza parole italiane, codici degli avvisi)"

    - nome: "CLI libretto (driver TWAIN)"
      file: "cmd/libretto/twaindrv.go"
      responsabilita: "Sceglie il driver (acquire: wia o twain, altro = errore) e, per twain, cerca twainscan32.exe (accanto a libretto.exe, poi cartella corrente, poi _out/) e lo lancia con -yes -ignora-sensore, -o, -dpi, -from, -len, -width, -gray, -force; il suo stdout va su stderr perche' lo stdout di libretto e' il JSON. Le uscite 3 e 4 del programma diventano errTwainNoPaper e errTwainJam (stessi codici di uscita di libretto). NOTA: -ignora-sensore e' SEMPRE passato, quindi il controllo del sensore dell'alimentatore non c'e' su questo percorso."
      funzioni_chiave: ["acquire", "findTwainHelper", "scanTwain"]
      tipi: []
      test: "nessuno"

    - nome: "twainscan (programma ausiliario TWAIN)"
      file: "cmd/twainscan/main.go"
      responsabilita: "AVVIA LO SCANNER via TWAIN (PaperStream IP, solo 32 bit: si compila con GOARCH=386 come twainscan32.exe). Opzioni -from, -len, -width, -dpi, -gray, -o, -yes, -force, -ignora-sensore. Chiede di digitare SI (salvo -yes), non sovrascrive mai un file (salvo -force), una pagina, nessun nuovo tentativo; scrive il PNG GREZZO in scans/twain-AAAAMMGG-hhmmss.png. Uscite 3 (carta assente) e 4 (inceppamento) come libretto. Lo lancia libretto -driver twain oppure l'utente a mano."
      funzioni_chiave: ["main"]
      test: "nessuno"

    - nome: "twainprobe (sonda TWAIN)"
      file: "cmd/twainprobe/main.go"
      responsabilita: "Diagnostica TWAIN, NON scansiona mai (non chiama MSG_ENABLEDS). Stampa data source e capability; con -try-frame scrive e rilegge ICAP_FRAMES (alto 90 mm) e lo ripristina; -all, -layout. Compilato come _out/twainprobe32.exe (funziona) e _out/twainprobe64.exe (spiega cosa manca per il driver a 64 bit)."
      funzioni_chiave: ["main"]
      test: "nessuno"

    - nome: "twain (client TWAIN)"
      file: "internal/twain/ (session.go, frames.go, area.go, container.go, layout.go, win.go [windows], dib.go, probe.go, capnames.go [352 voci], errors.go)"
      responsabilita: "Client TWAIN 2.x indipendente dall'architettura (32 e 64 bit): strutture TW_* a 2 byte costruite a mano in []byte con layout{ptr: 4|8}; trasporto astratto (interface transport) cosi' i test usano un data source finto e il DSM reale si carica solo in win.go. Scan/ScanInfo(Options{DeviceMatch, DPI, Gray, StartMM, HeightMM, WidthMM, Timeout, IgnoreFeederLoaded}): apre DSM e data source senza finestre, spegne il duplex e lo rilegge, una sola pagina (CAP_XFERCOUNT=1), controlla CAP_FEEDERLOADED, imposta ICAP_FRAMES con 'ricette' (formato predefinito, NONE, MAXSIZE, USLEGAL, carta personalizzata, cm, pixel, capability EXT, riquadro stretto) e rilegge: se il driver cambia l'alto annulla PRIMA di avviare lo scanner. Trasferimento nativo DIB -> image.Image, mai ritentato. Probe: elenco capability con flag e valore, mai ENABLEDS. Misurato: il driver ACCETTA l'alto del riquadro; NON verificato che il motore salti i primi mm con un documento."
      funzioni_chiave: ["Scan", "ScanInfo", "Probe", "ArchitectureHint", "IsPaperEmpty", "IsPaperJam", "newSession", "session.call", "session.prepare", "session.scan", "session.enableAndTransfer", "session.transfer", "session.applyRecipe", "session.setAreaFrame", "frameForArea", "recipes", "decodeDIB", "parseContainer", "Container.Current", "Container.Allows", "dsmCandidates", "unitsToMM"]
      tipi: ["Options", "Info", "ProbeOptions", "Frame", "Container", "Item", "Error"]
      test: "internal/twain/*_test.go (42 test, data source finto, strutture per 4 e 8 byte; nessun test avvia lo scanner; copertura 74%)"

    - nome: "wia (acquisizione)"
      file: "internal/wia/wia.go, options.go, area.go, probe.go, errors.go, bmp.go, outpath.go"   # wia.go e probe.go hanno //go:build windows
      responsabilita: "Acquisizione dal fi-800R via WIA/COM (go-ole). Imposta Document Handling Select=1 (alimentatore senza duplex) e Pages=1, risoluzione, colore/grigio, area (massima o limitata); salva PNG (converte BMP). Ogni operazione pubblica usa runtime.LockOSThread + apartment STA + timeout (Options.Timeout, default 2 min). Errori WIA tradotti in italiano con l'HRESULT. Riprova solo su Connect, mai su Transfer. Area limitata (Options.StartMM/HeightMM/WidthMM, checkAreaMM, areaPixels, applyArea): per ogni asse prima lo start, poi si rilegge il massimo dell'extent, poi l'extent, infine si rileggono tutti i valori; qualunque scostamento annulla prima dell'acquisizione. Il driver ACCETTA ma IGNORA lo start verticale (misurato: spostamento 0,0 mm). outpath.go: checkOutPath verifica la destinazione PRIMA di toccare lo scanner e di nuovo al salvataggio; temporanei con nome casuale accanto all'uscita (tempFiles), rimossi sempre; il file finale arriva con rename. probe.go: ProbeArea (sottocomando nascosto area-probe)."
      funzioni_chiave: ["Scan", "Describe", "List", "Dump", "ProbeArea", "scanItem", "limitArea", "applyArea", "checkAreaMM", "areaPixels", "finalizeImage", "withManager", "withDevice", "eachDeviceInfo", "runWithTimeout", "explain", "hresultOf", "describeHRESULT", "IsPaperEmpty", "IsPaperJam", "decodeBMP", "Options.normalized", "checkDPI", "checkSimplex", "checkOutPath", "newTempName", "tempFiles.new", "tempFiles.cleanup"]
      tipi: ["Options", "Info", "DeviceInfo", "Prop", "areaProps"]
      test: "internal/wia/wia_test.go, area_test.go, outpath_test.go (29 test, nessuno tocca lo scanner; area con un driver finto; copertura 35%)"

    - nome: "imgproc"
      file: "internal/imgproc/imgproc.go, rows.go, autocrop.go, deskew.go, pagecrop.go"
      responsabilita: "Elaborazioni di immagine con la sola libreria standard. imgproc.go: I/O PNG/JPEG, rotazione, ritaglio semplice CropContent (soglia fissa 225), scala bilineare. rows.go: binarizzazione, cancellazione di bordi scuri, righe di testo per proiezione, scarto delle barre di bordo (TrimRowEdgeBars), coppie di righe compatibili con la MRZ, ritaglio riga, misura e riduzione dello spessore del tratto per le stampe laser a 300 dpi (StrokeWidth, Erode, GrayCrop, InkFraction, ThinToInk, PadWhite). autocrop.go: ritaglio automatico del documento a TEXTURE (lowRes a ~800 px, sfondo = mediana della cornice, 'documento' = scarto dallo sfondo con deviazione standard locale alta o forte contrasto, componenti connesse, scarto di polvere, inclinazione +-6 gradi, raddrizzamento) e rilevamento scansione vuota. deskew.go (NUOVO): EstimateSkew (+-12 gradi, nitidezza della proiezione), RotateAngle, Deskew (non ruota sotto 2 gradi: i facsimili digitali danno 1,3-1,5 gradi spurii); usata da Extract, DataPage e readMRZBand. pagecrop.go: ritaglio della pagina dati TD3 (125x88 mm) dalla sola geometria delle due righe MRZ, senza OCR (passo 2,54 mm, 44 caratteri; td3BottomMM = 7,5 mm, affinamento sui bordi con refineWithEdges)."
      funzioni_chiave: ["Load", "SavePNG", "Rotate", "CropContent", "Scale", "Binarize", "ClearLines", "TextRows", "TrimRowEdgeBars", "MRZPairs", "MainSpan", "LowestPair", "CropRow", "StrokeWidth", "Erode", "GrayCrop", "InkFraction", "ThinToInk", "PadWhite", "IsBlank", "AutoCrop", "AutoCropInfo", "analyze", "localStd", "trimBorder", "estimateSkew", "rotateFill", "EstimateSkew", "RotateAngle", "Deskew", "CropDataPage", "DataPageGeometry", "geometryFromRows", "refineWithEdges"]
      tipi: ["Row", "CropInfo", "PageGeometry"]
      test: "autocrop_test.go, deskew_test.go, pagecrop_test.go (20 passati, 1 saltato: autocrop_diag_test.go e' opt-in con AUTOCROP_FILES); pagecrop_integration_test.go (1, build tag integration); nessun test diretto per rows.go e imgproc.go; copertura 78%"

    - nome: "ocr"
      file: "internal/ocr/ocr.go, ocr_native_windows.go, ocr_bitmap.go, ocr_native_other.go, ocr_powershell.go, winocr.ps1 (agente libretto-ocr, docs/agenti/16-ocr-nativo.md)"
      responsabilita: "OCR con Windows.Media.Ocr. PREDEFINITO: backend nativo (COM/WinRT a mano con syscall e x/sys/windows, senza cgo e senza processi figli; fino a 3 immagini insieme nel motore), perche' un controllo di sicurezza del PC termina intermittente i processi PowerShell (0xC0400065) e uccide l'albero di PanamaBook.exe. Ripiego automatico su powershell.exe (script winocr.ps1 incorporato con go:embed, BOM UTF-8, timeout 90 s) se il nativo non parte, avviso una volta su stderr; LIBRETTO_OCR=powershell forza il vecchio, LIBRETTO_OCR=native esclude il ripiego. Stesso JSON del vecchio percorso (misurato con regress: 64 invariate, 0 regressioni), 3,7 s contro 13,1 s per immagine. Restituisce righe e parole con riquadri in pixel; RecognizeMany legge piu' immagini in una chiamata."
      funzioni_chiave: ["Recognize", "RecognizeMany"]
      tipi: ["Result", "Line", "Word"]
      test: "nessuno (copertura 0%)"

    - nome: "mrz"
      file: "internal/mrz/mrz.go, internal/mrz/fix.go, internal/mrz/mrz_test.go, internal/mrz/fix_test.go"
      responsabilita: "Parser ICAO 9303 TD3 (2 righe da 44), check digit pesi 7-3-1, normalizzazione per classe di posizione (cifra/lettera) e riparazione degli errori OCR di un carattere confermata dal controllo finale. L'orologio e' iniettabile (var now = time.Now, per il secolo della data di nascita) cosi' i test sono deterministici. Le chiavi JSON di mrz.Data (doc_code, issuer, surname, ...) sono gia' inglesi."
      funzioni_chiave: ["Parse", "CheckDigit", "Checks.AllOK", "Normalize", "Repair", "candidates"]
      tipi: ["Data", "Checks", "Correzioni"]
      test: "mrz_test.go (6) e fix_test.go (4); copertura 91%"

    - nome: "libreta (orchestrazione)"
      file: "internal/libreta/extract.go, internal/libreta/regex.go"
      responsabilita: "Extract: carica, CropContent, Deskew (una volta sola, prima delle rotazioni), prova le 4 rotazioni con findOrientation (per ciascuna readMRZOriented: lettura a banda readMRZBand e, se c'e' un lettore registrato nel punto di aggancio mrzCellsReader, SEMPRE anche il lettore MRZ a template di mrzcells.go sulla stessa immagine; se la banda non trova l'MRZ o non tornano tutti i check digit si tiene il punteggio piu' alto (a parita' l'OCR); se la banda ha tutti i check digit il lettore a template fa da seconda lettura per riga 1, nazionalita' e sesso, che i check digit non coprono (reconcileMRZ in mrzmerge.go: si prende la sua lettura dove le due differiscono e nessuna cella diversa e' segnalata incerta nelle sue note, altrimenti resta l'OCR con un avviso MRZ_LINE1_UNCERTAIN o MRZ_FIELD_UNCERTAIN); callCellsReader intercetta un panic del lettore) e si ferma alla prima rotazione con tutti i check digit, prepara l'immagine VIZ, OCR, ripiego findMRZ, extractVIZ, crossCheck (campi derivati dall'MRZ e avvisi), finalizeVIZ, salvataggio debug. DataPage(path, rotation) ricarica l'immagine, applica CropContent, Deskew e la rotazione trovata e chiama imgproc.CropDataPage (usata da savePage del CLI per -page-out e -page-only). Result ha ancora chiavi interne italiane (cognome, nomi, sesso, ...): la traduzione in inglese e' in cmd/libretto/output.go."
      funzioni_chiave: ["Extract", "DataPage", "findOrientation", "readMRZOriented", "callCellsReader", "crossCheck", "autoScale", "dump", "firstMatch", "regexpCache", "minf", "maxf", "absf"]
      tipi: ["Result", "Options", "mrzBandFunc", "mrzRead"]
      variabili: ["mrzCellsReader (punto di aggancio, nil di default: il lettore a template si registra con init() nel suo file; contratto in docs/agenti/COORDINAMENTO.md)"]
      test: "extract_hook_test.go (12 test con un lettore finto, senza OCR)"

    - nome: "libreta (seconda lettura dell'MRZ)"
      file: "internal/libreta/mrzmerge.go, mrzmerge_test.go"
      responsabilita: "Confronto tra la lettura OCR dell'MRZ (con tutti i check digit) e quella del lettore a template sulle parti che i check digit non coprono: riga 1 (cognome e nomi; contano le lettere, non i riempimenti '<'), nazionalita' (celle 11-13) e sesso (cella 21) della riga 2. Dove differiscono e nessuna delle celle diverse e' segnalata nelle note del lettore (formato 'rigaN pos P: ...', flaggedCells) si prende la lettura a template e si scrive una correzione; se una e' segnalata resta l'OCR e si restituisce un avviso (warnMRZLine1, warnMRZFieldPrefix). Le parti coperte dai check digit restano quelle dell'OCR e il risultato e' riletto con mrz.Parse: i check digit non peggiorano mai. cmd/libretto/output.go traduce gli avvisi in MRZ_LINE1_UNCERTAIN e MRZ_FIELD_UNCERTAIN (docs/campi.md) e correction() le correzioni in inglese."
      funzioni_chiave: ["reconcileMRZ", "checkedPart", "diffPositions", "lettersOf", "flaggedCells", "anyFlagged"]
      tipi: []
      test: "mrzmerge_test.go (8 test con dati inventati)"

    - nome: "libreta (righe MRZ)"
      file: "internal/libreta/mrzband.go"   # circa 690 righe
      responsabilita: "Lettura delle due righe MRZ: Deskew, ritaglio per righe (Binarize, ClearLines, TextRows, TrimRowEdgeBars, MRZPairs), OCR a 3 scale in una sola chiamata, riparazione con check digit (bestReading, fitLength, scoreMRZ). Tratti spessi: varianti assottigliate (thinVariants: erosione di 1-2 px e soglia scelta per frazione di inchiostro, thinInkTargets); con tratto spesso (strokeRatio >= thickStrokeRatio) le varianti normali e assottigliate vanno nella stessa chiamata OCR, altrimenti in due. Lettura a pezzi sulle 44 celle con voto per cella quando una riga resta vuota (cellGrid, pieceSpecs, voteLine). Nessun OCR se non c'e' una coppia di righe. Ripiego sull'OCR della pagina intera (findMRZ)."
      funzioni_chiave: ["readMRZBand", "strokeRatio", "thinVariants", "rowVariants", "newCellGrid", "cellGrid.cut", "cellGrid.crop", "cellGrid.pieces", "pieceVotes.add", "voteLine", "ocrImages", "bestReading", "fitLength", "scoreMRZ", "mrzClean", "findMRZ", "finish", "debugNote", "appendUnique"]
      test: "mrzband_test.go (12 test sintetici); mrzband_integration_test.go (4) e mrzband_sweep_integration_test.go (1), build tag integration"

    - nome: "libreta (campi stampati)"
      file: "internal/libreta/viz.go (circa 2400 righe, 72 KB), viz_vote.go (voto), viz_page.go (modo pagina di ripiego), viz_jitter.go, viz_nationality.go"
      responsabilita: "Lettura dei 14 campi stampati. viz.go: modo foglio (vizLocateMRZ + vizSkew trovano la MRZ, vizBuildSheet ritaglia ogni campo: 15 fasce x 3 varianti, foglio 1500 px), extractVIZ legge il foglio e i ritagli singoli (vizOCRCrops), riconoscimento per formato (date, libretto, num9, identificazione, livello, testo), vizFinalize (normalizza e confronta con l'MRZ, avvisi; il tipo mancante e' ricavato da mrz.doc_code invertendo le lettere con avviso 'campo incerto: tipo'). Assottigliamento adattivo (vizAdaptiveThreshold misura lo spessore del tratto sulle righe MRZ) con 11 varianti binarizzate come soli ritagli (vizExtraVariants). viz_vote.go: voto per carattere con pesi misurati sulle cifre note dall'MRZ (vizMRZWeights, vizVoteAll, vizPosVote, vizTextVote*), vizApplyMRZ, avvisi 'campo incerto' (vizWarnUncertain). viz_page.go: ripiego senza MRZ su tre varianti della pagina (base, contrasto stirato, rampa morbida; vizPageThr = 48). viz_jitter.go (NUOVO): ritagli spostati di 3-5 px per i soli testi liberi luogo_nascita, cargo, regionale sulle scansioni a tratto spesso (circa 24 ritagli). viz_nationality.go (NUOVO): tabella ISO 3166-1 alpha-3 -> aggettivi inglesi (199 codici), vizExpectedNationality (soglia vizNatSim = 0,5), vizDemonyms."
      funzioni_chiave: ["prepareVIZImage", "vizLocateMRZ", "vizBinRows", "vizSkew", "vizBuildSheet", "vizStrokeRatio", "vizPickThreshold", "vizAdaptiveThreshold", "vizExtraVariants", "vizJitterJobs", "vizCropVals", "extractVIZ", "vizExtractSheet", "vizExtractPage", "vizExtractPageSet", "vizPreparePage", "vizReadBand", "vizVoteAll", "vizVote", "vizVoteLibretto", "vizPosVote", "vizTextVote", "vizApplyMRZ", "vizPickLongest", "vizOCRCrops", "vizExpectedNationality", "vizSnapNationality", "vizSetStash", "vizTakeStash", "vizSetReadings", "vizTakeReadings", "vizDates", "vizLibretto", "vizNum9", "vizIdent", "vizLevel", "vizFinalize", "finalizeVIZ", "vizSim", "vizLev"]
      tipi: ["vizBand", "vizVariant", "vizGeom", "vizCropSet", "vizCropWords", "vizJitter", "vizPageSet", "vizCand", "vizReadings", "vizLabel", "box", "word", "vizLine", "vizDate"]
      test: "viz_test.go, viz_vote_test.go, viz_nationality_test.go (73 test in tutto il pacchetto libreta con mrzband); viz_integration_test.go (3) e viz_stroke_integration_test.go (1), build tag integration"

    - nome: "CLI libretto (summary)"
      file: "cmd/libretto/summary.go"
      responsabilita: "Comando summary FILE.json: riassume in una riga, senza dati personali, un JSON prodotto da extract o read (rotazione, MRZ si/no, check digit su 5, campi trovati su 14, avvisi, nomi dei campi mancanti). L'elenco dei 14 campi e dei 5 check digit non e' piu' scritto qui ma in internal/outsum."
      funzioni_chiave: ["cmdSummary"]
      test: "nessuno diretto (la logica e' in outsum, con test)"

    - nome: "CLI libretto (pdf)"
      file: "cmd/libretto/pdf.go; voce 'case pdf' e riga di uso in cmd/libretto/main.go; tools/pdf.cmd"
      responsabilita: "Comando pdf -i scan.png [-o scan.pdf] [-max 1000000] [-dpi 300] [-force]: PDF di una pagina entro un peso massimo (il gestionale accetta allegati fino a 1 MB; si usano 1.000.000 byte). Non usa lo scanner. Carica l'immagine (imgproc.Load), chiama pdfout.Encode e scrive il PDF (permessi 0600); rifiuta un PDF esistente senza -force; ricorda su stderr che il PDF contiene dati personali; stampa dimensione, qualita' JPEG, scala dell'immagine. Uscita predefinita: come l'immagine con estensione .pdf. tools/pdf.cmd NOME: per l'utente, legge scans\\NOME.png e crea scans\\NOME.pdf (ripiega su _out/libretto_pdf.exe finche' libretto.exe non ha il comando). Scritto dal committente."
      funzioni_chiave: ["cmdPDF", "maxPDFBytes"]
      test: "nessuno diretto (la logica e' in pdfout, con test)"

    - nome: "CLI libretto (rename) e internal/filename"
      file: "cmd/libretto/rename.go, internal/filename/filename.go (+ test); voce 'case rename' in cmd/libretto/main.go; chiamata da tools/estrai.cmd"
      responsabilita: "libretto rename -json scans\\NOME.json [-n]: rinomina .png/.json/.pdf/.err con lo stesso nome base 'NOME - COGNOME - NUMERO LIBRETTO' (renameBase, renameAll con rollback tutto-o-niente), solo con MRZ valida (5 check) e senza MRZ_LINE1_UNCERTAIN; -n stampa soltanto. internal/filename: Clean (solo lettere/cifre ASCII e spazi, accenti traslitterati, max 45 caratteri per pezzo), Base(Parts{Given,Surname,Book}), Unique(dir, base, exts...) ('nome (2)'). Un JSON con BOM non si legge (le redirezioni di cmd.exe non lo scrivono). Scritto dal committente. In cmd/libretto/output.go toOutput scambia surname e given_names (e i gruppi del blocco mrz) per IS+PAN perche' nei libretti di Panama i nomi stanno prima del cognome (test TestToOutputOrdineNomiPanama); il dato interno resta in ordine ICAO."
      funzioni_chiave: ["cmdRename", "renameBase", "renameAll", "filename.Clean", "filename.Base", "filename.Unique"]
      test: "filename_test.go; output_test.go (ordine dei nomi)"

    - nome: "libreta/namecheck (nome stampato contro MRZ)"
      file: "internal/libreta/namecheck.go, namecheck_test.go; chiamata checkPrintedName(best) in extract.go dopo finalizeVIZ; avviso convertito in cmd/libretto/output.go (toWarning)"
      responsabilita: "Confronta il nome stampato letto dal VIZ (vizPrintedName, viz_name.go) con cognome+nomi dell'MRZ: parole senza ordine e senza accenti (nameTokens, namesDiffer). Avviso interno 'nome stampato diverso dall'MRZ' (codice MRZ_NAME_DIFFERS_FROM_PRINTED, senza valori) solo se la confidenza e' >= printedNameMinConf (0,9) e il campo nomi della riga 1 non e' tutto occupato (mrzNamesTruncated). La riga 1 non ha check digit. Soglia scelta sul corpus: sotto 0,9 i falsi allarmi superano i veri (un caso a 0,88 e' un errore del nome stampato con MRZ giusta); a 0,9 il corpus non produce avvisi."
      funzioni_chiave: ["checkPrintedName", "namesDiffer", "nameTokens", "mrzNamesTruncated"]
      test: "namecheck_test.go; output_test.go (TestToWarningMRZIncerto)"

    - nome: "pdfout (PDF di una pagina)"
      file: "internal/pdfout/pdfout.go, pdfout_test.go"
      responsabilita: "Scrive un PDF 1.4 di una sola pagina con l'immagine incorporata come JPEG (DCTDecode) senza altre ricodifiche: il PDF pesa quanto il JPEG piu' circa 1 KB. Encode(img, dpi, maxBytes): cerca con ricerca binaria la qualita' JPEG piu' alta (95..80) che tiene il PDF entro maxBytes; se a 80 non basta riduce l'immagine a passi del 7% (scaleStep 0,93, mai sotto la meta') e riprova; maxBytes 0 = nessun limite, qualita' 92. La misura della pagina deriva dai dpi (72 punti per pollice). Info riporta qualita', scala, pixel e byte. Misurato dal committente su una scansione 1560x1093: 949.247 byte, qualita' 92, risoluzione intera; riletta dal PDF a 300 dpi: MRZ 5/5 e campi identici."
      funzioni_chiave: ["Encode", "build", "wrap", "toRGBA"]
      tipi: ["Info"]
      test: "pdfout_test.go (4 test: limite rispettato, qualita' alta se ci sta tutto, limite impossibile, misura della pagina)"

    - nome: "outsum (riassunto numerico dell'uscita JSON)"
      file: "internal/outsum/outsum.go, outsum_test.go"
      responsabilita: "Legge il JSON di extract/read e ne ricava solo numeri: rotazione, MRZ presente, 5 check digit nell'ordine fisso CheckKeys, 14 campi stampati nell'ordine fisso FieldKeys (letti / non letti), numero di avvisi. Summary.Fields tiene anche i valori (dati personali) per i confronti in memoria: non vanno stampati. Usato da cmd/libretto/summary.go e cmd/regress. Parse legge ancora la vecchia chiave endorsement come certificate (legacyFieldKeys, alias in lettura TRANSITORIO dal 2026-10-23: da togliere quando non restano uscite con il nome vecchio)."
      funzioni_chiave: ["Parse", "Summary.ChecksOK", "Summary.Present", "Summary.Found", "Summary.Missing"]
      tipi: ["Summary"]
      test: "outsum_test.go (4 test)"

    - nome: "regress (harness di regressione)"
      file: "cmd/regress/ (main.go, corpus.go, run.go, row.go, compare.go, truth.go e i test)"
      responsabilita: "Esegue SOLO 'libretto extract -i FILE' (mai scan o read) su un corpus di immagini (scans/*.png, scans/vecchi test WIA/*.png senza *-pagina*.png, facsimili/panama/*.jpg e *.png: 63 immagini il 2026-10-23), in parallelo (-j 3), su una copia temporanea dell'eseguibile (-exe), e stampa una tabella TSV senza dati personali: file, rot, mrz, check n/5, campi n/14, avvisi, sec, giusti n/m, nomi (G/S/M: cognome e nomi della riga 1 dell'MRZ contro la riga del nome in verita', via verify.Verify), mappa_check, mappa_campi, mappa_giusti. Con -baseline confronta PER NOME (MRZ, check digit, campi letti, campi giusti): perdere anche un solo elemento e' REGRESSIONE (uscita 1), guadagnarne e' MIGLIORAMENTO; rotazione, avvisi e tempo sono solo NOTE. Con -truth CARTELLA confronta i campi con la verita' scritta a occhio (CARTELLA/NOME.json) usando verify.LoadTruth e verify.Compare di internal/verify (confronto normalizzato: maiuscole, spazi, trattini, accenti, date anche ISO) e riporta solo simboli G/S/M/X; l'intestazione della tabella porta l'impronta della verita' e il confronto avverte se due tabelle hanno verita' diverse (warnTruthChanged). Con -baseline PRIMA.tsv -against DOPO.tsv confronta due tabelle gia' prodotte, senza eseguire nulla. Con carico alto (molti processi insieme) le letture marginali perdono check digit per il tetto di tempo di mrzband.go (maxBandTime): una prova alla volta, -j 3. Non modifica le immagini (controlla dimensione e data prima e dopo). Costruito in _out/regress.exe. Baseline di libretto.exe in docs/baseline-regressione.tsv."
      funzioni_chiave: ["run", "discover", "filterFiles", "runAll", "measure", "copyExe", "describeExe", "snapshot", "changedSince", "rowFromSummary", "writeTable", "readTable", "compare", "compareRow", "delta", "loadTable", "tableMeta", "totals", "loadTruth", "scoreTruth", "truthPrint", "tablePrint", "warnTruthChanged"]
      tipi: ["Row", "config", "Finding", "Outcome", "corpusRule"]
      test: "30 test: corpus, tabella, confronto, verita' (con i nomi della riga 1) e il comando intero con un libretto finto (il binario di test stesso), senza OCR"

    - nome: "strumenti di prova"
      file: "tools/summary.py, tools/prova.cmd, tools/prova-libretto.cmd, tools/prova-pagina.cmd, tools/prova-twain.cmd"
      responsabilita: "Non fanno parte del programma. Gli script .cmd AVVIANO LO SCANNER (li lancia solo l'utente). summary.py (Python): riassume in una riga, senza dati personali, l'esito di un'immagine (lancia libretto.exe extract) o di un JSON (rotazione, MRZ si/no, check digit su 5, campi trovati su 14, avvisi, tempo); l'elenco dei 14 campi e' scritto a mano (KEYS) e va riallineato alle chiavi inglesi se usa i nomi vecchi. prova.cmd NOME [DPI]: read -yes con -page-out. prova-libretto.cmd [NOME] [DPI] [LUNGH] [DA]: libretto aperto, bordo con l'MRZ inserito per primo, default -len 190 (la pagina dati sta tra 90 e 180 mm), -page-only; ciclo 'nome della prova successiva', la finestra resta aperta. prova-pagina.cmd: foglio stampato, -len 110, -page-only. prova-twain.cmd: -driver twain -len 100 -from 90 (o LUNGH/DA indicati), -page-only. Tutti rifiutano se scans\\NOME.png o .json esistono. Richiedono libretto.exe nella radice e Python nel PATH."
      funzioni_chiave: []

    - nome: "driver scanner (script)"
      file: "drivers/PaperStream_IP_TWAIN_3.40.2_solo-driver/Installa-Solo-Driver.cmd, drivers/Installa-Driver.cmd, drivers/Installa-Driver.ps1"
      responsabilita: "Installazione del driver PaperStream IP 3.40.2 (solo driver, msiexec silenzioso) o con verifica SHA256/firma (ps1). Non e' codice Go. Il driver installato e' solo a 32 bit (data source in C:\\Windows\\twain_32\\Fjicube)."
      funzioni_chiave: []

    - nome: "driver scanner (installazione manuale)"
      file: "drivers/installazione_manuale/ (LEGGIMI.md, fujitsu/, ricoh/; 780 KB)"
      responsabilita: "INF/CAT/CAB originali del solo driver USB/WIA (versione 2.0.0.5), per installarlo a mano con pnputil o da Gestione dispositivi senza l'installer. fujitsu\\ lega VID_04C5&PID_15FC (questo scanner); ricoh\\ lega VID_05CA&PID_03D5 ed e' solo di riferimento. Non e' codice Go."
      funzioni_chiave: []

    - nome: "bacheca di coordinamento e baseline"
      file: "docs/agenti/COORDINAMENTO.md, docs/agenti/13-libretto-exe.md, docs/baseline-regressione.tsv, docs/baseline-regressione-1016.tsv"
      responsabilita: "COORDINAMENTO.md: proprietari dei file per agente, contratti (firme di readMRZBand, readMRZCells, mrzCellsReader), stato degli agenti, regole per sostituire libretto.exe (gofmt, go vet, go test e harness senza regressioni), richieste aperte. 13-libretto-exe.md: log dell'integratore. baseline-regressione.tsv: esiti numerici (nessun dato personale) del libretto.exe installato (build dai sorgenti del 2026-10-23 11:15, sha256 EF97DBB5294E) sul corpus dell'harness, con i campi giusti secondo scans/verita (impronta nell'intestazione); la baseline di libretto.exe delle 10:16 e' in baseline-regressione-1016.tsv."
      funzioni_chiave: []

    - nome: "documentazione di lavoro degli agenti"
      file: "docs/agenti/ (README.md + 01-mrz ... 11-revisione-test, 12 file; circa 57 KB)"
      responsabilita: "Un rapporto per agente (MRZ, VIZ, WIA/TWAIN, documentazione e sette revisori di architettura), con stato, misure, decisioni aperte e storico. Senza dati personali. Non fa parte del programma. 02-viz.md non ha ancora il rapporto del round 4 (nazionalita', jitter)."
      funzioni_chiave: []

    - nome: "agenti di progetto (non codice)"
      file: ".claude/agents/libretto-docs.md, libretto-mrz.md, libretto-viz.md, libretto-wia.md, libretto-exe.md, libretto-verifica.md"
      responsabilita: "Definizioni degli agenti che lavorano sul progetto (documentazione, MRZ, campi stampati, WIA, integratore, verificatore). Non fanno parte del programma."
      funzioni_chiave: []

  funzioni_utility:
    - nome: "mrz.CheckDigit"
      file: "internal/mrz/mrz.go"
      usata_in: ["internal/mrz/mrz.go (Parse)", "internal/mrz/fix.go (Repair)", "internal/mrz/mrz_test.go"]
    - nome: "mrz.Parse"
      file: "internal/mrz/mrz.go"
      usata_in: ["internal/mrz/fix.go (Repair)", "internal/mrz/mrz_test.go"]
    - nome: "mrz.Repair"
      file: "internal/mrz/fix.go"
      usata_in: ["internal/libreta/mrzband.go (bestReading, findMRZ)", "internal/mrz/mrz_test.go"]
    - nome: "imgproc.Load / SavePNG"
      file: "internal/imgproc/imgproc.go"
      usata_in: ["cmd/libretto/main.go", "cmd/twainscan/main.go", "internal/libreta/extract.go", "internal/libreta/mrzband.go (ocrImages)", "internal/libreta/viz.go (vizOCRCrops)", "internal/wia/wia.go (finalizeImage)"]
    - nome: "imgproc.Rotate / CropContent"
      file: "internal/imgproc/imgproc.go"
      usata_in: ["internal/libreta/extract.go (Extract, DataPage)"]
    - nome: "imgproc.Deskew (EstimateSkew, RotateAngle)"
      file: "internal/imgproc/deskew.go"
      usata_in: ["internal/libreta/extract.go (Extract, DataPage)", "internal/libreta/mrzband.go (readMRZBand)"]
    - nome: "imgproc.Scale"
      file: "internal/imgproc/imgproc.go"
      usata_in: ["internal/libreta/mrzband.go (rowVariants)", "internal/libreta/viz.go (prepareVIZImage ripiego, vizCropVals)", "internal/libreta/viz_page.go (vizPreparePage)"]
    - nome: "imgproc.Binarize / ClearLines / TextRows / TrimRowEdgeBars / MRZPairs / CropRow / MainSpan"
      file: "internal/imgproc/rows.go"
      usata_in: ["internal/libreta/mrzband.go (readMRZBand, rowVariants, newCellGrid)", "internal/imgproc/pagecrop.go (DataPageGeometry)"]
    - nome: "imgproc.LowestPair"
      file: "internal/imgproc/rows.go"
      usata_in: ["internal/libreta/viz.go (vizLocateMRZ)"]
    - nome: "imgproc.StrokeWidth"
      file: "internal/imgproc/rows.go"
      usata_in: ["internal/libreta/mrzband.go (strokeRatio)"]
    - nome: "imgproc.GrayCrop / ThinToInk / PadWhite (ThinToInk usa Erode e InkFraction)"
      file: "internal/imgproc/rows.go"
      usata_in: ["internal/libreta/mrzband.go (thinVariants)"]
    - nome: "imgproc.CropDataPage / DataPageGeometry"
      file: "internal/imgproc/pagecrop.go"
      usata_in: ["internal/libreta/extract.go (DataPage)", "internal/imgproc/pagecrop_test.go"]
    - nome: "imgproc.IsBlank / AutoCropInfo"
      file: "internal/imgproc/autocrop.go"
      usata_in: ["cmd/libretto/main.go (scanAndPost)"]
    - nome: "ocr.Recognize"
      file: "internal/ocr/ocr.go"
      usata_in: ["internal/libreta/extract.go (pagina intera / foglio VIZ)"]
    - nome: "ocr.RecognizeMany"
      file: "internal/ocr/ocr.go"
      usata_in: ["internal/ocr/ocr.go (Recognize)", "internal/libreta/mrzband.go (ocrImages)", "internal/libreta/viz.go (vizOCRCrops)", "internal/libreta/viz_page.go"]
    - nome: "wia.Scan / wia.Describe / wia.ProbeArea / wia.IsPaperEmpty / wia.IsPaperJam"
      file: "internal/wia/wia.go, internal/wia/probe.go, internal/wia/errors.go"
      usata_in: ["cmd/libretto/main.go (cmdInfo, cmdAreaProbe, exitCode, scanAndPost)", "cmd/libretto/twaindrv.go (acquire)"]
    - nome: "wia.checkOutPath / newTempName / tempFiles"
      file: "internal/wia/outpath.go"
      usata_in: ["internal/wia/wia.go (Scan, finalizeImage)", "internal/wia/outpath_test.go"]
    - nome: "twain.ScanInfo / twain.Probe / twain.IsPaperEmpty / twain.IsPaperJam / twain.ArchitectureHint"
      file: "internal/twain/win.go, internal/twain/errors.go"
      usata_in: ["cmd/twainscan/main.go", "cmd/twainprobe/main.go"]
    - nome: "libreta.Extract / libreta.DataPage"
      file: "internal/libreta/extract.go"
      usata_in: ["cmd/libretto/main.go (extract, savePage)"]
    - nome: "libreta.firstMatch / regexpCache"
      file: "internal/libreta/extract.go, internal/libreta/regex.go"
      usata_in: ["internal/libreta/viz.go (vizExtractPage), unica chiamata"]
    - nome: "libreta.minf / maxf / absf"
      file: "internal/libreta/extract.go"
      usata_in: ["internal/libreta/viz.go (union, vizGroupLines, vizReadBand, vizValueLine, vizDedupDates, vizAssignNum9, vizValueBelow)", "internal/libreta/viz_test.go (absf)"]
    - nome: "libreta.autoScale"
      file: "internal/libreta/extract.go"
      usata_in: ["internal/libreta/viz.go (prepareVIZImage, ripiego)", "internal/libreta/viz_integration_test.go"]
    - nome: "libreta.dump"
      file: "internal/libreta/extract.go"
      usata_in: ["internal/libreta/extract.go (debug page.txt)", "internal/libreta/viz_integration_test.go"]
    - nome: "libreta.findOrientation / readMRZOriented / callCellsReader"
      file: "internal/libreta/extract.go"
      usata_in: ["internal/libreta/extract.go (Extract)", "internal/libreta/extract_hook_test.go"]
    - nome: "outsum.Parse / outsum.FieldKeys / outsum.CheckKeys"
      file: "internal/outsum/outsum.go"
      usata_in: ["cmd/libretto/summary.go (cmdSummary)", "cmd/regress/run.go, row.go, compare.go, truth.go"]
    - nome: "libreta.mrzClean"
      file: "internal/libreta/mrzband.go"
      usata_in: ["internal/libreta/mrzband.go (readMRZBand, ocrImages, bestReading, findMRZ)"]
    - nome: "libreta.vizSim / vizLev / norm / foldAccent"
      file: "internal/libreta/viz.go"
      usata_in: ["internal/libreta/viz.go (etichette, livello, nazionalita', regionale)", "internal/libreta/viz_nationality.go", "internal/libreta/viz_vote.go"]

  pattern_ricorrenti:
    - descrizione: "Pipeline: acquisizione (WIA o TWAIN) -> scarto vuote e ritaglio -> orientamento+MRZ -> campi stampati -> confronto MRZ/VIZ (l'MRZ prevale, le differenze diventano Warnings) -> traduzione in JSON inglese con avvisi a codice"
      dove: "cmd/libretto/main.go (scanAndPost, extract), internal/libreta/extract.go, cmd/libretto/output.go"
    - descrizione: "Confine di traduzione: il dominio (libreta.Result) usa chiavi interne italiane e avvisi in italiano; solo cmd/libretto/output.go le converte in nomi inglesi e codici stabili. Un test vieta parole italiane nel JSON"
      dove: "cmd/libretto/output.go, cmd/libretto/output_test.go"
    - descrizione: "Un file Go per responsabilita' nel pacchetto libreta (extract.go, mrzband.go, viz.go, viz_vote.go, viz_page.go, viz_jitter.go, viz_nationality.go); le funzioni dei campi stampati hanno prefisso viz*"
      dove: "internal/libreta"
    - descrizione: "Dati con check digit validati (ICAO pesi 7-3-1) e correzione accettata solo se univoca e coerente col controllo composito"
      dove: "internal/mrz/fix.go"
    - descrizione: "Letture multiple dello stesso dato (3 scale delle righe MRZ, varianti x foglio/ritaglio/jitter per i campi) e decisione per voto: a pezzi cella per cella per la MRZ, per carattere con pesi misurati sulle cifre note dall'MRZ per i campi"
      dove: "internal/libreta/mrzband.go (voteLine), internal/libreta/viz_vote.go (vizVoteAll, vizPosVote)"
    - descrizione: "Una sola chiamata OCR per molte immagini (RecognizeMany): il costo fisso e' l'avvio di PowerShell (circa 1 s)"
      dove: "internal/ocr/ocr.go, internal/libreta/mrzband.go, internal/libreta/viz.go, internal/libreta/viz_page.go"
    - descrizione: "Nessun OCR se l'immagine non ha una coppia di righe compatibile con la MRZ (risparmio di tempo sulle rotazioni sbagliate)"
      dove: "internal/libreta/mrzband.go (readMRZBand)"
    - descrizione: "Assottigliamento dei tratti per le stampe laser a 300 dpi (il motore OCR di Windows non legge i tratti troppo spessi). MRZ: varianti erose di 1-2 px con soglia scelta per frazione di inchiostro. Campi stampati: soglia adattiva dal rapporto spessore/altezza, 11 varianti binarizzate e jitter dei testi liberi solo come ritagli; nessuna variante in piu' se il tratto e' gia' sottile (facsimili)"
      dove: "internal/imgproc/rows.go (StrokeWidth, Erode, ThinToInk), internal/libreta/mrzband.go (thinVariants), internal/libreta/viz.go (vizAdaptiveThreshold, vizExtraVariants), internal/libreta/viz_jitter.go"
    - descrizione: "Dato di riferimento esterno per correggere l'OCR: il codice ISO dell'MRZ sceglie l'aggettivo di nazionalita' atteso da una tabella; il tipo mancante si ricava dal codice documento MRZ"
      dove: "internal/libreta/viz_nationality.go, internal/libreta/viz.go (vizFinalize)"
    - descrizione: "Controllo della destinazione prima di toccare l'hardware (checkOutPath) e file temporanei con nome casuale accanto all'uscita, rimossi sempre in uscita; nessuna sovrascrittura senza -force"
      dove: "internal/wia/outpath.go, internal/wia/wia.go, cmd/libretto/main.go (savePage, saveRaw, checkPageOut), cmd/twainscan/main.go"
    - descrizione: "Area di scansione verificata prima di avviare lo scanner: si imposta, si rilegge, e se il driver non la accetta identica si annulla (WIA: applyArea; TWAIN: ricette di ICAP_FRAMES e rilettura)"
      dove: "internal/wia/area.go, internal/twain/frames.go, internal/twain/session.go"
    - descrizione: "Trasporto astratto per testare il protocollo senza hardware: interfaccia transport (TWAIN) e areaProps (WIA) con driver finti nei test; il codice reale e' isolato dietro build tag windows"
      dove: "internal/twain/session.go, internal/twain/win.go, internal/wia/area.go, internal/wia/wia.go"
    - descrizione: "Programma ausiliario a 32 bit lanciato come sottoprocesso per un driver solo a 32 bit; lo stdout del programma principale resta riservato al JSON (testo ausiliario su stderr); codici di uscita 3 e 4 condivisi"
      dove: "cmd/libretto/twaindrv.go, cmd/twainscan/main.go"
    - descrizione: "Operazioni con effetti fisici o dati personali dietro conferma esplicita (confirmScan, -yes per gli script; SI digitato in twainscan) e avvisi su stderr quando si salvano dati personali (-debug, -page-out, -raw); codici di uscita per classe di errore (3 carta assente, 4 inceppamento, 130 Ctrl+C)"
      dove: "cmd/libretto/main.go, cmd/twainscan/main.go"
    - descrizione: "Dipendenze dal tempo iniettabili nei test (var now = time.Now)"
      dove: "internal/mrz/mrz.go, internal/mrz/fix_test.go"
    - descrizione: "Wrapper COM con funzione di callback (withManager/withDevice(match, fn)), thread bloccato con LockOSThread, rilascio con defer, timeout con goroutine"
      dove: "internal/wia/wia.go, internal/wia/options.go"
    - descrizione: "Errori propagati con %w e tradotti in italiano (explain + hresultMessages; twain.describeCC); il CLI stampa 'errore:' e esce con 1; uso errato esce con 2"
      dove: "internal/wia/errors.go, internal/twain/errors.go, cmd/libretto/main.go"
    - descrizione: "Funzioni pure testabili con immagini o risultati OCR sintetici (nessun dato personale); test di integrazione con build tag che riportano solo conteggi"
      dove: "internal/*/*_test.go, internal/libreta/*_integration_test.go"
    - descrizione: "Dipendenze esterne minime: solo go-ole (+ x/sys indiretta); OCR nativo con chiamate COM/WinRT dirette (ripiego: PowerShell incorporato) invece di librerie esterne; x/sys ora diretta; TWAIN con chiamate dirette alla DLL del DSM"
      dove: "go.mod, internal/ocr, internal/twain/win.go"
    - descrizione: "Diagnostica opzionale con -debug: salva immagini intermedie e testo OCR (possono contenere dati personali); sonde che non scansionano (info, area-probe, twainprobe)"
      dove: "internal/libreta/extract.go, internal/libreta/mrzband.go (ocrImages), cmd/twainprobe/main.go"
    - descrizione: "Passaggio di stato nascosto tra funzioni con variabili globali protette da mutex (da eliminare, vedi todo): ritagli del foglio, varianti della pagina, letture per il voto"
      dove: "internal/libreta/viz.go (vizStash, vizSetStash, vizTakeStash), internal/libreta/viz_page.go (vizPageStash), internal/libreta/viz_vote.go (vizRdOwner, vizSetReadings)"
    - descrizione: "Un rapporto datato per ogni agente in docs/agenti/ (cosa ha fatto, misure, non verificato, file da pulire) e un indice con lo stato del programma"
      dove: "docs/agenti/"
    - descrizione: "Quarantena invece di cancellazione: i resti di prova si spostano in _da_eliminare/ con un ELENCO.md che riporta il percorso originale; l'utente cancella da solo"
      dove: "_da_eliminare/ELENCO.md"
    - descrizione: "Script di prova con ciclo 'prossima prova' e finestra aperta, che rifiutano di sovrascrivere scans\\NOME.* e riassumono l'esito senza dati personali"
      dove: "tools/prova*.cmd, tools/summary.py"

  file_temporanei:
    - percorso: "_da_eliminare/"
      motivo: "QUARANTENA (366 MB, 2311 file): resti di prova degli agenti, spostati e NON cancellati; sara' l'utente a cancellarli. Contiene immagini e testo OCR con dati personali. Sottocartelle: out\\ (55 MB), viz\\ (115 MB), round3\\ (31 MB), round4\\ (95 MB: scansioni, diag_tests, areaprobe, exp5, json), round5\\ (67 MB: sim_*_NNN.png simulazioni dei primi N mm, libretto-vero1/2, geom, k7.json, zz-test.*), scans\\ (2,4 MB), radice\\_old, internal\\zz_time_test.go, drivers\\ (vuota), zz_dbg_test.go.txt. L'elenco _da_eliminare/ELENCO.md NON e' aggiornato per round4\\, round5\\ e zz_dbg_test.go.txt"
      tag: "[TEMPORANEO]"
    - percorso: "_out/ (circa 102 MB)"
      motivo: "scratch di sviluppo, in .gitignore. Contiene: crops2 (30 MB), viz_t (21 MB), dbg_a4/dbg_k7/dbg_qr (immagini e testo OCR con DATI PERSONALI), eseguibili di prova (mrz4, mrz5, viz4, viz5, viz6, wia7: 6,5 MB ciascuno), diag_tests_v/v2/v3 e exp6 (test e programmi di diagnostica, ignorati da go per il prefisso '_'), backup (mrzband.go.bak, viz.go.bak_pre_variants, viz_jitter.go.bak, viz_tilt.go.txt, viz_tilt.go.txt2), ev2.py, ev3.py, final5.txt, weights_all.txt, e twainprobe32.exe / twainprobe64.exe / twainscan32.exe (QUESTI SONO STRUMENTI IN USO: libretto cerca twainscan32.exe anche in _out/). L'agente VIZ lavora ancora qui (viz6.exe, viz_t, ev3.py, final5.txt)"
      tag: "[TEMPORANEO]"
    - percorso: "internal/libreta/viz_tmp_integration_test.go"
      motivo: "file di lavoro dell'agente VIZ (TestTmpGeom), build tag integration, con riferimenti non risolti (imageGrayAlias): rompe 'go vet -tags integration' finche' non e' completato o rimosso. Da eliminare a fine lavoro"
      tag: "[TEMPORANEO]"
    - percorso: "scans/vecchi test WIA/ (circa 380 MB, 113 file)"
      motivo: "spostate qui dall'utente: scansioni di prova WIA (stampe e libretti veri: 16 grezze *-grezza.png da 159 MB, 11 *-pagina.png da 41 MB, png, .json e .err): DATI PERSONALI, in .gitignore. I test di integrazione libreta le cercano qui (vizScanPath), quelli di imgproc e mrzband no"
      tag: "[TEMPORANEO]"
    - percorso: "scans/twain-probe.txt, twain-probe2.txt, twain-probe3.txt, twain-probe4.txt, twain-probe2.err, twain-probe4.err"
      motivo: "uscite di twainprobe32.exe (capability e prova di ICAP_FRAMES; 34-41 KB ciascuna; i .err sono vuoti). Nessun dato personale, ma contengono il dettaglio del driver: documentate in docs/agenti/03-wia.md"
      tag: "[TEMPORANEO]"
    - percorso: "libretto.exe, twainscan32.exe"
      motivo: "artefatti di build (6,6 MB e 3,4 MB), ricostruiti il 2026-10-22 19:24; rigenerabili (go build; GOARCH=386 per twainscan32.exe). _out/twainscan32.exe e' una copia piu' vecchia (19:22, 3,3 MB). Nessuno contiene dati personali"
      tag: "[TEMPORANEO]"
    - percorso: "drivers/_work/"
      motivo: "estrazione dell'installer PFU (608 MB): intermedi che l'utente ha chiesto di TENERE; non in proposta di pulizia"
      tag: "[TEMPORANEO]"
    - percorso: "drivers/_extract/"
      motivo: "estrazione intermedia dell'installer (281 MB, incl. _out.txt e _err.txt): intermedi che l'utente ha chiesto di TENERE; non in proposta di pulizia"
      tag: "[TEMPORANEO]"
    - percorso: "drivers/download/PSIPTWAIN-3_40_2.exe"
      motivo: "installer scaricato (246 MB, con il PDF readme da 220 KB), gia' estratto in PaperStream_IP_TWAIN_3.40.2_solo-driver; si puo' riscaricare"
      tag: "[TEMPORANEO]"
    - percorso: "drivers/PaperStream_IP_TWAIN_3.40.2_solo-driver/install-psip-twain.log"
      motivo: "log di msiexec dell'installazione (3,8 MB)"
      tag: "[TEMPORANEO]"

  file_eliminati_ultima_sessione: []   # nessuno: nessun file e' stato eliminato ne' spostato da questa passata (i resti di prova sono in quarantena in _da_eliminare/, da cancellare a cura dell'utente)

  gitignore_pattern_aggiunti:
    - "libretto.exe, *.exe, *.test, *.out, *.prof   # *.exe copre anche twainscan32.exe e _out/twainprobe32.exe (commento aggiunto)"
    - "_out/, _old/, _viz_exp/, _viz_exp2/, _viz_exp*/, debug/, dbg*/, debug_*, tmp_*, temp_*, *.tmp, *.bak, *.old, *.orig"
    - "*.wia.* (file temporanei di wia.Scan accanto al PNG di uscita, se il processo si interrompe)"
    - "/*.json (uscite JSON salvate con > risultato.json nella radice, contengono dati personali)"
    - "*.log"
    - "drivers/_work/, drivers/_extract/, drivers/download/, drivers/PaperStream_IP_TWAIN_3.40.2_solo-driver/PSTWAIN/"
    - "facsimili/   # dati personali di persone reali"
    - "scans/   # scansioni reali e relativi risultati (dati personali); copre scans/vecchi test WIA/, scan-*.png, twain-*.png, twain-probe*.txt e le uscite di tools/prova*.cmd (commento aggiornato)"
    - "_da_eliminare/   # quarantena, contiene immagini con dati personali"
    - "*.png, *.jpg, *.jpeg, *.bmp, *.tif, *.tiff (eccetto sotto docs/)"
    - ".vscode/, .idea/, Thumbs.db, desktop.ini"
    - "(round 5: nessun pattern nuovo; _out/*.go.txt, *.go.txt2 e *.py non sono coperti per nome ma stanno sotto _out/, ignorato)"

  todo_e_criticita:
    - "APERTO: internal/libreta/viz.go e' troppo grande (circa 2400 righe, 72 KB, in crescita mentre l'agente VIZ lavora): mescola geometria del foglio, riconoscimento dei formati, etichette, modo pagina di ripiego e confronto con l'MRZ (il voto e il ripiego sono gia' in viz_vote.go e viz_page.go). Proposta: dividere in viz_sheet.go (geometria e ritagli), viz_parse.go (date, numeri, etichette), viz_finalize.go (confronto). Piano in docs/agenti/08-piano-rifattorizzazione.md (non applicato)."
    - "APERTO: il progetto NON e' un repository git (git init non fatto) e non esiste altro snapshot: ogni modifica a viz.go e' irreversibile salvo le copie .bak in _out/ (viz.go.bak_pre_variants, mrzband.go.bak, viz_jitter.go.bak, viz_tilt.go.txt*). Decisione dell'utente (git init o copia fuori dall'albero)."
    - "APERTO: stato globale tra funzioni in libreta, ora in TRE punti: vizStash (ritagli del foglio, riconosciuti solo dalle dimensioni del foglio, uguali per ogni scansione), vizPageStash (varianti della pagina, riconosciute dalle dimensioni) e vizRdOwner/vizRd (letture per il voto, legate al puntatore del Result). Due Extract concorrenti possono scambiarsi i dati e uno stash non consumato resta in memoria. Serve cambiare le firme in extract.go (prepareVIZImage restituisce il set di ritagli, extractVIZ lo riceve)."
    - "APERTO: extractVIZ / vizOCRCrops (viz.go:117) e vizExtractPageSet (viz_page.go:86) usano context.Background() con timeout (60 e 90 s) e ignorano l'annullamento di Extract(ctx): Ctrl+C non li interrompe."
    - "FATTO (2026-10-22): chiavi JSON in inglese uguali alle etichette stampate, schema_version 1, avvisi con codice (cmd/libretto/output.go). Resta la mappa delle vecchie chiavi in docs/campi.md. APERTO minore: il testo 'data di X stampata non interpretabile come data' (viz.go vizFinalize) non ha un codice in toWarning e finisce in UNKNOWN_WARNING (il campo viene tolto dal JSON senza altra traccia)."
    - "FATTO (round 3): conferma prima di scansionare (confirmScan) e rifiuto di sovrascrivere (wia.checkOutPath, -force); temporanei con nome casuale. Resta: con read, un -page-out o -raw esistente senza -force e' scoperto DOPO la scansione (avviso, JSON comunque stampato); checkPageOut copre solo -page-out, prima dello scanner."
    - "APERTO: copertura test bassa dove c'e' hardware o OCR: cmd/libretto 14% (solo output.go; main.go senza test), wia 35% (Scan e Describe richiedono il dispositivo), ocr 0%, cmd/twainscan e cmd/twainprobe 0%; imgproc 78% (rows.go e imgproc.go solo via libreta); twain 74% con data source finto. Strategia in docs/agenti/11-revisione-test.md."
    - "APERTO: usage() di cmd/libretto/main.go e' incompleto: le righe di sintassi di scan/read non elencano -device, -driver, -from, -len, -width, -raw, -page-only (compaiono solo nell'elenco sotto). 'scan' accetta -page-only e -page-out ma li ignora in silenzio (scanFlags e' condiviso con read)."
    - "APERTO: libretto passa SEMPRE -ignora-sensore a twainscan32.exe (cmd/libretto/twaindrv.go): sul percorso TWAIN il controllo CAP_FEEDERLOADED e quindi l'errore 'carta assente' da sensore non esistono mai. Voluto per il percorso frontale (il sensore non vede il libretto), ma va detto e rivalutato quando TWAIN diventera' il driver predefinito."
    - "APERTO: libretto cerca twainscan32.exe prima accanto a libretto.exe, poi nella cartella corrente, poi in _out/: esistono due copie di build diverse (radice 19:24, _out 19:22). Pulire _out/ senza guardare puo' togliere l'unico helper quando la radice e' vuota."
    - "NON VERIFICATO: TWAIN con un documento. Il driver accetta ICAP_FRAMES con l'alto a 90 mm (twainprobe32.exe -try-frame, solo proprieta'), ma non e' provato che il MOTORE salti i primi mm, ne' la scansione (DIB nativo, evento XFERREADY, CAP_AUTOFEED). Prova: twainscan32.exe -from 90 -len 100 (o tools/prova-twain.cmd) con un libretto vero. Solo dopo cambiare defaultDriver in 'twain'. Il driver a 64 bit non e' installato."
    - "NON VERIFICATO: -width (WIA e TWAIN); l'effetto di Document Handling Select=1 sul percorso fisico; Transfer con libretti di spessore vicino a 5 mm; le soglie di IsBlank/AutoCrop (ora a texture) su altre scansioni reali. La scelta del percorso fisico non e' controllata dal codice: rischio di inserimento sbagliato (README, 'Sicurezza')."
    - "APERTO (incoerenza script): tools/prova-pagina.cmd stampa 'LIBRETTO VERO: apri sulla pagina con la FOTO, inserisci SOLO quella meta'' ma legge solo i primi 110 mm; con un libretto vero la pagina dati sta tra 90 e 180 mm (misurato), quindi 110 mm leggono quasi solo l'informativa. Per il libretto vero vale prova-libretto.cmd (190 mm). Lo script e' di altri: da correggere il testo o limitarlo ai fogli."
    - "APERTO (incoerenza test): imgproc/pagecrop_integration_test.go e libreta/mrzband_integration_test.go cercano le scansioni in scans\\ e non in scans\\vecchi test WIA\\ (dove le ha spostate l'utente): saltano o non trovano i file; viz_integration_test.go (vizScanPath) cerca anche in 'vecchi test WIA'."
    - "APERTO (incoerenza docs): docs/agenti/02-viz.md si ferma al round 3 e non descrive tabella delle nazionalita', jitter, viz_vote/viz_page; docs/agenti/03-wia.md cita ancora 'copertura 29%' (ora 35%) e _out\\twainscan32.exe come percorso del comando di prova (ora c'e' anche twainscan32.exe nella radice); docs/agenti/04-documentazione.md aggiornato in questa passata."
    - "APERTO (incoerenza quarantena): _da_eliminare/ELENCO.md (ultima modifica 17:54) non elenca round4\\ (95 MB) e round5\\ (67 MB) ne' zz_dbg_test.go.txt; la sezione 'Rimasto al suo posto' e' superata; cita internal\\libreta\\zz_test.go, che non esiste."
    - "APERTO: internal/libreta/viz_tmp_integration_test.go (agente VIZ, in lavorazione) non compila con -tags integration (imageGrayAlias); finche' c'e', 'go vet -tags integration ./...' e 'go test -tags integration' del pacchetto libreta falliscono."
    - "APERTO (ancora presente): libreta.Options.Scale (extract.go:36) non e' mai letto: il fattore di scala e' sempre autoScale. libreta.Result conserva tag json con nomi interni (rotation_deg, mrz, fields, warnings) non piu' usati dal CLI, che serializza cmd/libretto/output.go."
    - "APERTO (ancora presente): libreta.regexpCache (regex.go) usa una mappa globale non protetta da mutex ed e' usata una sola volta (extract.go firstMatch, chiamata da viz.go vizExtractPage): sostituibile con una regexp.MustCompile a livello di pacchetto. vizExtractPage costruisce un ocr.Result finto (res2/allText) solo per quella regex."
    - "Funzioni duplicate ancora presenti: mrz.itoa (strconv.Itoa); imgproc.mini/maxi/clampi (builtin min/max, gia' usati altrove); libreta.minf/maxf (builtin min/max); quattro 'abs' (imgproc.abs in rows.go, imgproc.acAbs in autocrop.go, libreta.absInt in viz.go, libreta.absf in extract.go); libreta.mrzClean ~ mrz.cleanOCR; libreta.vizMainSpan ~ imgproc.MainSpan; libreta.vizToGray ~ imgproc.gray; libreta.vizTextRows ~ imgproc.TextRows. NUOVE: due stimatori di inclinazione (imgproc.estimateSkew in autocrop.go, +-6 gradi, e imgproc.EstimateSkew in deskew.go, +-12 gradi); twain.runWithTimeout ~ wia.runWithTimeout; twain.decodeDIB ~ wia.decodeBMP; twain.frameForArea / Options.normalized ~ wia.areaPixels / checkAreaMM (da portare in un pacchetto comune)."
    - "Prestazioni: dopo il round 2 dei campi stampati i tempi sono quasi raddoppiati (da 6-8 s a 11-13 s per immagine) per le 11 varianti binarizzate in piu'; il jitter aggiunge circa 24 ritagli (a una sola chiamata OCR in piu'). Le chiamate OCR di avvio PowerShell (circa 1 s ciascuna) dominano il costo: vedi docs/agenti/09-revisione-prestazioni.md."
    - "Campi trovati non e' campi esatti: su prova1 14/14 trovati ma 13/14 esatti (capacity/cargo con la sigla 'II/2' non letta); capacity e place_of_birth (testo libero su filigrana) sono i piu' instabili. Sul facsimile 2020 (QR) restavano sbagliati luogo_nascita, numero_libretto e identificazione (misura del round 2)."
    - "Commento incoerente in mrzband.go: strokeRatio dichiara 'serve solo come informazione di debug', ma readMRZBand lo usa per decidere se mettere le varianti normali e assottigliate nella stessa chiamata OCR (soglia thickStrokeRatio = 0,185). In viz_jitter.go il commento di vizJitterJobs dice 'tre varianti di base' ma il codice ne usa due (2 basi x 4 spostamenti = 8 lavori, 24 ritagli sui 3 campi)."
    - "viz.go: il commento di vizCropSet dice 'firma fissa' di prepareVIZImage/extractVIZ come motivo dello stash: e' il motivo storico, da togliere quando lo stash sparisce."
    - "Il modo di ripiego sulla pagina intera (MRZ non trovata) e' debole: pochi campi e non affidabili (5-6 su 14 sulle pagine ritagliate). Da migliorare o da dichiarare solo diagnostico."
    - "Layout dei ritagli (vizBands) calibrato su facsimili 2020, 2026 A/B e scansioni reali; 2017 e 2019 mai misurati con MRZ leggibile. vizFinalize segnala PRINTED_FIELDS_NOT_READ anche per il modello 2017, dove id_marino, identificazione e regionale non esistono (incoerenza con docs/campi.md, regola 7); avviso spurio 'refrendo non letto' sul modello B."
    - "vizFinalize confronta regional con l'opzionale MRZ (similarita' < 0,7 = avviso): se nel 2020 l'opzionale e' 'PANAMA' e il Regional stampato e' il nome dell'ufficio, l'avviso e' spurio. Da verificare su scansioni vere."
    - "'country_code' e 'type' stampati non sono confrontati con mrz.issuer e mrz.doc_code; 'surname' e 'given_names' vengono solo dall'MRZ (la VIZ non li legge, anche se le etichette sono in vizLabels). Il tipo mancante e' ricavato dall'MRZ con avviso."
    - "Extract usa CropContent (soglia fissa 225) + Deskew e non AutoCrop: su 'read' il ritaglio del documento e' fatto due volte (AutoCropInfo in scanAndPost, poi CropContent in Extract); DataPage ricarica l'immagine e ripete CropContent, Deskew e rotazione (secondo caricamento)."
    - "APERTO: calibrazioni su pochissime scansioni reali: td3BottomMM = 7,5 mm del ritaglio -page-out (una stampa + 4 facsimili), soglie thickStrokeRatio e thinInkTargets, soglie del jitter. Possono non generalizzare."
    - "DA VERIFICARE (incoerenza codice/messaggi): main.go fa signal.NotifyContext(os.Interrupt) ma passa ctx solo a extract e read -> libreta.Extract; wia.Scan, scanTwain e confirmScan non lo ricevono. Con NotifyContext il Ctrl+C non termina piu' il processo da solo: durante la scansione o l'attesa della conferma (il messaggio dice 'Ctrl+C per annullare') potrebbe non succedere nulla. Verificare su Windows e valutare di far leggere ctx a confirmScan e alle scansioni."
    - "Codice non raggiunto dal CLI: wia.List, wia.Dump (diagnostica), imgproc.AutoCrop (usata solo dai test; il CLI usa AutoCropInfo). imgproc.LowestPair ora e' USATA (viz.go vizLocateMRZ). Esporre List/Dump con un comando o eliminarle."
    - "Timeout della scansione WIA (Options.Timeout, 2 min) e di twain.Options.Timeout non esposti da riga di comando."
    - "tools/summary.py: l'elenco dei 14 campi (KEYS) e' scritto a mano e va tenuto allineato alle chiavi inglesi di docs/campi.md. DUPLICATO dal 2026-10-23: lo stesso riassunto lo fa 'libretto summary' (cmd/libretto/summary.go, elenco in internal/outsum) e, su tutto il corpus, cmd/regress. Proposta: spostarlo in _da_eliminare\\ (serve l'ok dell'utente)."
    - "FATTO (2026-10-23): la normalizzazione provvisoria di cmd/regress/truth.go e' stata tolta; l'harness usa verify.LoadTruth e verify.Compare di internal/verify (agente verificatore). Dipendenza da tenere d'occhio: se il verificatore cambia quelle firme o i nomi verify.Status*, cmd/regress non compila."
    - "PARZIALMENTE RISOLTO (2026-10-23 12:13): la riga 1 dell'MRZ (cognome e nomi) non ha check digit e una riga 1 sbagliata con prefisso ISPAN e 44 caratteri li passa tutti. Ora il lettore a template gira sempre e fa da seconda lettura (internal/libreta/mrzmerge.go, reconcileMRZ): dove differisce dall'OCR e non segnala incerte le celle diverse si prende la sua lettura; i nomi dell'MRZ giusti rispetto alla verita' passano da 39/40 a 40/40. Resta aperto il confronto con i nomi STAMPATI: il VIZ trova le etichette Apellidos/Surname e Nombres/Given Names (vizLabels) ma non ne legge il valore; richiesta al VIZ in docs/agenti/COORDINAMENTO.md (sezione 5)."
    - "Il lettore MRZ a template (internal/libreta/mrzcells.go, mrzcells_glyph.go, mrzcells_decode.go, glyphs.bin; agente concluso alle 11:46) si registra con init() in mrzCellsReader ed e' in libretto.exe (build del 2026-10-23 12:05, sha256 B13F115F66A1). La libreria dei glifi (8 libretti) contiene anche le scansioni di prova: su un libretto nuovo con lettere mai viste conviene guardare gli avvisi MRZ_*_UNCERTAIN e mrz_corrections."
    - "drivers/LEGGIMI.md parla di PSIPTWAIN-3_40_2.iso 'DA SCARICARE', ma in drivers/download c'e' l'.exe (246 MB) e il driver e' gia' estratto; Installa-Driver.ps1 gestisce entrambi, Installa-Solo-Driver.cmd e' il percorso usato. C'e' anche drivers/installazione_manuale/ (non citata in drivers/LEGGIMI.md); il suo LEGGIMI dice che il driver WIA da solo dovrebbe bastare ma non e' verificato su un PC senza PaperStream IP."
    - "facsimili/panama/panama.png (2,7 MB) e' una scansione reale di una persona e non e' elencata nel LEGGIMI dei facsimili."
    - "go.mod dichiara go 1.22 mentre il toolchain locale e' 1.27: verificare con 1.22 prima di dichiarare il requisito."
    - "Barcode QR/PDF417 non decodificati; foto e firma non estratte."
```

## Pulizia proposta

Analisi `/cleanup` del 2026-10-22 (sera), dopo i round 4 e 5 e l'avvio del lavoro TWAIN. **Non e' stato eliminato ne' spostato nulla**: i resti di prova sono gia' in `_da_eliminare\`
(366 MB, 2311 file; elenco in `_da_eliminare\ELENCO.md`, **incompleto**) e li cancellera' l'utente da solo. `go vet ./...` non segnala import inutilizzati (il compilatore Go li rifiuta);
`staticcheck` e `deadcode` non sono installati: il dead code e' cercato con ricerca testuale (`grep`) sui riferimenti. Sui sorgenti `.go` del modulo non risultano `test_*`, `*_copy.*`,
`__pycache__` ne' blocchi di codice commentato fuori uso, a parte il file temporaneo dell'agente VIZ qui sotto. I numeri di riga di `viz.go` sono omessi perche' il file cambia mentre l'agente VIZ lavora.
Dimensioni misurate con `du` (1 MB = 1.048.576 byte). **Non toccare finche' l'agente VIZ lavora:** `_out\viz6.exe`, `_out\viz_t`, `_out\ev3.py`, `_out\final5.txt`, `internal\libreta\viz_tmp_integration_test.go`.

```
[TEMP]    _da_eliminare\round4\                                95 MB   quarantena: scansioni, anteprime, diag_tests, areaprobe, exp5, mrz3.exe e json del round 4  [DATI PERSONALI]
[TEMP]    _da_eliminare\round5\                                67 MB   quarantena: sim_*.png (simulazioni dei primi N mm), libretto-vero1/2, geom, k7.json, zz-test.*  [DATI PERSONALI]
[TEMP]    _da_eliminare\viz\                                  115 MB   quarantena: programmi di esperimento VIZ, fogli (dbg_viz 34 MB), p1_*.png, viz*.exe, script e*.py  [DATI PERSONALI]
[TEMP]    _da_eliminare\out\                                   55 MB   quarantena: debug MRZ, esperimenti, PNG sciolti, mrz.exe, main.exe, ocrmany.exe, wia.exe  [DATI PERSONALI]
[TEMP]    _da_eliminare\round3\                                31 MB   quarantena: crops\, pagina_prova1.png, prova1_estratto.json, mrz2.exe, wia2.exe  [DATI PERSONALI]
[TEMP]    _da_eliminare\scans\                                2,4 MB   quarantena: cartelle di debug dbg1, dbg3, dbg4  [DATI PERSONALI]
[TEMP]    _da_eliminare\zz_dbg_test.go.txt                      4 KB   quarantena: test di diagnostica temporaneo (non e' in ELENCO.md)
[TEMP]    _da_eliminare\radice\_old, internal\zz_time_test.go, drivers\   <1 MB   quarantena: codice accantonato, test di tempi, cartella vuota
[TEMP]    _da_eliminare\ELENCO.md                              12 KB   l'elenco di ripristino: tenerlo (e aggiornarlo) finche' resta qualcosa in quarantena
[TEMP]    _out\crops2\                                         30 MB   ritagli di campi di una scansione reale  [DATI PERSONALI]
[TEMP]    _out\viz_t\                                          21 MB   fogli VIZ scritti dai test di integrazione  [DATI PERSONALI]  (si ricrea)
[TEMP]    _out\dbg_a4\, dbg_k7\, dbg_qr\                      3 MB   cartelle -debug: pagina, righe MRZ e testo OCR  [DATI PERSONALI]
[TEMP]    _out\mrz4.exe, mrz5.exe, viz4.exe, viz5.exe, wia7.exe   32 MB   eseguibili di prova superati (6,4-6,5 MB ciascuno); viz6.exe (6,5 MB) e' dell'agente VIZ in corso
[TEMP]    _out\diag_tests_v\, diag_tests_v2\, diag_tests_v3\, exp6\   <1 MB   test e programmi di diagnostica (ignorati da go per il prefisso '_')
[TEMP]    _out\ev2.py, ev3.py, final5.txt, weights_all.txt      <5 KB   script e risultati di valutazione (final5.txt riporta conteggi di test di integrazione; non ispezionato per dati personali)
[TEMP]    _out\twainprobe64.exe                               2,9 MB   sonda a 64 bit: serve solo a dire che il driver a 64 bit non c'e'
[TEMP]    _out\twainscan32.exe                                3,3 MB   copia piu' vecchia di ..\twainscan32.exe (radice, 3,4 MB): tenere una sola copia; occhio: libretto cerca l'helper anche in _out
[BACKUP]  _out\viz.go.bak_pre_variants                         69 KB   copia di viz.go prima delle varianti (utile finche' non c'e' git)
[BACKUP]  _out\mrzband.go.bak                                  23 KB   copia di mrzband.go
[BACKUP]  _out\viz_jitter.go.bak                                2 KB   copia di viz_jitter.go
[BACKUP]  _out\viz_tilt.go.txt, viz_tilt.go.txt2                4 KB   due copie identiche (3,6 KB) di un'idea di stima dell'inclinazione, mai integrata
[TEMP]    internal\libreta\viz_tmp_integration_test.go          1 KB   file di lavoro dell'agente VIZ (non compila con -tags integration): da eliminare a fine lavoro
[TEMP]    scans\vecchi test WIA\*-grezza.png                  155 MB   16 scansioni grezze (circa 10-12 MB ciascuna), ridondanti con i png ritagliati  [DATI PERSONALI]
[TEMP]    scans\vecchi test WIA\ (resto)                      225 MB   scansioni ritagliate, pagine dati, json ed err di prova: l'utente le ha spostate qui apposta; i test le usano  [DATI PERSONALI]
[TEMP]    scans\twain-probe.txt .. twain-probe4.txt           ~150 KB   uscite di twainprobe32.exe, gia' riassunte in docs\agenti\03-wia.md (twain-probe2.err e twain-probe4.err sono vuoti)
[TEMP]    drivers\download\PSIPTWAIN-3_40_2.exe               246 MB   installer originale (con il PDF readme da 220 KB), gia' estratto; riscaricabile
[TEMP]    drivers\PaperStream_..._solo-driver\install-psip-twain.log   3,8 MB   log di installazione
[BACKUP]  drivers\Installa-Driver.cmd                         0,4 KB   superato da Installa-Solo-Driver.cmd? (dipende dall'ISO, che non c'e')
[BACKUP]  drivers\Installa-Driver.ps1                         3,0 KB   idem: e' l'unico script con la verifica SHA256/firma, valutare se tenerlo
[DEAD]    internal\libreta\extract.go:36                              campo Options.Scale mai letto
[DEAD]    internal\wia\wia.go:69 e :112                               List e Dump: nessun chiamante nel CLI (API diagnostiche); non eliminare senza decidere se esporle
[DEAD]    internal\imgproc\autocrop.go:59                             AutoCrop usata solo dai test (il CLI usa AutoCropInfo)
[DEAD]    internal\mrz\fix.go:118                                     itoa riscrive strconv.Itoa: sostituibile
[DEAD]    internal\imgproc\imgproc.go:148-162                         mini, maxi, clampi duplicano i builtin min/max (Go 1.21+); usati ancora da autocrop.go e imgproc.go
[DEAD]    internal\libreta\extract.go:167-184                         minf, maxf, absf: i builtin min/max sostituiscono minf e maxf (absf no)
[DEAD]    internal\imgproc\rows.go:453, autocrop.go:431, extract.go:181, viz.go (absInt)   quattro funzioni "abs" equivalenti
[DEAD]    internal\libreta\regex.go                                   regexpCache: una sola chiamata (firstMatch), mappa senza mutex
[DEAD]    internal\libreta\mrzband.go:644                             mrzClean duplica mrz.cleanOCR (fix.go, stessi caratteri riempitivi)
[DEAD]    internal\libreta\viz.go (vizMainSpan)                        duplica imgproc.MainSpan
[DEAD]    internal\libreta\viz.go (finalizeVIZ)                       alias di vizFinalize (extract.go lo chiama, 18 test)
[DEAD]    internal\twain\win.go (Scan)                                twain.Scan non e' chiamata da nessuno (i programmi usano ScanInfo); solo wrapper
[DEAD]    internal\imgproc\autocrop.go (estimateSkew)                 duplica in parte imgproc.EstimateSkew (deskew.go): due stimatori con intervalli diversi
[IMPORT]  nessuno                                                     tutti gli import risultano usati (go vet pulito)
```

Non in proposta (l'utente li vuole tenere): `drivers\_work\` (608 MB) e `drivers\_extract\` (281 MB), intermedi dell'installer; `drivers\installazione_manuale\`
(780 KB, INF/CAT/CAB per l'installazione a mano); `drivers\PaperStream_..._solo-driver\PSTWAIN\` (178 MB), che serve a `Installa-Solo-Driver.cmd`;
`libretto.exe` (6,6 MB) e `twainscan32.exe` (3,4 MB), ricostruiti dal codice attuale; `_out\twainprobe32.exe` (2,8 MB, strumento documentato); `tools\` (script e `summary.py`);
`docs\agenti\` e `.claude\agents\`; `scans\vecchi test WIA\` (salvo la scelta dell'utente sulle grezze).

### Privacy: immagini con dati personali di persone reali

Da non copiare, non inviare e non versionare; il `.gitignore` le esclude, ma restano su disco (e nelle copie di backup o cloud, se la cartella
e' sincronizzata).

- `scans\vecchi test WIA\` (circa 380 MB, 113 file): **scansioni reali** di prova (stampe e libretti veri, grezze e ritagliate, `*-pagina.png`) con i `.json` ed `.err` dei risultati.
- `facsimili\panama\panama.png` (2,7 MB): **scansione reale** di una persona; non e' elencata nel `LEGGIMI.md` dei facsimili.
- `facsimili\panama\libreta_2020_pagina-dati_QR.jpg`, `libreta_2026_anexo8_pagina-dati_A.jpg`, `libreta_2026_anexo8_pagina-dati_B.jpg`: pagine dati
  con nome, foto, data di nascita e numeri **non oscurati** (AMP le pubblica come "sample", ma potrebbero essere persone reali).
  Gli stessi dati stanno nei PDF `facsimili\panama\fonti\MMC-331_AMP_2020-12.pdf` e `Anexo-8-Libreta-de-Embarque_AMP_2026-03.pdf`.
  `libreta_2017_pagina-dati_PDF417_oscurata.jpg` e `libreta_2019_appendice6_pagina-dati.jpg` sono oscurate (anche parzialmente).
- Tutto `_da_eliminare\` (366 MB) e le cartelle `_out\crops2`, `_out\viz_t`, `_out\dbg_*` sono **derivati** da quelle immagini: copie della pagina intera
  (`page.png`), righe e fogli di campi, rotazioni, simulazioni dei primi N mm, e file `page.txt` / `mrz_ocr.txt` / `*.json` (testo OCR e valori con nomi e numeri). Vanno cancellati appena possibile.
- Gli eseguibili, `scans\twain-probe*.txt` e `docs\agenti\` non contengono dati personali.
- Le uscite del programma contengono dati personali: `scans\scan-*.png` (default di `scan`/`read`), i file di `-page-out`, `-page-only` e `-raw`, le cartelle di `-debug` e i JSON salvati con `>`; il programma avvisa su stderr per `-debug`, `-page-out` e `-raw`.

Totale ancora proposto fuori dalle quarantene: `drivers\download\PSIPTWAIN-3_40_2.exe` (246 MB), il log di installazione (3,8 MB), circa 100 MB in `_out\` (di cui circa 54 MB con dati personali) e, a scelta dell'utente, 155 MB di scansioni grezze in
`scans\vecchi test WIA\`. In quarantena: `_da_eliminare\` 366 MB, da cancellare a cura dell'utente.

Vuoi che elimini tutti i file elencati, solo alcuni, o preferisci gestirli manualmente?
