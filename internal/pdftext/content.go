package pdftext

import (
	"strconv"
	"strings"
)

// Lettura del flusso di contenuto di una pagina: solo cio' che serve per sapere QUALE testo sta DOVE (operatori di testo e
// di trasformazione); disegni, immagini e colori si saltano.

type matrix [6]float64 // a b c d e f

var ident = matrix{1, 0, 0, 1, 0, 0}

// mul restituisce m x n (prima m, poi n), come nella specifica PDF.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4], m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

type tok struct {
	kind byte // 'n' numero, 'N' nome, 's' stringa, 'a' array, 'o' operatore
	num  float64
	str  string
	arr  []tok
}

// tokenizer legge i token di un flusso di contenuto.
type tokenizer struct {
	b []byte
	i int
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == 0 || c == '\f'
}
func isDelim(c byte) bool { return strings.IndexByte("()<>[]{}/%", c) >= 0 }

func (t *tokenizer) next() (tok, bool) {
	for t.i < len(t.b) {
		c := t.b[t.i]
		switch {
		case isSpace(c):
			t.i++
		case c == '%':
			for t.i < len(t.b) && t.b[t.i] != '\n' && t.b[t.i] != '\r' {
				t.i++
			}
		case c == '(':
			return tok{kind: 's', str: t.litString()}, true
		case c == '<' && t.i+1 < len(t.b) && t.b[t.i+1] == '<':
			t.i += 2 // dizionario in linea: si salta (lo chiude «>>»)
			depth := 1
			for t.i+1 < len(t.b) && depth > 0 {
				switch {
				case t.b[t.i] == '<' && t.b[t.i+1] == '<':
					depth++
					t.i += 2
				case t.b[t.i] == '>' && t.b[t.i+1] == '>':
					depth--
					t.i += 2
				default:
					t.i++
				}
			}
		case c == '<':
			return tok{kind: 's', str: t.hexString()}, true
		case c == '[':
			t.i++
			var arr []tok
			for {
				x, ok := t.next()
				if !ok || (x.kind == 'o' && x.str == "]") {
					break
				}
				arr = append(arr, x)
			}
			return tok{kind: 'a', arr: arr}, true
		case c == ']':
			t.i++
			return tok{kind: 'o', str: "]"}, true
		case c == '/':
			j := t.i + 1
			for j < len(t.b) && !isSpace(t.b[j]) && !isDelim(t.b[j]) {
				j++
			}
			s := string(t.b[t.i+1 : j])
			t.i = j
			return tok{kind: 'N', str: s}, true
		case c == '>' || c == ')' || c == '{' || c == '}':
			t.i++
		default:
			j := t.i
			for j < len(t.b) && !isSpace(t.b[j]) && !isDelim(t.b[j]) {
				j++
			}
			w := string(t.b[t.i:j])
			t.i = j
			if f, err := strconv.ParseFloat(w, 64); err == nil {
				return tok{kind: 'n', num: f}, true
			}
			return tok{kind: 'o', str: w}, true
		}
	}
	return tok{}, false
}

// litString legge «(...)» con parentesi annidate e sequenze di escape.
func (t *tokenizer) litString() string {
	var sb strings.Builder
	depth := 0
	for t.i < len(t.b) {
		c := t.b[t.i]
		t.i++
		switch c {
		case '(':
			depth++
			if depth > 1 {
				sb.WriteByte(c)
			}
		case ')':
			depth--
			if depth == 0 {
				return sb.String()
			}
			sb.WriteByte(c)
		case '\\':
			if t.i >= len(t.b) {
				break
			}
			e := t.b[t.i]
			t.i++
			switch {
			case e == 'n':
				sb.WriteByte('\n')
			case e == 'r':
				sb.WriteByte('\r')
			case e == 't':
				sb.WriteByte('\t')
			case e == 'b':
				sb.WriteByte('\b')
			case e == 'f':
				sb.WriteByte('\f')
			case e >= '0' && e <= '7':
				v := int(e - '0')
				for k := 0; k < 2 && t.i < len(t.b) && t.b[t.i] >= '0' && t.b[t.i] <= '7'; k++ {
					v = v*8 + int(t.b[t.i]-'0')
					t.i++
				}
				sb.WriteByte(byte(v))
			case e == '\n' || e == '\r':
				if e == '\r' && t.i < len(t.b) && t.b[t.i] == '\n' {
					t.i++
				}
			default:
				sb.WriteByte(e)
			}
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// hexString legge «<48656C6C6F>».
func (t *tokenizer) hexString() string {
	t.i++
	var sb strings.Builder
	hi, have := 0, false
	for t.i < len(t.b) {
		c := t.b[t.i]
		t.i++
		if c == '>' {
			break
		}
		var v int
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'a' && c <= 'f':
			v = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = int(c-'A') + 10
		default:
			continue
		}
		if have {
			sb.WriteByte(byte(hi<<4 | v))
			have = false
		} else {
			hi, have = v, true
		}
	}
	if have {
		sb.WriteByte(byte(hi << 4))
	}
	return sb.String()
}

// cp1252 mappa i byte 0x80-0x9F di WinAnsi (il resto coincide con Latin-1).
var cp1252 = [32]rune{0x20AC, 0x81, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x8D, 0x017D, 0x8F,
	0x90, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x9D, 0x017E, 0x0178}

func winAnsi(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 0x80 && c <= 0x9F:
			sb.WriteRune(cp1252[c-0x80])
		case c < 0x20 && c != '\t':
			sb.WriteByte(' ')
		default:
			sb.WriteRune(rune(c))
		}
	}
	return sb.String()
}

// runContent esegue gli operatori di testo e raccoglie i frammenti con la loro posizione sulla pagina.
func runContent(b []byte) []item {
	tz := &tokenizer{b: b}
	var (
		out        []item
		ops        []tok
		ctm        = ident
		stack      []matrix
		tm, tlm    = ident, ident
		size, lead float64
	)
	show := func(s string) {
		if len(out) >= maxItems {
			return
		}
		m := tm.mul(ctm)
		sz := size * absf(m[3])
		if m[3] == 0 {
			sz = size * absf(m[1])
		}
		y := m[5]
		if m[3] < 0 { // pagina capovolta: la quota cresce verso il basso
			y = -y
		}
		text := winAnsi(s)
		out = append(out, item{x: m[4], y: y, size: sz, text: text})
		tm = matrix{1, 0, 0, 1, 0.5 * size * float64(len([]rune(text))), 0}.mul(tm) // avanzamento approssimato: serve solo a non sovrapporre
	}
	nl := func(tx, ty float64) { tlm = matrix{1, 0, 0, 1, tx, ty}.mul(tlm); tm = tlm }
	num := func(i int) float64 {
		if i < len(ops) && ops[i].kind == 'n' {
			return ops[i].num
		}
		return 0
	}
	for {
		t, ok := tz.next()
		if !ok {
			break
		}
		if t.kind != 'o' {
			ops = append(ops, t)
			if len(ops) > 64 {
				ops = ops[len(ops)-64:]
			}
			continue
		}
		n := len(ops)
		switch t.str {
		case "q":
			if len(stack) < 64 {
				stack = append(stack, ctm)
			}
		case "Q":
			if k := len(stack); k > 0 {
				ctm, stack = stack[k-1], stack[:k-1]
			}
		case "cm":
			if n >= 6 {
				ctm = matrix{num(n - 6), num(n - 5), num(n - 4), num(n - 3), num(n - 2), num(n - 1)}.mul(ctm)
			}
		case "BT":
			tm, tlm = ident, ident
		case "Tf":
			if n >= 2 {
				size = num(n - 1)
			}
		case "TL":
			if n >= 1 {
				lead = num(n - 1)
			}
		case "Td":
			if n >= 2 {
				nl(num(n-2), num(n-1))
			}
		case "TD":
			if n >= 2 {
				lead = -num(n - 1)
				nl(num(n-2), num(n-1))
			}
		case "T*":
			nl(0, -lead)
		case "Tm":
			if n >= 6 {
				tm = matrix{num(n - 6), num(n - 5), num(n - 4), num(n - 3), num(n - 2), num(n - 1)}
				tlm = tm
			}
		case "Tj":
			if n >= 1 && ops[n-1].kind == 's' {
				show(ops[n-1].str)
			}
		case "'":
			nl(0, -lead)
			if n >= 1 && ops[n-1].kind == 's' {
				show(ops[n-1].str)
			}
		case `"`:
			nl(0, -lead)
			if n >= 1 && ops[n-1].kind == 's' {
				show(ops[n-1].str)
			}
		case "TJ":
			if n >= 1 && ops[n-1].kind == 'a' {
				var sb strings.Builder
				for _, e := range ops[n-1].arr {
					switch {
					case e.kind == 's':
						sb.WriteString(e.str)
					case e.kind == 'n' && e.num < -250: // spazio largo tra parole
						sb.WriteByte(' ')
					}
				}
				show(sb.String())
			}
		case "BI": // immagine in linea: si salta fino a EI
			for {
				x, ok := tz.next()
				if !ok || (x.kind == 'o' && x.str == "EI") {
					break
				}
			}
		}
		ops = ops[:0]
	}
	return out
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
