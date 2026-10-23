# 10 · Revisione della sicurezza (`architect-security-reviewer`)

**Lente:** sicurezza applicativa e protezione dei dati personali. Sola lettura, nessuna scansione eseguita.
**Non verificato:** ACL reali di `%TEMP%` e `scans\`; contenuto di `scans\prova1.json`; esito di `go mod`/`govulncheck`; assenza di CI e repository git.

## Alta
Nessun rilievo di gravita' alta dimostrabile.

## Media
1. **Dati personali in chiaro nel workspace.** `scans\prova1.json` e `prova1.err` non sono coperti da `.gitignore` (`/*.json` vale solo per la radice); con `git add .` finirebbero in un repository.
   `scans\` contiene dati reali (anche `scans\dbg*\...\mrz_ocr.txt`). *Rimedio:* `scans/` e `_viz_exp3/` in `.gitignore`; spostare le scansioni in una cartella cifrata (BitLocker/EFS).
2. **`-debug` salva dati personali senza protezione** (`extract.go:91-93`, `mrzband.go:411,448`): permessi 0o755/0o644 (su Windows contano le ACL ereditate). Scenario: un altro utente locale legge la cartella, o resta su un disco condiviso senza scadenza.
   *Rimedio:* 0o700/0o600, avviso su stderr "salvo dati personali in X", cancellazione a fine lavoro, oppure `-debug` solo con variabile d'ambiente.
3. **Sovrascrittura senza conferma** (`wia.go:224,240`): `os.Rename` e `SavePNG` su `-o` distruggono la scansione precedente. *Rimedio:* rifiutare un file esistente, salvo `-force`.
4. **Nessuna protezione contro scansioni involontarie** (`main.go:102,122`): `scan` e `read` avviano subito il caricatore. Le protezioni di `wia.go:139-160` (simplex, `Pages=1`) limitano il danno ma non evitano l'avvio.
   *Rimedio:* prompt "Premi Invio" se stdin e' un terminale, con `-yes` per gli script.

## Bassa
5. `imgproc.go:20`: `image.Decode` senza `DecodeConfig`; un PNG di pochi KB che dichiara dimensioni enormi puo' esaurire la memoria. Aggiungere `DecodeConfig` e un limite (es. 20000x20000).
6. `bmp.go:32-42`: limiti presenti (<=32768, controllo della lunghezza dei dati), nessun overflow a 64 bit trovato; resta un'allocazione RGBA fino a 4 GB con un BMP enorme. Accettabile: il file arriva dal driver.
7. `ocr.go:78`: `powershell.exe` risolto via PATH; usare il percorso completo in System32. **Nessuna iniezione**: gli argomenti sono percorsi assoluti passati a `-File`; lo script (0o600, in una cartella `MkdirTemp` con `RemoveAll`) e' ragionevole; `ExecutionPolicy Bypass` e' limitato a uno script incorporato.
8. `wia.go:211`: file temporaneo `OutPath.wia.ext` con nome prevedibile accanto a una cartella potenzialmente condivisa: usare `CreateTemp` nella stessa cartella.
9. `go.mod`: `x/sys v0.1.0` (2022) e' vecchio: `go get golang.org/x/sys@latest`, poi `govulncheck`.

## Nessun problema trovato
Dati personali nei log: nessuno (gli errori riportano solo codici HRESULT, dimensioni e nomi dello scanner). Segreti: nessuno. COM: apartment STA e `Release` corretti.
