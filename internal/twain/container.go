package twain

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// Item e' un elemento di un container TWAIN. Il campo usato dipende dal tipo del container.
type Item struct {
	U uint32 // interi, booleani, FIX32 grezzo (vedi Container.Float)
	S string // stringhe
	F Frame  // TW_FRAME
}

// Container e' il contenuto decodificato di una capability (OneValue, Enumeration, Array o Range).
type Container struct {
	Con   uint16
	Type  uint16
	Items []Item
	Cur   int // Enumeration: indice corrente
	Def   int // Enumeration: indice predefinito
	// Range (valori grezzi, da interpretare secondo Type)
	Min, Max, Step, RDef, RCur uint32
}

// itemSize restituisce la dimensione in byte di un elemento del tipo dato (0 se il tipo non e' supportato).
func itemSize(ty uint16, ptr int) int {
	switch ty {
	case tyInt8, tyUint8:
		return 1
	case tyInt16, tyUint16, tyBool:
		return 2
	case tyInt32, tyUint32, tyFix32:
		return 4
	case tyFrame:
		return frameSize
	case tyStr32:
		return 34
	case tyStr64:
		return 66
	case tyStr128:
		return 130
	case tyStr255:
		return 256
	case tyStr1k:
		return 1026
	case 15: // TWTY_HANDLE
		return ptr
	}
	return 0
}

func decodeItem(ty uint16, b []byte) Item {
	le := binary.LittleEndian
	switch ty {
	case tyInt8:
		return Item{U: uint32(int32(int8(b[0])))}
	case tyUint8:
		return Item{U: uint32(b[0])}
	case tyInt16:
		return Item{U: uint32(int32(int16(le.Uint16(b))))}
	case tyUint16, tyBool:
		return Item{U: uint32(le.Uint16(b))}
	case tyInt32, tyUint32, tyFix32:
		return Item{U: le.Uint32(b)}
	case tyFrame:
		return Item{F: decodeFrame(b)}
	case tyStr32, tyStr64, tyStr128, tyStr255, tyStr1k:
		return Item{S: cString(b)}
	}
	return Item{}
}

// parseContainer decodifica il contenuto di un container di tipo con. b e' la memoria del container.
func parseContainer(b []byte, con uint16, ptr int) (Container, error) {
	c := Container{Con: con}
	le := binary.LittleEndian
	need := func(n int) error {
		if len(b) < n {
			return fmt.Errorf("container TWAIN troppo corto (%d byte, ne servono %d)", len(b), n)
		}
		return nil
	}
	if err := need(2); err != nil {
		return c, err
	}
	c.Type = le.Uint16(b)
	switch con {
	case conOneValue:
		sz := itemSize(c.Type, ptr)
		if sz == 0 {
			return c, fmt.Errorf("tipo TWAIN %d non supportato", c.Type)
		}
		if err := need(2 + sz); err != nil {
			return c, err
		}
		c.Items = []Item{decodeItem(c.Type, b[2:])}
	case conEnumeration, conArray:
		head := 6
		if con == conEnumeration {
			head = 14
		}
		if err := need(head); err != nil {
			return c, err
		}
		n := int(le.Uint32(b[2:]))
		sz := itemSize(c.Type, ptr)
		if sz == 0 {
			return c, fmt.Errorf("tipo TWAIN %d non supportato", c.Type)
		}
		if n < 0 || n > 100000 || head+n*sz > len(b) {
			return c, fmt.Errorf("container TWAIN con %d elementi non valido", n)
		}
		if con == conEnumeration {
			c.Cur, c.Def = int(le.Uint32(b[6:])), int(le.Uint32(b[10:]))
		}
		for i := 0; i < n; i++ {
			c.Items = append(c.Items, decodeItem(c.Type, b[head+i*sz:]))
		}
	case conRange:
		if err := need(22); err != nil {
			return c, err
		}
		c.Min, c.Max, c.Step = le.Uint32(b[2:]), le.Uint32(b[6:]), le.Uint32(b[10:])
		c.RDef, c.RCur = le.Uint32(b[14:]), le.Uint32(b[18:])
	default:
		return c, fmt.Errorf("tipo di container %d non supportato", con)
	}
	return c, nil
}

// oneValueBytes costruisce un container OneValue: tipo (u16) + elemento. item e' il valore intero grezzo
// (per FIX32 usare fix32Encode, per i booleani 0/1) oppure, per tyFrame, il riquadro.
func oneValueBytes(ty uint16, item uint32, frame *Frame, ptr int) []byte {
	sz := itemSize(ty, ptr)
	total := 2 + sz
	if total < 6 { // TW_ONEVALUE ha sempre un Item da 4 byte
		total = 6
	}
	b := make([]byte, total)
	binary.LittleEndian.PutUint16(b, ty)
	if ty == tyFrame && frame != nil {
		copy(b[2:], frame.encode())
		return b
	}
	switch sz {
	case 1:
		b[2] = byte(item)
	case 2:
		binary.LittleEndian.PutUint16(b[2:], uint16(item))
	default:
		binary.LittleEndian.PutUint32(b[2:], item)
	}
	return b
}

// raw converte un valore grezzo secondo il tipo del container in float64 (FIX32 -> reale, interi con segno).
func (c Container) float(u uint32) float64 {
	switch c.Type {
	case tyFix32:
		return fix32Decode(u)
	case tyInt8:
		return float64(int8(u))
	case tyInt16:
		return float64(int16(u))
	case tyInt32:
		return float64(int32(u))
	}
	return float64(u)
}

// Current restituisce il valore corrente come reale (OneValue: l'unico; Enumeration: l'elemento corrente;
// Range: il valore corrente). ok=false se il container non ha un valore numerico.
func (c Container) Current() (v float64, ok bool) {
	switch c.Con {
	case conOneValue:
		if len(c.Items) == 1 && c.Type != tyFrame && !isStr(c.Type) {
			return c.float(c.Items[0].U), true
		}
	case conEnumeration:
		if c.Cur >= 0 && c.Cur < len(c.Items) && c.Type != tyFrame && !isStr(c.Type) {
			return c.float(c.Items[c.Cur].U), true
		}
	case conRange:
		return c.float(c.RCur), true
	}
	return 0, false
}

// CurrentFrame restituisce il riquadro corrente per container di tipo TW_FRAME.
func (c Container) CurrentFrame() (Frame, bool) {
	if c.Type != tyFrame || len(c.Items) == 0 {
		return Frame{}, false
	}
	if c.Con == conEnumeration && c.Cur >= 0 && c.Cur < len(c.Items) {
		return c.Items[c.Cur].F, true
	}
	return c.Items[0].F, true
}

// Allows dice se il valore v e' ammesso dal container (Range: dentro min..max; Enumeration/Array: tra gli
// elementi; OneValue: uguale). Per tipi non numerici risponde true (non si puo' escludere).
func (c Container) Allows(v float64) bool {
	if c.Type == tyFrame || isStr(c.Type) {
		return true
	}
	const eps = 1e-6
	switch c.Con {
	case conRange:
		return v >= c.float(c.Min)-eps && v <= c.float(c.Max)+eps
	case conOneValue, conEnumeration, conArray:
		for _, it := range c.Items {
			if math.Abs(c.float(it.U)-v) < eps {
				return true
			}
		}
		return false
	}
	return true
}

func isStr(ty uint16) bool { return ty >= tyStr32 && ty <= tyStr1k }

var conNames = map[uint16]string{conOneValue: "OneValue", conEnumeration: "Enumeration", conArray: "Array", conRange: "Range"}
var tyNames = map[uint16]string{tyInt8: "INT8", tyInt16: "INT16", tyInt32: "INT32", tyUint8: "UINT8", tyUint16: "UINT16",
	tyUint32: "UINT32", tyBool: "BOOL", tyFix32: "FIX32", tyFrame: "FRAME", tyStr32: "STR32", tyStr64: "STR64",
	tyStr128: "STR128", tyStr255: "STR255", tyStr1k: "STR1024"}

func (c Container) fmtItem(it Item) string {
	switch {
	case c.Type == tyFrame:
		return fmt.Sprintf("[sx %.3f, alto %.3f, dx %.3f, basso %.3f]", it.F.Left, it.F.Top, it.F.Right, it.F.Bottom)
	case isStr(c.Type):
		return fmt.Sprintf("%q", it.S)
	case c.Type == tyFix32:
		return fmt.Sprintf("%.4g", fix32Decode(it.U))
	case c.Type == tyBool:
		if it.U != 0 {
			return "vero"
		}
		return "falso"
	case c.Type == tyInt8 || c.Type == tyInt16 || c.Type == tyInt32:
		return fmt.Sprintf("%g", c.float(it.U))
	}
	return fmt.Sprintf("%d", it.U)
}

// String descrive il container in una riga leggibile.
func (c Container) String() string {
	ty := tyNames[c.Type]
	if ty == "" {
		ty = fmt.Sprintf("tipo%d", c.Type)
	}
	switch c.Con {
	case conRange:
		return fmt.Sprintf("Range %s min=%s max=%s passo=%s predefinito=%s corrente=%s", ty,
			c.fmtItem(Item{U: c.Min}), c.fmtItem(Item{U: c.Max}), c.fmtItem(Item{U: c.Step}),
			c.fmtItem(Item{U: c.RDef}), c.fmtItem(Item{U: c.RCur}))
	case conOneValue:
		if len(c.Items) == 1 {
			return fmt.Sprintf("OneValue %s = %s", ty, c.fmtItem(c.Items[0]))
		}
	case conEnumeration, conArray:
		var p []string
		for i, it := range c.Items {
			s := c.fmtItem(it)
			if c.Con == conEnumeration && i == c.Cur {
				s += "*" // corrente
			}
			if c.Con == conEnumeration && i == c.Def {
				s += "(pred)"
			}
			p = append(p, s)
		}
		if len(p) > 24 {
			p = append(p[:24], fmt.Sprintf("... (%d elementi)", len(c.Items)))
		}
		return fmt.Sprintf("%s %s [%s]", conNames[c.Con], ty, strings.Join(p, " "))
	}
	return fmt.Sprintf("container %d tipo %s", c.Con, ty)
}
