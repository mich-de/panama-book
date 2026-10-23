# 12 · Agente MRZ a template (`af33114ed9381b4a6`)

**Ruolo:** leggere le due righe MRZ (ICAO TD3, `IS` + `PAN`) **senza il motore OCR di Windows**, confrontando ogni carattere con una libreria di
bitmap di glifi e validando il risultato con i check digit. E' la seconda strada per le scansioni su cui l'OCR restituisce testo vuoto (es.
`scans\provamrz.png` e `scans\DEFINITIVO_spero.png`, TWAIN pulite e dritte).
**File suoi:** `internal/libreta/mrzcells.go` (geometria, lettura, aggancio), `mrzcells_glyph.go` (libreria e classificazione),
`mrzcells_decode.go` (decodifica con i vincoli), `glyphs.bin` (libreria incorporata con `go:embed`), `mrzcells_test.go`,
`mrzcells_decode_test.go` e, sotto build tag `glyphgen`, `mrzcells_glyphgen_test.go` e `mrzcells_eval_test.go` (generazione e valutazione).
Non ho toccato `mrzband.go`, `rows.go`, `pagecrop.go`, `deskew.go`, `extract.go`, `cmd/`, `internal/twain`, `internal/wia`, `viz*.go`.
Nessun dato personale in questa pagina ne' in `glyphs.bin` (solo bitmap di simboli e un numero d'ordine del libretto).

Formato confermato sui facsimili AMP del corpus (2020 e Anexo 8 A e B: `IS` + `PAN`, numero libretto da 9 caratteri, nazionalita' del titolare, campo opzionale di 14 caratteri con il nome dell'ufficio) e sulle 8 scansioni reali.

## Come funziona
`readMRZCells(img, debugDir) (*mrz.Data, []string, int)`: stessa immagine orientata e stessa scala di punteggio di `readMRZBand` (`scoreMRZ`, 0 = non trovato).

1. **Righe.** Come `readMRZBand`: `Deskew`, `Binarize`, `ClearLines`, `TextRows`, `TrimRowEdgeBars`, `MRZPairs` (funzioni di `imgproc`, non modificate). Senza coppia di righe torna subito.
2. **Griglia** (`tplFitGrid`). Le 44 celle hanno passo costante: si cerca (bordo sinistro, passo) che mette i 43 confini nei vuoti tra i caratteri (profilo delle colonne di
   inchiostro delle due righe, sfumato; il bordo sinistro sta accanto al primo inchiostro e quello destro dopo l'ultimo, cosi' la griglia non scivola di una cella).
   La qualita' (inchiostro sui confini / inchiostro medio) vale 0,02-0,06 sulle MRZ vere e oltre 0,9 su due righe qualunque.
3. **Riga di testo** (`tplRowLine`). Per ogni cella gli estremi verticali dell'inchiostro; retta robusta (mediana delle pendenze) per l'**inclinazione** (sul corpus fino a 1,5 gradi,
   uguale nelle due righe) e mediana per l'**altezza** dei caratteri. Le scansioni hanno la scala verticale diversa dall'orizzontale (altezza dei caratteri 0,75-1,1 passi nella riga 1,
   0,93-1,16 nella riga 2), quindi si normalizza per riga.
4. **Riquadro canonico.** Ogni cella e' ricampionata (media d'area su tabella delle somme cumulate) in un riquadro 40x40 con il passo a 20 px, centrato sulla cella e sulla retta del testo,
   altezza dei caratteri normalizzata; la copertura dell'inchiostro e' ricavata dal grigio con fondo (mediana) e inchiostro (1% piu' scuro) delle righe.
5. **Spessore del tratto** (`tplNormalizeThickness`). Misurato come 2*area/perimetro (variazione totale della copertura, indipendente dalla sfumatura) e portato a 3,3 px con
   erosione/dilatazione sulla distanza con segno dal bordo. Senza questo +2 px di inchiostro sulla scansione facevano scartare quasi tutte le letture.
6. **Vettore ridotto.** Sfumatura gaussiana (sigma 0,9), ritaglio centrale 24x30, media 2x2: 12x15 = 180 valori.
7. **Libreria** (`glyphs.bin`, 98 KB compressa, 707 glifi). Per ogni simbolo (A-Z, 0-9, `<`) al massimo 24 glifi scelti col campionamento del punto piu' lontano (tengono la varieta' di
   spessore e forma, scartano i quasi duplicati). Distanza cella-glifo = minimo su spostamenti di +-1 px (a 12x15) della somma dei quadrati; distanza cella-simbolo = minimo sui glifi.
   Probabilita' con softmax a temperatura 0,15 (entropia incrociata nel leave-one-booklet-out: minimo piatto tra 0,15 e 0,2).
8. **Decodifica con i vincoli** (`mrzcells_decode.go`), programmazione dinamica (Viterbi, e la stessa ricorsione con la somma per la probabilita' a posteriori):
   - riga 1: `ISPAN` (prior morbido) e grammatica parole `<` parole `<<` (o piu') parole `<` parole `<`...: solo lettere e `<`. **Nei libretti di Panama il separatore e' spesso `<<<`** (6 libretti su 8), e il
     blocco prima del separatore e' a volte il cognome, a volte i nomi: la grammatica non li distingue;
   - riga 2: tipo per posizione (numero libretto `PA` + 7 cifre con prior morbido, nazionalita' in lista ICAO, sesso M/F, date con mese 01-12 e giorno 01-31, campo opzionale di lettere) e i **cinque check digit
     come vincolo esatto** (stato = somma del composito x somma del campo, 200 stati). Il risultato e' la stringa **piu' probabile secondo il classificatore** che rispetta tutti i check, non una
     qualunque: una correzione si fa solo se costa poco.
9. **Plausibilita'.** La decodifica trova sempre una stringa con i check validi, anche da rumore o da testo capovolto: si accetta (punteggio `scoreMRZ`, 13 se PAN) solo se la griglia e' buona (< 0,25), la distanza
   mediana cella-glifo < 1,8 (0,2-0,6 sulle MRZ vere; 2,4-5,2 sulle stesse righe capovolte), la probabilita' a posteriori della riga 2 >= 0,5 (riga 1 >= 0,02) e il **costo dei vincoli** (evidenza persa rispetto al simbolo piu' vicino di ogni cella) <= 8 nat nella riga 2 e <= 20 nella riga 1
   (riga 2: 0,0 su tutte le letture giuste; 44 nat se la MRZ stampata ha un check digit sbagliato: il lettore non inventa la cifra).
10. **Note** (`[]string`): `riga2 pos 7: O→0 (vincoli MRZ del lettore a template)` per le celle cambiate dai vincoli; `riga1 pos 22: lettura incerta (O)` per le celle con probabilita' < 70% o lontane
    da ogni glifo (distanza > 1,9: forma mai vista). Nel leave-one-booklet-out **tutte le 8 celle sbagliate erano segnalate, nessuna sbagliata in silenzio** (55 celle giuste segnalate su 4752).

## Corpus ed etichette
54 immagini con MRZ leggibile (da `scans\`, `scans\vecchi test WIA\`, `facsimili\panama\`), **8 libretti** (28, 9, 5, 4, 3, 3, 1 e 1 scansioni: stessa riga 2 = stesso libretto). L'etichetta e'
il testo MRZ validato: la riga 2 dai check digit; la riga 1, che non ha check, **dall'OCR piu' un controllo a occhio**. Scoperta: **5 immagini su 54 avevano l'etichetta ricavata dall'OCR sbagliata
pur con 5/5 check**: 4 nella riga 1 (`readMRZBand` lascia buchi sui `<` e sui nomi lunghi) e 1 nella nazionalita' della riga 2, che i check non coprono; per due libretti la riga 1 era sbagliata nell'unica scansione (1 su 1) o in 2 su 3. Le etichette curate stanno fuori dal repository
(contengono dati personali). **Controllo indipendente:** le righe 1 curate coincidono con la riga del nome scritta a occhio dal verificatore (`scans\verita\*.json`, chiave `_riga_nome`, parole senza ordine) in **52 immagini su 52** (2 senza verita'), quindi anche le due trascrizioni a mano; numero libretto e date della riga 2 coincidono con i campi stampati nelle stesse 52. Un'etichetta sbagliata avvelenerebbe la libreria: per questo il generatore (`TestGlyphgenDump`) fa votare le scansioni dello stesso libretto e segnala quelli con meno di 3 scansioni
o con celle discordanti: la loro riga 1 va controllata a occhio.

## Valutazione onesta: leave-one-booklet-out
Quando si legge una scansione la libreria **non contiene nessuna cella dello stesso libretto** (stessa riga 2: per `provamrz` e `DEFINITIVO_spero` sono escluse anche `scana`, `scana2`, `vero16` e le altre scansioni
dello stesso titolare, 28 in tutto) e viene ricostruita e potata dalle celle degli altri 7 libretti come in produzione. Le costanti (temperatura, soglie, spessore, altezza) sono tarate sullo stesso corpus: lieve ottimismo.
Esecuzione: `TPL_LABELS=<labels.json> go test -tags glyphgen -run TestGlyphgenEval -v ./internal/libreta`.

**Riepilogo (54 immagini):** riga 1 **99,66%** dei caratteri, riga 2 **100,00%**; MRZ completa e giusta 47/54; check digit 5/5 e lettura plausibile **54/54**; **158 ms** a immagine (circa 79 ms per riga).
Le 7 immagini imperfette hanno una lettera sbagliata nella riga 1 (mai nella riga 2): lettere che nel corpus compaiono in un solo libretto (`Q`; `B` e `Y`): escludendo quel libretto la libreria non le ha mai viste,
e il facsimile 2020 a tratto sottile (due lettere). Senza i vincoli la riga 1 sarebbe il 99,49% e la riga 2 il 100%.

**`scans\provamrz.png`: riga 1 44/44, riga 2 44/44, check 5/5, 147 ms. `scans\DEFINITIVO_spero.png`: 44/44 e 44/44, check 5/5, 163 ms**, senza usare l'OCR e senza nessuna cella dello stesso titolare nella libreria.

### Tabella per immagine (leave-one-booklet-out)

| Immagine | Libretto | Riga 1 | Riga 2 | Check digit | Punteggio | ms |
|---|---|---|---|---|---|---|
| `facsimili/panama/libreta_2020_pagina-dati_QR.jpg` | 0 | 42/44 | 44/44 | 5/5 | 13 | 174 |
| `facsimili/panama/libreta_2026_anexo8_pagina-dati_A.jpg` | 2 | 44/44 | 44/44 | 5/5 | 13 | 91 |
| `facsimili/panama/libreta_2026_anexo8_pagina-dati_B.jpg` | 1 | 43/44 | 44/44 | 5/5 | 13 | 100 |
| `facsimili/panama/panama.png` | 3 | 44/44 | 44/44 | 5/5 | 13 | 87 |
| `scans/DEFINITIVO_spero.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 113 |
| `scans/ScanTest00000001.jpg` | 2 | 44/44 | 44/44 | 5/5 | 13 | 99 |
| `scans/scana.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 55 |
| `scans/scana2.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 111 |
| `scans/provamrz.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 106 |
| `scans/vecchi test WIA/a4-1-pagina.png` | 1 | 43/44 | 44/44 | 5/5 | 13 | 108 |
| `scans/vecchi test WIA/a4-1.png` | 1 | 43/44 | 44/44 | 5/5 | 13 | 174 |
| `scans/vecchi test WIA/dimensioni_libretto_panama.png` | 6 | 44/44 | 44/44 | 5/5 | 13 | 169 |
| `scans/vecchi test WIA/libretto-2-grezza.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 174 |
| `scans/vecchi test WIA/libretto-2-pagina.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 111 |
| `scans/vecchi test WIA/libretto-2.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 175 |
| `scans/vecchi test WIA/libretto-vero1-grezza.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 260 |
| `scans/vecchi test WIA/libretto-vero1-pagina.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 111 |
| `scans/vecchi test WIA/libretto-vero1.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 252 |
| `scans/vecchi test WIA/libretto-vero2-grezza.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 254 |
| `scans/vecchi test WIA/libretto-vero2-pagina.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 108 |
| `scans/vecchi test WIA/libretto-vero2.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 244 |
| `scans/vecchi test WIA/libretto-vero3-grezza.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 258 |
| `scans/vecchi test WIA/libretto-vero3-pagina.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 102 |
| `scans/vecchi test WIA/libretto-vero3.png` | 7 | 44/44 | 44/44 | 5/5 | 13 | 219 |
| `scans/vecchi test WIA/libretto-vero4-grezza.png` | 6 | 44/44 | 44/44 | 5/5 | 13 | 243 |
| `scans/vecchi test WIA/libretto-vero4-pagina.png` | 6 | 44/44 | 44/44 | 5/5 | 13 | 103 |
| `scans/vecchi test WIA/libretto-vero4.png` | 6 | 44/44 | 44/44 | 5/5 | 13 | 239 |
| `scans/vecchi test WIA/libretto-vero5-grezza.png` | 5 | 43/44 | 44/44 | 5/5 | 13 | 252 |
| `scans/vecchi test WIA/libretto-vero5-pagina.png` | 5 | 43/44 | 44/44 | 5/5 | 13 | 104 |
| `scans/vecchi test WIA/libretto-vero5.png` | 5 | 43/44 | 44/44 | 5/5 | 13 | 205 |
| `scans/vecchi test WIA/libretto-vero6-grezza.png` | 4 | 44/44 | 44/44 | 5/5 | 13 | 232 |
| `scans/vecchi test WIA/libretto-vero6.png` | 4 | 44/44 | 44/44 | 5/5 | 13 | 196 |
| `scans/vecchi test WIA/libretto-vero7.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 198 |
| `scans/vecchi test WIA/librettook1-pagina2.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 101 |
| `scans/vecchi test WIA/librettook1.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 222 |
| `scans/vecchi test WIA/librettook11-grezza.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 173 |
| `scans/vecchi test WIA/librettook11-pagina.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 104 |
| `scans/vecchi test WIA/librettook11.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 175 |
| `scans/vecchi test WIA/librettook4-grezza.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 179 |
| `scans/vecchi test WIA/librettook4-pagina.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 102 |
| `scans/vecchi test WIA/librettook4.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 178 |
| `scans/vecchi test WIA/librettook7-grezza.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 178 |
| `scans/vecchi test WIA/librettook7-pagina.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 104 |
| `scans/vecchi test WIA/librettook7.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 174 |
| `scans/vecchi test WIA/prova1.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 170 |
| `scans/vecchi test WIA/vero-pagina.png` | 4 | 44/44 | 44/44 | 5/5 | 13 | 106 |
| `scans/vecchi test WIA/vero.png` | 4 | 44/44 | 44/44 | 5/5 | 13 | 226 |
| `scans/vecchi test WIA/vero6.png` | 6 | 44/44 | 44/44 | 5/5 | 13 | 246 |
| `scans/vero10.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 57 |
| `scans/vero12.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 118 |
| `scans/vero13.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 106 |
| `scans/vero14.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 133 |
| `scans/vero15.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 129 |
| `scans/vero16.png` | 2 | 44/44 | 44/44 | 5/5 | 13 | 127 |

(Tempi: lettura intera di `readMRZCells`, front-end incluso, a leave-one-booklet-out; media 158 ms.)

### Righe capovolte e pagine senza MRZ (falsi positivi)
Le 54 immagini ruotate di 180, 90 e 270 gradi (162 letture: `Extract` chiama il lettore a ogni rotazione): **0 falsi positivi**, al massimo 372 ms. A 180 gradi le righe si trovano ma la distanza mediana
sale a 2,4-5,2. Immagini del corpus senza etichetta: nessuna lettura plausibile tranne `vero9.png` a 180 gradi (check 5/5, probabilita' 1,00; l'OCR non la leggeva): e' un'altra scansione di un libretto
gia' nel corpus e la sua riga 2 coincide con quella validata. Prove sintetiche: pagine bianche, nere, rumore, bande, barre regolari, una sola riga, riga con check digit stampato sbagliato: nessuna lettura.

### Robustezza (leave-one-booklet-out, 14 immagini: 2 per libretto)
| Degrado | 10 scansioni a 300 dpi (riga 1 / riga 2 / plausibili) | 4 facsimili a 150 dpi |
|---|---|---|
| nessuno | 99,3% / 100% / 10 su 10 | 98,3% / 100% / 4 su 4 |
| risoluzione 70% | 98,9% / 100% / 10 su 10 | 97,7% / 100% / 4 su 4 |
| risoluzione 50% | 98,9% / 100% / 10 su 10 | non letti |
| sfocatura 1 px | 99,1% / 100% / 10 su 10 | 97,7% / 100% / 4 su 4 |
| sfocatura 1,6 px | 99,3% / 100% / 10 su 10 | non letti |
| rumore 8 livelli | 99,3% / 100% / 10 su 10 | 98,3% / 100% / 4 su 4 |
| rumore 20 livelli (estremo) | 69,5% / 70% / 7 su 10 (l'inchiostro non si binarizza piu') | 3 su 4 |
| contrasto 50% | 99,3% / 100% / 10 su 10 | 98,3% / 100% / 4 su 4 |
| ulteriori 1,2 gradi di inclinazione | 98,6% / 100% / 10 su 10 | 3 su 4 |
| tratto piu' spesso (+2 px) | 99,1% / 100% / 9 su 10 | 3 su 4 |
| tratto piu' sottile (-2 px) | 98,4% / 100% / 10 su 10 | non letti |

### Tempi (libreria incorporata, media di 5 esecuzioni)
`scans\provamrz.png` (1560x1098): 130 ms = righe 87 (front-end di `imgproc`) + griglia e celle 17 + classificazione e decodifica 35. Pagina da 2638x2255: 340 ms (il front-end sale a 277). Su una rotazione sbagliata
senza coppia di righe costa solo il front-end.

## Come agganciarlo
Gia' fatto: `func init() { mrzCellsReader = readMRZCells }` in `mrzcells.go` (la variabile e' in `extract.go`). `Extract` lo chiama dopo `readMRZBand` quando questo non trova l'MRZ o non tornano tutti i check.
Candidata: `_out\libretto_template.exe`. Suggerimento all'integratore: con 4 immagini su 54 a riga 1 sbagliata (e 1 a nazionalita' sbagliata) nell'etichetta OCR nonostante i 5/5 check, conviene far girare il lettore a celle **anche quando l'OCR ha 5/5**
(costa ~0,15 s) e, se le due righe 1 differiscono, avvisare in `Warnings` o preferire quella a celle senza note di incertezza.

## Come rigenerare la libreria
1. `TPL_OUT=<cartella> go test -tags glyphgen -run TestGlyphgenDump -timeout 60m ./internal/libreta` legge il corpus con l'OCR e scrive `<cartella>\labels.json` (voto tra le scansioni dello stesso libretto; ~6 minuti la prima volta, poi riusa le etichette in cache).
2. Controllare a occhio la riga 1 dei libretti segnalati e correggerla nel file. La riga 2 e' validata dai check.
3. `TPL_LABELS=<cartella>\labels.json TPL_WRITE=1 go test -tags glyphgen -run TestGlyphgenBuild ./internal/libreta` scrive `internal/libreta/glyphs.bin` (deterministico). `TPL_CAP` = glifi per simbolo (24).
4. `TestGlyphgenEval`, `TestGlyphgenNegative`, `TestGlyphgenStress`, `TestGlyphgenTiming` ripetono le misure di questa pagina.
La `W` non e' mai comparsa nel corpus: il generatore ne ricava 5 sintetiche componendo due mezze `V` reali (`TPL_NOSYNTH=1` le esclude).

## Non verificato / limiti
- **Solo 8 libretti.** Lettere rare nel corpus: `B` 3 glifi, `Q` 3, `Z` 1, `X` 1 (a tratto sottile), `Y` 4, `V` 5; **`W` sintetica, mai vista su una scansione**: la sua affidabilita' non e' misurabile. Una lettera nuova viene
  letta come la piu' vicina e segnalata come incerta (distanza > 1,9), ma la riga 1 non ha check digit e puo' restare sbagliata: confrontarla con i nomi stampati (VIZ).
- Le costanti (temperatura 0,15, soglie di plausibilita', spessore 3,3 px, altezze 1,05/1,11 passi, 24 glifi per simbolo) sono tarate sullo stesso corpus del leave-one-booklet-out.
- Il front-end e' quello di `imgproc` (non modificato): con rumore molto forte o facsimili a bassa risoluzione degradati le righe non si trovano. Si leggono solo le due righe MRZ della pagina dati.
- Solo scansioni a 300 dpi dei due percorsi del corpus (WIA e TWAIN del fi-800R) e facsimili digitali a 150 dpi; nessun'altra stampante, ne' foto.
- La libreria incorporata contiene anche le scansioni dei libretti usati per le prove: in produzione sullo stesso titolare il risultato non e' una misura indipendente (usare `TestGlyphgenEval`).

## File temporanei (da cancellare con il tuo ok)
`_out\mrz_tpl_base.exe` (eseguibile del 10:36, superato); `_out\libretto_template.exe` (candidata, da tenere finche' l'integratore non l'ha valutata). Nello scratchpad della sessione (fuori dal progetto): estrazioni dei riquadri
e `labels_curated.json` con **testo MRZ vero**: eliminarli alla chiusura della sessione.
