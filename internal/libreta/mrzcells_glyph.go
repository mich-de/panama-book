package libreta

// Libreria di glifi e classificazione delle celle del lettore MRZ a template (vedi mrzcells.go).
//
// Ogni cella e' un riquadro canonico 40x40 (copertura dell'inchiostro 0..255, scala e posizione normalizzate dalla
// geometria della riga). Per confrontarla con i glifi della libreria si riduce: sfumatura gaussiana (sigma 0,9),
// ritaglio centrale 24x30 e media 2x2, cioe' 12x15 = 180 valori in 0..1. La distanza tra due celle e' la minima, su
// spostamenti di +-1 px (a quella risoluzione), della somma dei quadrati delle differenze: tollera gli errori di
// qualche decimo di pixel nella posizione della griglia e i residui dell'inclinazione. La distanza di una cella da
// un simbolo e' la minima sui glifi di quel simbolo; da li' si ricavano probabilita' con una softmax a temperatura
// fissa (tplTemp), che alimentano la decodifica con i vincoli della struttura MRZ (mrzcells_decode.go).
//
// La libreria (glyphs.bin) contiene soltanto bitmap di glifi 12x15 con il simbolo e un numero d'ordine del libretto
// di provenienza (serve alla valutazione "leave-one-booklet-out"): nessun testo, nome o numero di persone. Si
// rigenera con il test opt-in di mrzcells_glyphgen_test.go (build tag glyphgen).

import (
	"bytes"
	"compress/flate"
	_ "embed"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
)

const (
	tplCropU0, tplCropU1 = 8, 32 // colonne del riquadro canonico che si usano (24: un passo e mezzo)
	tplCropV0, tplCropV1 = 5, 35 // righe del riquadro canonico che si usano (30)
	tplRedW              = (tplCropU1 - tplCropU0) / 2
	tplRedH              = (tplCropV1 - tplCropV0) / 2
	tplRedN              = tplRedW * tplRedH // 180
)

// tplVec e' una cella ridotta: copertura 0..1, riga per riga.
type tplVec [tplRedN]float32

// tplSigma: sfumatura gaussiana (px del riquadro canonico) applicata prima di ridurre.
const tplSigma = 0.9

var tplKernel = func() []float32 {
	const rad = 3
	k := make([]float32, 2*rad+1)
	var s float64
	for i := range k {
		d := float64(i - rad)
		v := math.Exp(-d * d / (2 * tplSigma * tplSigma))
		k[i] = float32(v)
		s += v
	}
	for i := range k {
		k[i] /= float32(s)
	}
	return k
}()

// tplReduce riduce il riquadro canonico (tplFrame x tplFrame, 0..255) a un vettore di tplRedN valori.
func tplReduce(fr []uint8) tplVec {
	const rad = 3
	// sfumatura separabile sulla sola zona che serve (ritaglio piu' il raggio del filtro)
	v0, v1 := max(0, tplCropV0-rad), min(tplFrame, tplCropV1+rad)
	u0, u1 := max(0, tplCropU0-rad), min(tplFrame, tplCropU1+rad)
	tmp := make([]float32, tplFrame*tplFrame)
	for y := v0; y < v1; y++ { // orizzontale
		for x := tplCropU0; x < tplCropU1; x++ {
			var s float32
			for j, kv := range tplKernel {
				xx := min(max(x+j-rad, u0), u1-1)
				s += kv * float32(fr[y*tplFrame+xx])
			}
			tmp[y*tplFrame+x] = s / 255
		}
	}
	var out tplVec
	var blur [tplCropV1 - tplCropV0][tplCropU1 - tplCropU0]float32
	for y := tplCropV0; y < tplCropV1; y++ { // verticale
		for x := tplCropU0; x < tplCropU1; x++ {
			var s float32
			for j, kv := range tplKernel {
				yy := min(max(y+j-rad, v0), v1-1)
				s += kv * tmp[yy*tplFrame+x]
			}
			blur[y-tplCropV0][x-tplCropU0] = s
		}
	}
	for j := 0; j < tplRedH; j++ { // media 2x2
		for i := 0; i < tplRedW; i++ {
			out[j*tplRedW+i] = (blur[2*j][2*i] + blur[2*j][2*i+1] + blur[2*j+1][2*i] + blur[2*j+1][2*i+1]) / 4
		}
	}
	return out
}

// tplShifts: 9 versioni del vettore spostate di -1, 0, +1 px in orizzontale e verticale (bordi a zero = fondo).
func tplShifts(v *tplVec) [9]tplVec {
	var out [9]tplVec
	n := 0
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			for j := 0; j < tplRedH; j++ {
				for i := 0; i < tplRedW; i++ {
					sj, si := j+dy, i+dx
					if sj >= 0 && sj < tplRedH && si >= 0 && si < tplRedW {
						out[n][j*tplRedW+i] = v[sj*tplRedW+si]
					}
				}
			}
			n++
		}
	}
	return out
}

// tplGlyph e' un glifo della libreria.
type tplGlyph struct {
	sym   uint8  // indice in tplAlphabet
	group uint8  // numero d'ordine del libretto di provenienza (solo per la valutazione); tplSynthetic = glifo sintetico
	v     tplVec // cella ridotta
}

// tplSynthetic marca i glifi prodotti artificialmente (non estratti da una scansione).
const tplSynthetic = 255

// tplLibrary e' l'insieme dei glifi, indicizzato per simbolo.
type tplLibrary struct {
	glyphs    []tplGlyph
	bySym     [tplNSym][]int32
	onlySynth [tplNSym]bool // il simbolo ha solo glifi sintetici: mai visto su una scansione, lettura non verificata
}

func newTplLibrary(gl []tplGlyph) *tplLibrary {
	l := &tplLibrary{glyphs: gl}
	for i, g := range gl {
		l.bySym[g.sym] = append(l.bySym[g.sym], int32(i))
	}
	for s, idx := range l.bySym {
		l.onlySynth[s] = len(idx) > 0
		for _, gi := range idx {
			if gl[gi].group != tplSynthetic {
				l.onlySynth[s] = false
			}
		}
	}
	return l
}

// without restituisce la libreria senza i glifi del libretto indicato (valutazione leave-one-booklet-out).
func (l *tplLibrary) without(groups ...uint8) *tplLibrary {
	var gl []tplGlyph
next:
	for _, g := range l.glyphs {
		for _, x := range groups {
			if g.group == x {
				continue next
			}
		}
		gl = append(gl, g)
	}
	return newTplLibrary(gl)
}

// tplInf e' la distanza di un simbolo senza glifi nella libreria.
const tplInf = float32(1e9)

// classDist calcola, per ogni simbolo, la distanza minima della cella q dai glifi di quel simbolo (con gli
// spostamenti di +-1 px); tplInf per i simboli che la libreria non ha.
func (l *tplLibrary) classDist(q *tplVec) [tplNSym]float32 {
	sh := tplShifts(q)
	var best [tplNSym]float32
	for s := range best {
		best[s] = tplInf
	}
	for s := 0; s < tplNSym; s++ {
		for _, gi := range l.bySym[s] {
			r := &l.glyphs[gi].v
			for k := range sh {
				// somma dei quadrati con uscita anticipata appena supera la migliore del simbolo
				var d float32
				q := &sh[k]
				lim := best[s]
				for i := 0; i < tplRedN; i += 4 {
					a0, a1, a2, a3 := q[i]-r[i], q[i+1]-r[i+1], q[i+2]-r[i+2], q[i+3]-r[i+3]
					d += a0*a0 + a1*a1 + a2*a2 + a3*a3
					if d >= lim {
						break
					}
				}
				if d < best[s] {
					best[s] = d
				}
			}
		}
	}
	return best
}

// tplTemp e' la temperatura della softmax che trasforma le distanze in probabilita' (misurata sul leave-one-booklet-out
// del corpus: minimizza l'entropia incrociata sulle celle di prova).
const tplTemp = 0.15

// tplLogProbs trasforma le distanze per simbolo in log-probabilita' (softmax con temperatura tplTemp sulla differenza
// dalla distanza minima). I simboli senza glifi ricevono una probabilita' trascurabile.
func tplLogProbs(d *[tplNSym]float32) [tplNSym]float64 {
	dmin := tplInf
	for _, v := range d {
		dmin = min(dmin, v)
	}
	var out [tplNSym]float64
	if dmin >= tplInf {
		for s := range out {
			out[s] = -math.Log(float64(tplNSym))
		}
		return out
	}
	var z float64
	for s, v := range d {
		if v < tplInf {
			out[s] = -float64(v-dmin) / tplTemp
			z += math.Exp(out[s])
		}
	}
	lz := math.Log(z)
	for s, v := range d {
		if v < tplInf {
			out[s] -= lz
		} else {
			out[s] = -60
		}
	}
	return out
}

// ---------------------------------------------------------------------------------------------------------------
// File della libreria

//go:embed glyphs.bin
var tplGlyphsBin []byte

// Formato (little endian), compresso con flate: "TPLG", versione (1), larghezza, altezza, numero di glifi (uint16), poi
// per ogni glifo: simbolo, libretto, e larghezza*altezza byte di copertura (0..255).
const tplMagic = "TPLG"

// tplWriteLibrary scrive i glifi nel formato del file glyphs.bin.
func tplWriteLibrary(w io.Writer, gl []tplGlyph) error {
	var raw bytes.Buffer
	raw.WriteString(tplMagic)
	raw.WriteByte(1)
	raw.WriteByte(tplRedW)
	raw.WriteByte(tplRedH)
	_ = binary.Write(&raw, binary.LittleEndian, uint16(len(gl)))
	for _, g := range gl {
		raw.WriteByte(g.sym)
		raw.WriteByte(g.group)
		for _, v := range g.v {
			raw.WriteByte(uint8(math.Round(float64(min(1, max(0, v))) * 255)))
		}
	}
	zw, err := flate.NewWriter(w, flate.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(raw.Bytes()); err != nil {
		return err
	}
	return zw.Close()
}

// tplReadLibrary legge i glifi dal formato di tplWriteLibrary.
func tplReadLibrary(data []byte) ([]tplGlyph, error) {
	raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(data)))
	if err != nil {
		return nil, err
	}
	if len(raw) < 9 || string(raw[:4]) != tplMagic || raw[4] != 1 || raw[5] != tplRedW || raw[6] != tplRedH {
		return nil, errors.New("glyphs.bin: formato non riconosciuto")
	}
	n := int(binary.LittleEndian.Uint16(raw[7:9]))
	rec := 2 + tplRedN
	if len(raw) != 9+n*rec {
		return nil, errors.New("glyphs.bin: dimensione incoerente")
	}
	gl := make([]tplGlyph, n)
	for i := range gl {
		p := raw[9+i*rec:]
		if int(p[0]) >= tplNSym {
			return nil, errors.New("glyphs.bin: simbolo non valido")
		}
		gl[i].sym, gl[i].group = p[0], p[1]
		for j := 0; j < tplRedN; j++ {
			gl[i].v[j] = float32(p[2+j]) / 255
		}
	}
	return gl, nil
}

var (
	tplLibOnce sync.Once
	tplLib     *tplLibrary
)

// tplEmbedded restituisce la libreria incorporata (vuota se il file manca o e' illeggibile).
func tplEmbedded() *tplLibrary {
	tplLibOnce.Do(func() {
		gl, err := tplReadLibrary(tplGlyphsBin)
		if err != nil {
			gl = nil
		}
		tplLib = newTplLibrary(gl)
	})
	return tplLib
}

// tplPrune riduce i glifi a al piu' perSym per simbolo scegliendo i piu' diversi tra loro (campionamento del punto
// piu' lontano, partendo dal glifo piu' vicino alla media del simbolo): tiene la varieta' di spessore e forma
// scartando i quasi duplicati (le molte scansioni dello stesso libretto).
func tplPrune(gl []tplGlyph, perSym int) []tplGlyph {
	var bySym [tplNSym][]int
	for i, g := range gl {
		bySym[g.sym] = append(bySym[g.sym], i)
	}
	dist := func(a, b *tplVec) float32 {
		var d float32
		for i := range a {
			x := a[i] - b[i]
			d += x * x
		}
		return d
	}
	var out []tplGlyph
	for _, idx := range bySym {
		if len(idx) <= perSym {
			for _, i := range idx {
				out = append(out, gl[i])
			}
			continue
		}
		var mean tplVec
		for _, i := range idx {
			for k := range mean {
				mean[k] += gl[i].v[k] / float32(len(idx))
			}
		}
		first, fd := 0, tplInf
		for n, i := range idx {
			if d := dist(&gl[i].v, &mean); d < fd {
				first, fd = n, d
			}
		}
		chosen := []int{first}
		near := make([]float32, len(idx)) // distanza dal gia' scelto piu' vicino
		for n, i := range idx {
			near[n] = dist(&gl[i].v, &gl[idx[first]].v)
		}
		for len(chosen) < perSym {
			far, fv := -1, float32(-1)
			for n := range idx {
				if near[n] > fv {
					far, fv = n, near[n]
				}
			}
			if fv <= 0 {
				break // tutti i rimanenti sono identici a uno scelto
			}
			chosen = append(chosen, far)
			for n, i := range idx {
				near[n] = min(near[n], dist(&gl[i].v, &gl[idx[far]].v))
			}
		}
		for _, n := range chosen {
			out = append(out, gl[idx[n]])
		}
	}
	return out
}
