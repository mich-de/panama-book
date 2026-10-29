# 08 · Piano di rifattorizzazione (`architect-migration-planner`)

**Lente:** piano graduale, non codice. **Non applicato.** Il piano parte solo quando gli agenti MRZ e VIZ dichiarano "finito" e i loro file sono fermi.
Go 1.22 (quindi `min`/`max` predefiniti); la cartella **non e' un repository git**.

## Obiettivi
1. spezzare `viz.go` (~2000 righe) senza cambiare comportamento;
2. eliminare la variabile globale con mutex che passa il "foglio" di ritagli;
3. dare un `context` a `extractVIZ`;
4. armonizzare le chiavi JSON (oggi miste ES/IT);
5. rimuovere i duplicati (`itoa`, `mini/maxi`, `mrzClean/cleanOCR`, `regexpCache`, `Options.Scale`).

## Passi
| # | Cosa | Rischio | Verifica |
|---|---|---|---|
| 0 | **Sincronizzazione e baseline**: file congelati, snapshot (`git init` o copia fuori albero: **decisione**), `go vet ./... && go test ./...`, poi integrazione in serie (MRZ 5/5 su 4 immagini, 12-14/14 campi). Ritorno indietro = ripristino dallo snapshot | basso | baseline |
| 1 | `mini/maxi` e `minf/maxf` diventano `min/max`; `itoa` diventa `strconv.Itoa` | basso | vet + test |
| 2 | `mrz.CleanOCR` esportata (equivalente a `mrzClean`); si cancella `mrzClean` | basso | test mrz e mrzband |
| 3 | `regexpCache` diventa una `var regexp.MustCompile`; si elimina `regex.go`; si rimuove `Options.Scale` (nessuno la legge) | basso | test |
| 4 | variabile globale: `prepareVIZImage` restituisce `(image.Image, *vizCropSet)`; si eliminano `vizStash*`; si adattano `extract.go`, `viz_test.go`, `viz_integration_test.go`. Resta il riconoscimento del foglio dalle dimensioni (i test sintetici passano `crops` nil) | medio | test + integrazione |
| 5 | `extractVIZ(ctx, res, out, crops)` e `vizOCRCrops(ctx, c)` con `WithTimeout(ctx, 60s)` al posto di `Background` | medio | test con `ctx` annullato, nessun blocco |
| 6 | split di `viz.go` nello stesso package, un file per passo: `viz_sheet` (geometria), `viz_locate` (gray/ink/MRZ/skew), `viz_text` (norm/Levenshtein/etichette), `viz_words` (box/linee), `viz_parse` (date, numeri, livello), `viz_read` (extractVIZ, voti), `viz_page`, `viz_finalize` | basso | elenco ordinato di funzioni/tipi/var identico prima e dopo, gofmt, vet, test |
| 7 | chiavi JSON: prima costanti per le chiavi (~60 letterali), poi rinomina. Compatibilita' additiva: `schema_version` e opzione per emettere i nomi legacy. Solo `nacionalidad` contrasta con `nazionalita_iso` | **alto** (chi consuma il JSON) | test golden dei set di chiavi + integrazione |

**Parallelismo:** nessuno dentro `libreta` (stesso package). I passi 1 e 2 toccano entrambi `mrz/fix.go`: restano sequenziali. L'integrazione e' sempre in serie (OCR via PowerShell).
**Decisioni richieste:** snapshot o git; lingua target delle chiavi (ES o IT) e se esistono consumatori del JSON.
**Rischi:** OCR non deterministico (confronto solo per conteggi); assenza di git.
