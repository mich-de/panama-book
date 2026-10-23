package twain

import (
	"encoding/binary"
	"strings"
	"testing"
)

func u16(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
func u32(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }
func cat(parts ...[]byte) []byte {
	var o []byte
	for _, p := range parts {
		o = append(o, p...)
	}
	return o
}

func TestParseOneValue(t *testing.T) {
	// BOOL vero in TW_ONEVALUE (tipo + Item da 4 byte)
	c, err := parseContainer(cat(u16(tyBool), u32(1)), conOneValue, 4)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := c.Current(); !ok || v != 1 {
		t.Errorf("BOOL: %v %v", v, ok)
	}
	// INT16 negativo
	c, _ = parseContainer(cat(u16(tyInt16), u32(0xFFFF)), conOneValue, 4)
	if v, _ := c.Current(); v != -1 {
		t.Errorf("INT16 -1 = %v", v)
	}
	// FIX32 300 dpi
	c, _ = parseContainer(cat(u16(tyFix32), u32(fix32Encode(300))), conOneValue, 8)
	if v, _ := c.Current(); v != 300 {
		t.Errorf("FIX32 = %v", v)
	}
	// FRAME
	f := Frame{0, 3.5, 8.5, 7.5}
	c, err = parseContainer(cat(u16(tyFrame), f.encode()), conOneValue, 4)
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := c.CurrentFrame(); !ok || !framesClose(f, g, 1e-4) {
		t.Errorf("frame = %+v %v", g, ok)
	}
	if _, err := parseContainer([]byte{1}, conOneValue, 4); err == nil {
		t.Error("container troppo corto accettato")
	}
	if _, err := parseContainer(cat(u16(99), u32(0)), conOneValue, 4); err == nil {
		t.Error("tipo sconosciuto accettato")
	}
}

func TestParseEnumerationArrayRange(t *testing.T) {
	// enumerazione UINT16: 75 150 300 600, corrente indice 2, predefinito 1
	en := cat(u16(tyUint16), u32(4), u32(2), u32(1), u16(75), u16(150), u16(300), u16(600))
	c, err := parseContainer(en, conEnumeration, 8)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Current(); v != 300 || !c.Allows(600) || c.Allows(200) {
		t.Errorf("enum: corrente %v, ammette 600=%v 200=%v", v, c.Allows(600), c.Allows(200))
	}
	if !strings.Contains(c.String(), "300*") {
		t.Errorf("String: %s", c)
	}
	// array UINT16 (elenco capability)
	ar := cat(u16(tyUint16), u32(3), u16(0x1002), u16(0x1114), u16(0x80FE))
	c, err = parseContainer(ar, conArray, 4)
	if err != nil || len(c.Items) != 3 || c.Items[2].U != 0x80FE {
		t.Errorf("array: %v %v", c, err)
	}
	// range FIX32 75..600 passo 1, predefinito 200, corrente 300
	rg := cat(u16(tyFix32), u32(fix32Encode(75)), u32(fix32Encode(600)), u32(fix32Encode(1)), u32(fix32Encode(200)), u32(fix32Encode(300)))
	c, err = parseContainer(rg, conRange, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Allows(300) || c.Allows(601) || c.Allows(74) {
		t.Error("range: Allows errato")
	}
	if v, _ := c.Current(); v != 300 {
		t.Errorf("range corrente = %v", v)
	}
	// elementi dichiarati piu' di quelli presenti: errore, non panico
	if _, err := parseContainer(cat(u16(tyUint16), u32(1000), u32(0), u32(0), u16(1)), conEnumeration, 4); err == nil {
		t.Error("enumerazione troncata accettata")
	}
	if _, err := parseContainer(cat(u16(tyUint16), u32(0xFFFFFFFF), u32(0), u32(0)), conEnumeration, 4); err == nil {
		t.Error("numero di elementi enorme accettato")
	}
}

func TestOneValueBytes(t *testing.T) {
	b := oneValueBytes(tyBool, 0, nil, 4)
	if len(b) != 6 || binary.LittleEndian.Uint16(b) != tyBool {
		t.Errorf("BOOL: %v", b)
	}
	b = oneValueBytes(tyUint16, 2, nil, 8)
	if len(b) != 6 || binary.LittleEndian.Uint16(b[2:]) != 2 {
		t.Errorf("UINT16: %v", b)
	}
	f := Frame{0, 3.543, 8.5, 7.48}
	b = oneValueBytes(tyFrame, 0, &f, 4)
	if len(b) != 2+frameSize {
		t.Fatalf("FRAME: %d byte", len(b))
	}
	// andata e ritorno
	c, err := parseContainer(b, conOneValue, 4)
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := c.CurrentFrame(); !framesClose(f, g, 1e-4) {
		t.Errorf("andata e ritorno: %+v", g)
	}
}

func TestItemSizes(t *testing.T) {
	for _, tc := range []struct {
		ty        uint16
		ptr, want int
	}{
		{tyBool, 4, 2}, {tyUint16, 4, 2}, {tyUint32, 4, 4}, {tyFix32, 8, 4}, {tyFrame, 8, 16}, {tyStr32, 4, 34}, {15, 4, 4}, {15, 8, 8}, {99, 4, 0},
	} {
		if got := itemSize(tc.ty, tc.ptr); got != tc.want {
			t.Errorf("itemSize(%d, %d) = %d, atteso %d", tc.ty, tc.ptr, got, tc.want)
		}
	}
}

func TestCapNamesTable(t *testing.T) {
	// id standard confermati dalle tabelle del driver
	for id, name := range map[uint16]string{
		0x1002: "CAP_FEEDERENABLED", 0x1013: "CAP_DUPLEXENABLED", 0x1114: "ICAP_FRAMES", 0x1118: "ICAP_XRESOLUTION",
		0x111A: "ICAP_MAXFRAMES", 0x80FE: "CAP_EXT_PAPERPROTECTION", 0xA020: "ICAP_EXT_FRAMES_INDUNIT", 0xA069: "ICAP_EXT_FRAMEMODE",
	} {
		if capNames[id] != name {
			t.Errorf("capNames[0x%04X] = %q, atteso %q", id, capNames[id], name)
		}
	}
	if capIDsUsed := []uint16{capXferCount, icapPixelType, icapUnits, icapXferMech, capFeederEnabled, capFeederLoaded, capSupportedCaps,
		capIndicators, capDuplexEnabled, icapPhysicalWidth, icapPhysicalHeigh, icapFrames, icapXResolution, icapYResolution,
		icapMaxFrames, icapSupportedSize, icapUndefImgSize, icapAutoBorder, icapAutoDeskew, icapAutoRotate}; len(capIDsUsed) == 0 {
		t.Fatal("vuoto")
	} else {
		for _, id := range capIDsUsed {
			if capNames[id] == "" {
				t.Errorf("capability 0x%04X usata ma assente nella tabella del driver", id)
			}
		}
	}
}
