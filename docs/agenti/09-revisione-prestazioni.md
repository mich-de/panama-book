# 09 · Revisione delle prestazioni (`architect-scalability-reviewer`)

**Lente:** prestazioni, affidabilita' operativa, osservabilita'. Sola lettura; nessuno scanner toccato.

## Misurato (benchmark su immagine 2550x4200 RGBA, copia di `imgproc.go` nello scratchpad)
- `Rotate` 150-160 ms e 107 MB allocati (10,7 milioni di allocazioni); `CropContent` 325 ms e 150 MB; codifica PNG circa 0,4 s.
- L'elaborazione di immagini pesa circa 1 s in totale: **non e' il collo di bottiglia, lo e' PowerShell.**
- Dal codice (stima, non misurato su `read`/`extract`): chiamate a PowerShell per documento = 1-2 per ogni rotazione che trova una coppia di righe (`mrzband.go`) + 1 per la pagina (`extract.go:86`) + 1 per i ritagli (`viz.go:103`).
  Caso ideale 3 (circa 3 s fissi); peggiore oltre 10.
Misure complete del committente: estrazione 5,8-8,4 s per immagine; scansione 11 s.

## Alta
1. **I ritagli partono in una seconda chiamata sequenziale** (`viz.go:1090-1091`) pur essendo gia' in memoria: passarli a `RecognizeMany` insieme al foglio. Risparmio stimato circa 1 s per documento.
2. **`context.Background()`** in `viz.go:101` ignora il `ctx` del chiamante; `main.go:132` passa `Background` a `Extract` senza gestire Ctrl+C. *Correzione:* propagare il `ctx` e usare `signal.NotifyContext`.
3. **Stash globale dei ritagli** (`viz.go` 59-80): non sicuro con elaborazioni concorrenti. Restituire i ritagli come valore.
4. **Uso in serie (batch):** `scan.png` e' il nome predefinito (`main.go:77`), quindi ogni libretto sovrascrive il precedente; il file temporaneo WIA ha nome fisso (`wia.go:211`) e due scansioni in parallelo collidono. *Correzione:* nome con indice o timestamp.

## Media
5. `ocr.go:57-67`: a ogni chiamata si riscrive lo script e si paga l'avvio di PowerShell piu' `Add-Type`. Un processo persistente (righe JSON su stdin) farebbe risparmiare circa 1 s per chiamata, ma ha senso solo per un comando `batch`; per un libretto singolo e' meglio il punto 1.
6. Immagini con dati personali in `%TEMP%` restano se il processo viene ucciso: pulizia dei residui all'avvio.
7. `wia.go:130`, `options.go:89`: dopo il timeout il goroutine COM continua e lo scanner puo' restare occupato. Oggi regge perche' il comando termina con `os.Exit(1)`; non va usato come servizio di lunga durata.
8. `viz.go:104-106`: gli errori OCR dei ritagli sono ignorati senza traccia e non c'e' nessun log: un fallimento si vede solo come campi mancanti.

## Bassa
9. `Rotate` e `CropContent` usano `At`/`Set` per pixel: con accesso diretto a `Pix` sarebbero circa 10 volte piu' veloci, ma il risparmio reale e' circa 0,5 s.
10. Un solo codice di uscita (1) per ogni errore: codici distinti per carta assente, inceppamento, occupato; una riga di log per fase con durata e numero di chiamate OCR, senza testo OCR ne' dati personali.
Documentare che la cartella di `-debug` va cancellata.

## Punti corretti
Il tentativo di riconnessione su occupato/riscaldamento (`wia.go:334-345`); `Transfer` mai ripetuto (evita doppie scansioni e carta consumata); messaggi di errore WIA in italiano e senza dati personali.
