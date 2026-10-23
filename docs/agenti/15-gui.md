# 15 - Agente GUI (Panama Book)

Ultimo aggiornamento: **2026-10-23**. Nessun dato personale in questa pagina: solo esiti. Manuale d'uso: [../panamabook.md](../panamabook.md).

## File dell'agente
`cmd/panamabook/**` (`main.go`, `server.go`, `jobs.go`, `doc.go`, `save.go`, `web/` incorporata con `embed`, `fakelibretto/` finto
`libretto.exe` per test e screenshot), `internal/memoria/**`, `docs/panamabook.md`, questo file. Dipende in sola lettura da
`internal/mrz` (tipo `mrz.Data`), `internal/outsum` (`FieldKeys`) e, nei test, da `internal/verify` (`ParseTruth`).
Una riga in `.gitignore`: `dati/` (archivio della memoria e profilo di Edge: dati personali).

## Cosa c'e'
- **Server locale**: solo `net/http`, `127.0.0.1` su porta casuale, token di sessione su tutte le `/api/*` (header `X-PB-Token` o query `t`),
  controlli `Host`, `Origin`, `Sec-Fetch-Site`, CSP stretta, immagini solo da `scans\`. Finestra: `msedge.exe --app=URL --user-data-dir=dati\edge`
  (ripiego `rundll32 url.dll,FileProtocolHandler`). Chiusura: heartbeat ogni 5 s (spenta dopo 25 s o 4 s dopo `bye`), Ctrl+C; non esce mai con un lavoro in corso
  e durante la scansione fisica aspetta la fine.
- **Lavori asincroni** con id e polling (700 ms): scansione + lettura (`libretto.exe scan ...` poi `extract -i`) e sola lettura. Uno alla volta.
  Annulla solo nella fase di lettura (termina il sottoprocesso); durante `scan` l'API risponde 409. Codici di uscita dello scanner tradotti
  (3 carta assente, 4 inceppamento).
- **Memoria** (`internal/memoria`): chiave = numero libretto MRZ + data di nascita MRZ, creabile solo con i 5 check digit validi
  (`memoria.Chiave`); `dati\memoria.json` con scrittura atomica (`memoria.ScriviFile`), permessi 0600; per campo: corretto, letto, data.
  Applicata dopo l'estrazione ai campi che differiscono; "dimentica" per campo e per libretto.
- **Salvataggio**: "Salva correzioni" = memoria; "Conferma e salva" = memoria + `scans\NOME.json` (con blocco `corrections`) + verita'
  `scans\verita\NOME.json` (14 chiavi di `outsum.FieldKeys`, `_riga_nome`, `_note`, `_origine`), con conferma e copia `.prec-DATA` se esistono.
- **PDF** (richiesta del coordinatore): `libretto pdf` come sottoprocesso, ripiego su `_out\libretto_pdf.exe`; mostra il peso; non sovrascrive senza conferma.
- Alias in lettura `endorsement` -> `certificate`; si salva sempre `certificate`.
- Interfaccia in italiano, tema chiaro con dark mode automatico, tastiera, `aria-live`, responsive. Si ricarica la finestra senza perdere l'ultimo documento.

## Aggiunte del secondo giro (2026-10-23)
- **Campi controllati**: stato `reviewed` (Controllato / Controllato da memoria), pulsante, Invio e Alt+C, pillola in testata con contatore, avviso `campi_aperti`
  (Torna a controllare / Conferma comunque) con verità vuota per i campi non controllati, blocco `review` nel JSON salvato. Memoria: `Conferma` per campo
  (`SalvaConferme`, `RimuoviConferme`, `Confermati`; formato compatibile: campo `confermati` opzionale), applicata dopo l'estrazione se il valore è lo stesso.
- **Ricomincia da capo** (Alt+N, `POST /api/reset`, `GET /api/suggest`: nome libero `libretto-NNN`), a tre scelte con modifiche non salvate.
- **Nome dei file** `NOME - COGNOME - NUMERO LIBRETTO` **solo con "Conferma e salva"** (correzione dell'utente: prima della conferma il nome potrebbe essere sbagliato).
  `naming.go`: `futureName` (da `internal/filename`: `Base`, `Unique`, dai valori del form), `renameAll` tutto-o-niente di .png/.jpg/.json/.pdf/.err, `POST /api/preview-name` per
  la riga "Dopo la conferma i file si chiameranno: ...". Dopo la lettura e con "Salva correzioni" nessuna rinomina; l'MRZ valida non è richiesta; mancano nome, cognome o numero
  = restano col nome scelto (e lo dice); riconfermando non si aggiunge "(2)"; la verità già scritta segue la rinomina. `fields.given_names`/`surname` non si invertono.
  Non uso `libretto rename` come sottoprocesso: il pacchetto basta e non dipende dalla build installata.
- Test: `review_test.go`, `naming_test.go` (nomi sintetici con accenti e simboli, `(2)`, rollback) e prova dell'interfaccia con Edge headless pilotato da DevTools
  (39 controlli: pulsante, contatore, Invio, Alt+C, avviso, memoria, ricomincia, scelte, tema scuro).

## Flusso "Anteprima" (richiesta dell'utente, 2026-10-23)
- **Anteprima**: `POST /api/preview` scansiona (o copia la demo) in `dati\anteprima\anteprima-NNN.png`; niente in `scans\`, niente JSON, niente lettura (`preview.go`).
  **Leggi i dati**: `POST /api/open {file, dir:"preview"}` (`extract -i dati\anteprima\...`). **Riscansiona** = nuova anteprima. **Scarta**: `POST /api/preview/discard`, solo
  `anteprima-NNN.png/.pdf` in `dati\anteprima\`. Striscia delle ultime 5 (`GET /api/previews`), pulizia > 7 giorni all'avvio, suggerimento "immagine molto bassa" (altezza/larghezza < 0,5).
- **Conferma e salva** su un'anteprima: `movePreview` sposta immagine e PDF in `scans\` col nome finale (`futureName` o nome di riserva, Unique), poi JSON e verità; prima nulla in `scans\`.
  PDF prima del salvataggio in `dati\anteprima\`. "Scansiona" che salvava subito è rimosso dall'interfaccia (l'API `/api/scan` resta, usata dai test).
- Ricomincia da capo e `beforeunload` avvisano se ci sono anteprime non salvate. Lock: l'ordine è sempre `docsMu` poi `d.mu` (il documento non tiene più una mappa file->documento: la striscia lo cerca).
- Test: `preview_test.go` (nessun file in scans dopo Anteprima e Leggi, file solo dopo Conferma, scarto, striscia, pulizia, (2), nome di riserva, immagine bassa, demo) e prova DevTools (75 controlli).

## Grezza, diagnosi e pagina dati (segnalazione "taglia male", 2026-10-23)
- **Lunghezza predefinita 190 mm** (copre la pagina dati sia con il bordo inserito per primo sia con quello in mano), TWAIN o WIA, impostazioni salvate nel browser con chiave nuova (`pbset2`).
- **Grezza**: `scan ... -raw dati\anteprima\anteprima-NNN-grezza.png` (flag esistente, nome sempre nuovo); si scarta/pulisce con l'anteprima; "Ritagliata/Grezza" diventa un interruttore **Pagina dati / Intera / Grezza**.
- **Diagnosi** (`diagnose.go`, `internal/imgproc.AutoCropInfo` in sola lettura, senza OCR): decentrato destra/sinistra (direzioni dell'anteprima, la grezza e' capovolta), troppo poco, inclinato, nessun documento;
  scritta in `anteprima-NNN-diagnosi.json` e mostrata sotto l'immagine. Non bloccante. Il suggerimento "immagine molto bassa" resta.
- **Pagina dati**: `Leggi i dati` su un'anteprima aggiunge `-page-out anteprima-NNN-pagina.png -top-margin M -force`; e' l'immagine di `Conferma e salva` e del PDF (`movePreview`: scans\NOME.png = pagina dati;
  `NOME (intera).png` e `NOME (grezza).png` solo con le caselle; senza pagina si salva l'anteprima e lo si dice). Il PDF fatto prima della lettura (dall'intera) si scarta alla lettura.
- Test: `diagnose` pura (`TestAnalyzeBoxCasi`) e con grezze sintetiche vere (`TestDiagnosiSuGrezzeSintetiche`: centrata, destra, sinistra, bassa, inclinata, vuota, via il finto libretto con `FAKE_RAW`), grezza/pagina/diagnosi nello scarto e nella pulizia, salvataggio
  della sola pagina (200x140 del finto), caselle, assenza di pagina; prova DevTools in modo `diag` (scansione "fisica" del finto: 19 controlli) e in demo (77).

## Ricetta WIA e lettura automatica (2026-10-23)
- Driver predefinito WIA (`defDriver`), `-from 0 -rotate 180` solo con TWAIN (`scanArgsTo`), diagnosi senza specchio per WIA (`diagnoseRaw(..., flip)`), impostazioni browser `pbset3`.
- A fine scansione parte `extractCached` (page-out + JSON in `-lettura.json`); `Leggi i dati` usa la cache (nessun sottoprocesso); vista predefinita Pagina dati. Test: `TestLetturaAutomaticaPaginaDatiESalvataggio`, `TestPaginaNonTrovataSiMostraLIntera`, `TestScansioneWIAnonRuota`; DevTools 77/22/17.

## Robustezza e registri (segnalazione "ha crashato", 2026-10-23)
- **Causa piu' probabile del caso dell'utente: l'ho causata io.** Nei miei script di prova (`scenario.sh`, comandi PowerShell) usavo
  `Get-Process PanamaBook,msedge | Stop-Process -Force`: terminava per NOME anche l'istanza reale dell'utente e il suo `libretto.exe`,
  senza lasciare tracce (un TerminateProcess non si intercetta; `panamabook-crash.log` vuoto, nessuna riga di uscita). Corretto: nessun
  script termina piu' processi per nome; le prove avviano una COPIA del binario (`PanamaBookTest.exe` nello scratchpad, mai quello della
  radice) e terminano solo i pid avviati da loro; `PanamaBook.exe` della radice si ricostruisce solo se non gira.
- **Registri**: `dati\panamabook.log` (eventi, motivo dell'uscita, segnali SIGTERM/Interrupt, "vivo" al minuto, errori della pagina),
  `dati\panamabook-crash.log` (`debug.SetCrashOutput`, file `crash_go123.go` con build tag), fallback `panamabook-eventi.log` se il nome e'
  occupato da una redirezione di `cmd` (verificato: cmd tiene il file senza condivisione in scrittura); `tools\panamabook.cmd` ora
  redirige la console in `dati\panamabook-console.log` e mostra gli altri due.
- **Panic**: `recoverTo` in tutte le goroutine (lavori -> errore visibile, server, attesa di Edge, controllo di uscita) e nel gestore HTTP (500 con messaggio, stack nel registro).
- **Uscita prudente**: mai con un lavoro in corso, mai con un heartbeat negli ultimi 60 s; con Edge nostro la chiusura e' la fine del suo processo (>= 10 s di vita),
  rete di sicurezza 3 minuti senza heartbeat; `bye` accelera solo senza Edge nostro e dopo 15 s senza heartbeat nuovi. Nessuna navigazione fuori dalla pagina (link, file trascinati);
  heartbeat immediato al ritorno in primo piano; gli errori JavaScript vanno nel registro e in un avviso.
- **Lock** rivisti a mano su App, lavori, documenti, memoria: corretto `hJob` (leggeva fase e nome del documento dopo aver rilasciato il lucchetto); stderr del sottoprocesso sempre svuotato.
  Senza cgo non c'e' `-race`: test di stress (`TestStatoCondivisoConcorrente`, 10 goroutine con rinomina in corso, ripetuto con `-count=8`) e prova DevTools sul documento vuoto (1513x590, nessuna MRZ, 17 campi vuoti; Invio e Alt+C su tutti i campi).

## Verifiche (2026-10-23)
- `gofmt`, `go vet ./...` puliti per i miei file; `go test ./cmd/panamabook ./internal/memoria` verdi (token/Host/Origin, scansione con finto scanner e
  argomenti, errori scanner, demo senza scanner, niente annulla durante la scansione, annulla la lettura, percorsi malevoli, memoria che impara/applica/dimentica,
  memoria non usata con MRZ non valida, verita' leggibile da `verify.ParseTruth`, sovrascritture con consenso, PDF e ripiego, heartbeat/uscita).
- Estrazione reale provata solo su una COPIA di un'immagine di `scans\` in una cartella temporanea: lettura riuscita in 11 s, MRZ 5/5, 17 campi mostrati, un campo "Da controllare".
- Screenshot con Edge headless su dati sintetici: tema chiaro, scuro, lavoro in corso, MRZ non valida, memoria applicata.

## Limiti e prossimi passi
- La memoria e' **per libretto**: un errore di lettura ricorrente tra libretti diversi non viene imparato (idea: alias errore -> valore per campo e per etichetta).
- Nessun ritaglio manuale dell'immagine, nessuna firma del codice ne' installer (SmartScreen puo' avvisare al primo avvio).
- La finestra nera di console resta aperta (build normale); con `-ldflags -H=windowsgui` sparirebbe ma si perderebbero i messaggi.
- Il margine sopra il titolo e' passato a `libretto.exe` ma incide solo sul salvataggio della pagina ritagliata.
- `libretto.exe` installato non ha ancora il comando `pdf` (ripiego su `_out\libretto_pdf.exe`).
- Prova con lo scanner vero **non fatta** (per regola): la prima scansione reale va guardata dall'utente.
