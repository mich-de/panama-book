# 06 · Revisione del modello dati (`architect-data-reviewer`)

**Lente:** schema del JSON in uscita e integrita', file su disco, dati personali. Non c'e' un database. Sola lettura, nessuna esecuzione.

## Alta
1. **`-debug` salva dati personali senza avvisi, scadenza o cancellazione** (`extract.go` 91-94, `mrzband.go` 411-451): `page.png`, fasce MRZ, `page.txt`, `mrz_ocr.txt`.
   In `scans/` ci sono `dbg1`, `dbg3`, `dbg4` e `prova1.json`. Il `.gitignore` copre `dbg*/` e `*.png` ma **non** `*.json`, `*.err` ne' `scans/`.
   *Correzione:* `scans/` in `.gitignore`; avviso su stderr "`-debug` salva dati personali in <dir>"; valutare un comando `libretto purge`.
2. **Temporanei in `%TEMP%`** (`libretto-*.png`, `vizcrop*`, `mrzband*`, `scan.png.wia.*`): rimossi con `defer`, quindi restano se il processo viene ucciso. Trovato 1 residuo (contenuto non letto).
   *Correzione:* all'avvio eliminare i `libretto-*` piu' vecchi di 1 ora; una sola cartella temporanea per esecuzione.

## Media
3. **`Result.Fields` e' `map[string]string`** (`extract.go:30`): nessuno schema, nessun controllo sui nomi; `viz.go` legge le chiavi per stringa e un refuso passa in silenzio.
   Chiavi miste: `nacionalidad` (ES), `nazionalita_iso`, `cognome`, `sesso_viz`, `data_nascita_mrz` (IT/ES). *Correzione:* una struct `Printed` e una `Derived`, con costanti per le chiavi.
4. **Nessuna versione dello schema** e un campo vuoto non si distingue da uno non letto: `extract.go` scrive sempre `""` per i campi MRZ, `viz.go` cancella la chiave.
   *Correzione:* `schema_version` e `omitempty` o `*string`.

## Bassa
5. Date in formati misti: `birth_date` ISO nell'MRZ, `data_*` stampate `DD-MM-YYYY` piu' `_iso`. Solo ISO; la forma stampata in un campo `printed`.
6. `extract.go` stampa `%+v` di `Checks`: nessun dato personale, ma non strutturato; meglio un campo `checks`.
7. Gli avvisi sono testo libero e usati come chiavi (`vizHasWarning`): usare codici (es. `MRZ_MISMATCH_DOCNO`).
8. `VIZ_NOCROPS` cambia l'output senza essere documentata.

## Schema proposto
```json
{"schema_version":1,"rotation_deg":0,
 "mrz":{"...":"...","checks":{"...":true}},
 "printed":{"nationality":"","issue_date":{"printed":"","iso":""}},
 "warnings":[{"code":"","field":""}]}
```
**Come provarlo:** su una copia di `scans/prova1.png`, con `libretto extract` e confronto dei JSON.
