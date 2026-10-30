package qrcode

import (
	"math/bits"
	"strings"
)

const maxVersion = 10

// blockGroup: n blocchi da data byte di dati.
type blockGroup struct{ n, data int }

// ecTable[versione-1][livello]: byte di correzione per blocco e gruppi di blocchi. Livelli: 0=L 1=M 2=Q 3=H (ISO 18004).
type ecInfo struct {
	ec     int
	groups []blockGroup
}

var ecTable = [maxVersion][4]ecInfo{
	{{7, []blockGroup{{1, 19}}}, {10, []blockGroup{{1, 16}}}, {13, []blockGroup{{1, 13}}}, {17, []blockGroup{{1, 9}}}},
	{{10, []blockGroup{{1, 34}}}, {16, []blockGroup{{1, 28}}}, {22, []blockGroup{{1, 22}}}, {28, []blockGroup{{1, 16}}}},
	{{15, []blockGroup{{1, 55}}}, {26, []blockGroup{{1, 44}}}, {18, []blockGroup{{2, 17}}}, {22, []blockGroup{{2, 13}}}},
	{{20, []blockGroup{{1, 80}}}, {18, []blockGroup{{2, 32}}}, {26, []blockGroup{{2, 24}}}, {16, []blockGroup{{4, 9}}}},
	{{26, []blockGroup{{1, 108}}}, {24, []blockGroup{{2, 43}}}, {18, []blockGroup{{2, 15}, {2, 16}}}, {22, []blockGroup{{2, 11}, {2, 12}}}},
	{{18, []blockGroup{{2, 68}}}, {16, []blockGroup{{4, 27}}}, {24, []blockGroup{{4, 19}}}, {28, []blockGroup{{4, 15}}}},
	{{20, []blockGroup{{2, 78}}}, {18, []blockGroup{{4, 31}}}, {18, []blockGroup{{2, 14}, {4, 15}}}, {26, []blockGroup{{4, 13}, {1, 14}}}},
	{{24, []blockGroup{{2, 97}}}, {22, []blockGroup{{2, 38}, {2, 39}}}, {22, []blockGroup{{4, 18}, {2, 19}}}, {26, []blockGroup{{4, 14}, {2, 15}}}},
	{{30, []blockGroup{{2, 116}}}, {22, []blockGroup{{3, 36}, {2, 37}}}, {20, []blockGroup{{4, 16}, {4, 17}}}, {24, []blockGroup{{4, 12}, {4, 13}}}},
	{{18, []blockGroup{{2, 68}, {2, 69}}}, {26, []blockGroup{{4, 43}, {1, 44}}}, {24, []blockGroup{{6, 19}, {2, 20}}}, {28, []blockGroup{{6, 15}, {2, 16}}}},
}

// alignment[versione-1]: coordinate dei centri dei motivi di allineamento.
var alignment = [maxVersion][]int{
	nil, {6, 18}, {6, 22}, {6, 26}, {6, 30}, {6, 34}, {6, 22, 38}, {6, 24, 42}, {6, 26, 46}, {6, 28, 50},
}

// Livelli di correzione: i bit del formato 01=L 00=M 11=Q 10=H.
var levelOfBits = [4]int{1, 0, 3, 2}

// formatCodes: i 32 codici di formato validi (BCH 15,5 con maschera 0x5412), indice = 5 bit dati.
var formatCodes = func() [32]int {
	var t [32]int
	for d := 0; d < 32; d++ {
		v := d << 10
		for i := 14; i >= 10; i-- {
			if v&(1<<uint(i)) != 0 {
				v ^= 0x537 << uint(i-10)
			}
		}
		t[d] = (d<<10 | v) ^ 0x5412
	}
	return t
}()

// readFormat legge il formato dalle due copie e lo corregge (fino a 3 bit sbagliati).
func readFormat(m [][]bool) (level, mask int, ok bool) {
	dim := len(m)
	bit := func(x, y int) int {
		if m[y][x] {
			return 1
		}
		return 0
	}
	a := 0
	for i := 0; i < 6; i++ {
		a = a<<1 | bit(i, 8)
	}
	a = a<<1 | bit(7, 8)
	a = a<<1 | bit(8, 8)
	a = a<<1 | bit(8, 7)
	for j := 5; j >= 0; j-- {
		a = a<<1 | bit(8, j)
	}
	b := 0
	for j := dim - 1; j >= dim-7; j-- {
		b = b<<1 | bit(8, j)
	}
	for i := dim - 8; i < dim; i++ {
		b = b<<1 | bit(i, 8)
	}
	best, bd := -1, 4
	for d, code := range formatCodes {
		for _, got := range []int{a, b} {
			if h := bits.OnesCount(uint(got ^ code)); h < bd {
				best, bd = d, h
			}
		}
	}
	if best < 0 {
		return 0, 0, false
	}
	return levelOfBits[best>>3], best & 7, true
}

func maskBit(mask, r, c int) bool {
	switch mask {
	case 0:
		return (r+c)%2 == 0
	case 1:
		return r%2 == 0
	case 2:
		return c%3 == 0
	case 3:
		return (r+c)%3 == 0
	case 4:
		return (r/2+c/3)%2 == 0
	case 5:
		return r*c%2+r*c%3 == 0
	case 6:
		return (r*c%2+r*c%3)%2 == 0
	}
	return ((r+c)%2+r*c%3)%2 == 0
}

// functionMap segna i moduli che non portano dati: quadrati di riferimento, formato, sincronismo, allineamento, versione.
func functionMap(ver int) [][]bool {
	dim := 17 + 4*ver
	f := make([][]bool, dim)
	for i := range f {
		f[i] = make([]bool, dim)
	}
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				if x >= 0 && y >= 0 && x < dim && y < dim {
					f[y][x] = true
				}
			}
		}
	}
	fill(0, 0, 8, 8)
	fill(dim-8, 0, dim-1, 8)
	fill(0, dim-8, 8, dim-1)
	fill(6, 0, 6, dim-1)
	fill(0, 6, dim-1, 6)
	al := alignment[ver-1]
	for _, cy := range al {
		for _, cx := range al {
			if (cx == 6 && cy == 6) || (cx == 6 && cy == al[len(al)-1]) || (cy == 6 && cx == al[len(al)-1]) {
				continue
			}
			fill(cx-2, cy-2, cx+2, cy+2)
		}
	}
	if ver >= 7 {
		fill(dim-11, 0, dim-9, 5)
		fill(0, dim-11, 5, dim-9)
	}
	return f
}

// decodeMatrix legge il testo da una griglia di moduli.
func decodeMatrix(m [][]bool) (string, error) {
	dim := len(m)
	ver := (dim - 17) / 4
	if ver < 1 || ver > maxVersion {
		return "", ErrUnsupported
	}
	level, mask, ok := readFormat(m)
	if !ok {
		return "", ErrNotFound
	}
	fn := functionMap(ver)
	// byte dei dati e della correzione, nell'ordine a zigzag
	var stream []byte
	cur, nb := byte(0), 0
	up := true
	for x := dim - 1; x > 0; x -= 2 {
		if x == 6 {
			x--
		}
		for i := 0; i < dim; i++ {
			y := i
			if up {
				y = dim - 1 - i
			}
			for dx := 0; dx < 2; dx++ {
				xx := x - dx
				if fn[y][xx] {
					continue
				}
				v := m[y][xx] != maskBit(mask, y, xx)
				cur <<= 1
				if v {
					cur |= 1
				}
				if nb++; nb == 8 {
					stream = append(stream, cur)
					cur, nb = 0, 0
				}
			}
		}
		up = !up
	}
	info := ecTable[ver-1][level]
	var blocks [][]byte // ciascuno: dati poi correzione
	var dlen []int
	totalData := 0
	for _, g := range info.groups {
		for k := 0; k < g.n; k++ {
			blocks = append(blocks, make([]byte, 0, g.data+info.ec))
			dlen = append(dlen, g.data)
			totalData += g.data
		}
	}
	if len(stream) < totalData+info.ec*len(blocks) {
		return "", ErrNotFound
	}
	pos := 0
	maxData := 0
	for _, d := range dlen {
		if d > maxData {
			maxData = d
		}
	}
	for i := 0; i < maxData; i++ { // dati intercalati
		for b := range blocks {
			if i < dlen[b] {
				blocks[b] = append(blocks[b], stream[pos])
				pos++
			}
		}
	}
	for i := 0; i < info.ec; i++ { // correzione intercalata
		for b := range blocks {
			blocks[b] = append(blocks[b], stream[pos])
			pos++
		}
	}
	var data []byte
	for b := range blocks {
		if !rsCorrect(blocks[b], info.ec) {
			return "", ErrNotFound
		}
		data = append(data, blocks[b][:dlen[b]]...)
	}
	return parseSegments(data, ver)
}

// parseSegments legge i segmenti (modo, lunghezza, dati) fino al terminatore.
func parseSegments(data []byte, ver int) (string, error) {
	r := &bitReader{b: data}
	var sb strings.Builder
	for {
		mode, ok := r.read(4)
		if !ok || mode == 0 {
			break
		}
		cnt := func(b9, b26 int) (int, bool) {
			if ver < 10 {
				return r.read(b9)
			}
			return r.read(b26)
		}
		switch mode {
		case 4: // byte
			n, ok := cnt(8, 16)
			if !ok {
				return "", ErrNotFound
			}
			for i := 0; i < n; i++ {
				v, ok := r.read(8)
				if !ok {
					return "", ErrNotFound
				}
				sb.WriteByte(byte(v))
			}
		case 2: // alfanumerico
			n, ok := cnt(9, 11)
			if !ok {
				return "", ErrNotFound
			}
			const set = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"
			for ; n > 1; n -= 2 {
				v, ok := r.read(11)
				if !ok || v/45 >= 45 {
					return "", ErrNotFound
				}
				sb.WriteByte(set[v/45])
				sb.WriteByte(set[v%45])
			}
			if n == 1 {
				v, ok := r.read(6)
				if !ok || v >= 45 {
					return "", ErrNotFound
				}
				sb.WriteByte(set[v])
			}
		case 1: // numerico
			n, ok := cnt(10, 12)
			if !ok {
				return "", ErrNotFound
			}
			for ; n >= 3; n -= 3 {
				v, ok := r.read(10)
				if !ok || v > 999 {
					return "", ErrNotFound
				}
				sb.WriteString(pad(v, 3))
			}
			if n == 2 {
				v, ok := r.read(7)
				if !ok || v > 99 {
					return "", ErrNotFound
				}
				sb.WriteString(pad(v, 2))
			} else if n == 1 {
				v, ok := r.read(4)
				if !ok || v > 9 {
					return "", ErrNotFound
				}
				sb.WriteString(pad(v, 1))
			}
		case 7: // ECI: si salta l'identificatore (un byte nei casi comuni)
			if _, ok := r.read(8); !ok {
				return "", ErrNotFound
			}
		default:
			return "", ErrUnsupported
		}
	}
	if sb.Len() == 0 {
		return "", ErrNotFound
	}
	return sb.String(), nil
}

func pad(v, n int) string {
	s := ""
	for i := 0; i < n; i++ {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}

type bitReader struct {
	b   []byte
	pos int
}

func (r *bitReader) read(n int) (int, bool) {
	if r.pos+n > len(r.b)*8 {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		v = v<<1 | int(r.b[r.pos>>3]>>(7-uint(r.pos&7))&1)
		r.pos++
	}
	return v, true
}
