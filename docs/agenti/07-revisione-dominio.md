# 07 · Revisione del dominio (`architect-domain-modeler`)

**Lente:** concetti del dominio e linguaggio ubiquo; dove tagliare i moduli e dove mettere le regole. Sola lettura.

## Glossario proposto (italiano, con il termine stampato tra parentesi)
| Termine | Significato |
|---|---|
| **Libreta** (libretto) | il documento, con modello 2017/2020/2026 |
| **PaginaDati** | la scansione di una facciata |
| **VIZ** | zona visiva |
| **MRZ** | TD3, con 5 controlli (`ControlliMRZ`) |
| **Titolare** | nazionalita' ISO (dall'MRZ) e demonimo (dalla VIZ) |
| **Qualifica** | cargo / capacity |
| **Vidimazione** | refrendo / endorsement (solo modello 2026) |
| **UfficioRegionale** | regional |

## Problemi (fatti verificati)
1. **Alta — manca un tipo `Libreta`.** `Result.Fields map[string]string` (`extract.go:30`) mescola chiavi IT, ES ed EN (`nacionalidad`, `cargo`, `refrendo`, `regionale`, `nazionalita_iso`);
   i sinonimi `_mrz` e `_iso` sono chiavi stringa. Le invarianti sono sparse e le chiavi non tipizzate.
2. **Alta — le regole di dominio stanno in `viz.go`** (2051 righe) insieme alla geometria dell'immagine: `vizYearRange`, `vizDemonyms`, `vizFinalize` con la regola
   "refrendo solo se `PA`", il confronto con l'MRZ e le date; lo snap di `SI`/`PAN`.
3. **Media — `crossCheck` e `vizFinalize` si sovrappongono.** Il confronto sul numero libretto e' duplicato; il doppio avviso si evita con `vizHasWarning` che confronta prefissi di stringa.
4. **Media — `scoreMRZ`** mette una regola di Panama (+2 se `Issuer=="PAN"`) nel ranking dell'OCR; l'invariante "MRZ valida = 5 check" e' ripetuta a mano invece di usare `Checks.AllOK`.
5. **Media — `mrz.date()`** usa `time.Now()`; la regola "l'MRZ prevale" esiste solo come sovrascrittura delle chiavi in `crossCheck`.
6. **Bassa — mix di lingue** (`regionale_mrz`, `sesso_viz`); la differenza `IS` (MRZ) / `SI` (VIZ) e' documentata solo nei commenti.

## Tre spostamenti consigliati (3 file, un solo pacchetto, `cmd/libretto` non cambia)
1. `libreta/modello.go`: `type Libreta` con campi tipizzati, `Modello()` derivato dal prefisso del numero e `Verifica()` che restituisce gli avvisi. Il JSON resta invariato con un `Fields()` di compatibilita'. Test: i golden test di `viz_test.go` e `mrzband_test.go`.
2. `libreta/regole.go`: spostare `vizYearRange`, `vizDemonyms`, la regola del refrendo e i confronti MRZ/VIZ come funzioni pure, senza OCR. Test: tabelle sintetiche nuove. Serve anche a `crossCheck`.
3. Tenere in `mrz` solo ICAO; portare `Issuer=="PAN"` fuori da `scoreMRZ` nel pacchetto `libreta` e usare `AllOK`.
