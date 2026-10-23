# 05 · Revisione dei confini (`architect-boundary-reviewer`)

**Lente:** accoppiamenti tra pacchetti, stato globale, duplicati, moduli troppo grandi. Sola lettura.

**Grafo delle dipendenze: pulito.** `cmd` -> `libreta`, `wia`, `imgproc`; `libreta` -> `imgproc`, `mrz`, `ocr`; `wia` -> `imgproc` (solo per la BMP).
Nessun ciclo; `imgproc` non dipende da pacchetti interni. I rilievi 1-3 sono dimostrati, il 4 e' un sospetto.

## Alta
1. **Stato globale `vizStash` con mutex** (`viz.go` righe 59-80, scritto a 516, letto a 1090; `extract.go` 76 e 98). E' un canale laterale tra `prepareVIZImage` ed `extractVIZ`,
   riconosciuto dalle dimensioni del foglio. Due `Extract` concorrenti si sovrascrivono i ritagli; un OCR di pagina intera con la stessa dimensione userebbe i ritagli sbagliati.
   *Correzione piccola:* `prepareVIZImage` restituisce `(image.Image, *vizCropSet)` e `extractVIZ` riceve il set come parametro.
2. **`viz.go` duplica `imgproc/rows.go`**: `vizToGray`/`vizInk` contro `Binarize`, `vizTextRows` contro `TextRows`, `vizMainSpan` contro `MainSpan`, `vizThin` contro `Erode`/`ThinToInk`, `vizStrokeWidth` contro `StrokeWidth`.
   `mrzband.go` usa invece `imgproc`. *Correzione:* far usare `imgproc` a `viz`, dopo che gli agenti hanno finito; i due tipi `Row` differiscono (`vizRow` ha il campo `runs`): confrontare i comportamenti con i test prima di sostituire.
3. **`viz.go` (2051 righe)** mescola geometria, immagini, OCR dei ritagli, riconoscimento valori, modo pagina e finalizzazione. *Correzione:* spezzare nello stesso package in
   `viz_sheet.go`, `viz_values.go`, `viz_page.go`, `viz_finalize.go`: solo spostamenti, nessun cambio di logica.

## Media
4. `viz.go:1090` `os.Getenv("VIZ_NOCROPS")`: configurazione nascosta nella logica; meglio un campo in `Options` (sospetto: forse solo per debug).
5. `regex.go` `reCache`: mappa senza mutex, accesso concorrente non sicuro; in `viz.go` ci sono gia' regexp globali precompilate. Sostituire con `var` precompilate.
6. `mrzClean` (`mrzband.go`) e `mrz.cleanOCR` (`fix.go`) fanno quasi lo stesso (il secondo scarta piu' caratteri e non gestisce `«‹〈＜≪`); `mrz.clean` e' un terzo, piu' debole. Un solo `mrz.CleanLine` esportato.

## Bassa
7. Helper duplicati: `itoa` a mano in `fix.go` (usare `strconv.Itoa`), `minf/maxf/absf` in `extract.go` e `mini/maxi/clampi` in `imgproc.go` (Go 1.22 ha `min`/`max`), `abs/absInt/acAbs` in tre file.
8. API esportate usate solo nel pacchetto: `mrz.Normalize`, `mrz.CheckDigit` (test non verificati), `imgproc.Binarize/Erode/StrokeWidth`.
9. File non di produzione nel modulo: `_out/`, `_viz_exp*`, `_old`, `zz_test.go`.

**Non verificato:** duplicazioni dentro `mrzband.go`; contenuto di `cmd/libretto/main.go`.
