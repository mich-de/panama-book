package twain

import (
	"encoding/binary"
	"math"
	"testing"
)

// Dimensioni e offset delle strutture TWAIN (pack 2) per 32 e 64 bit: le stesse tabelle valgono per l'exe
// GOARCH=386 e per quello amd64, anche se il test gira su una sola architettura.
func TestLayoutSizesBothArchitectures(t *testing.T) {
	for _, tc := range []struct {
		ptr                 int
		capSize, ui, ev     int
		capHandle, uiParent int
		evMsg               int
	}{
		{4, 8, 8, 6, 4, 4, 4},
		{8, 12, 12, 10, 4, 4, 8},
	} {
		l := layout{ptr: tc.ptr}
		if l.capabilitySize() != tc.capSize || l.uiSize() != tc.ui || l.eventSize() != tc.ev {
			t.Errorf("ptr=%d: dimensioni cap/ui/event = %d/%d/%d, attese %d/%d/%d",
				tc.ptr, l.capabilitySize(), l.uiSize(), l.eventSize(), tc.capSize, tc.ui, tc.ev)
		}
		// TW_CAPABILITY: Cap@0 ConType@2 hContainer@4
		b := l.encodeCapability(0x1114, conOneValue, 0x1234)
		if len(b) != tc.capSize || binary.LittleEndian.Uint16(b[0:]) != 0x1114 || binary.LittleEndian.Uint16(b[2:]) != conOneValue {
			t.Errorf("ptr=%d: TW_CAPABILITY mal formata: %v", tc.ptr, b)
		}
		id, con, h := l.decodeCapability(b)
		if id != 0x1114 || con != conOneValue || h != 0x1234 {
			t.Errorf("ptr=%d: decodeCapability = %x %d %x", tc.ptr, id, con, h)
		}
		// TW_EVENT: pEvent@0, TWMessage@ptr
		ev := l.encodeEvent(0xABCD)
		binary.LittleEndian.PutUint16(ev[tc.evMsg:], msgXferReady)
		if l.eventMessage(ev) != msgXferReady || getPtr(ev, 0, tc.ptr) != 0xABCD {
			t.Errorf("ptr=%d: TW_EVENT mal formato", tc.ptr)
		}
		// TW_USERINTERFACE: ShowUI@0 ModalUI@2 hParent@4
		ui := l.encodeUI(false, true, 0x55)
		if binary.LittleEndian.Uint16(ui[0:]) != 0 || binary.LittleEndian.Uint16(ui[2:]) != 1 || getPtr(ui, tc.uiParent, tc.ptr) != 0x55 {
			t.Errorf("ptr=%d: TW_USERINTERFACE mal formata: %v", tc.ptr, ui)
		}
	}
}

// Le strutture senza puntatori hanno la stessa dimensione a 32 e 64 bit.
func TestFixedStructSizes(t *testing.T) {
	if identitySize != 156 || statusSize != 4 || pendingSize != 6 || imageInfoSize != 42 || imageLayoutSize != 28 || frameSize != 16 {
		t.Errorf("dimensioni fisse errate")
	}
	id := newIdentity("scanlibretto", "libretto", "prova")
	if len(id) != identitySize {
		t.Fatalf("TW_IDENTITY = %d byte, attesi 156", len(id))
	}
	// offset: Version@4 (42 byte), ProtocolMajor@46, SupportedGroups@50, Manufacturer@54, ProductFamily@88, ProductName@122
	if identityProduct(id) != "prova" || identityFamily(id) != "libretto" || identityMaker(id) != "scanlibretto" {
		t.Errorf("stringhe di TW_IDENTITY: %q %q %q", identityMaker(id), identityFamily(id), identityProduct(id))
	}
	maj, min := identityProtocol(id)
	if maj != 2 || min != 3 {
		t.Errorf("protocollo %d.%d", maj, min)
	}
	if g := binary.LittleEndian.Uint32(id[50:]); g&dfApp2 == 0 || g&dgControl == 0 || g&dgImage == 0 {
		t.Errorf("gruppi supportati 0x%x", g)
	}
}

func TestPtrHelpers(t *testing.T) {
	for _, p := range []int{4, 8} {
		b := make([]byte, 8)
		putPtr(b, 0, 0x01020304, p)
		if getPtr(b, 0, p) != 0x01020304 {
			t.Errorf("ptr=%d: andata e ritorno", p)
		}
	}
}

func TestFix32(t *testing.T) {
	for _, v := range []float64{0, 1, 8.5, 14, 3.543307, 300, -1.25, 0.001} {
		got := fix32Decode(fix32Encode(v))
		if math.Abs(got-v) > 1.0/65536 {
			t.Errorf("fix32(%v) = %v", v, got)
		}
	}
	// 8,5 pollici: Whole=8, Frac=32768
	if u := fix32Encode(8.5); u&0xFFFF != 8 || u>>16 != 32768 {
		t.Errorf("fix32Encode(8.5) = %#x", u)
	}
	// 0,9999 -> arrotondamento al piu' vicino, senza overflow della parte frazionaria
	if got := fix32Decode(fix32Encode(0.999999)); math.Abs(got-1) > 1.0/65536 {
		t.Errorf("fix32(0.999999) = %v", got)
	}
}

func TestFrameEncodeDecode(t *testing.T) {
	f := Frame{Left: 0.25, Top: 3.543, Right: 8.25, Bottom: 7.48}
	b := f.encode()
	if len(b) != frameSize {
		t.Fatalf("TW_FRAME = %d byte", len(b))
	}
	g := decodeFrame(b)
	if !framesClose(f, g, 1.0/65536) {
		t.Errorf("riquadro %+v -> %+v", f, g)
	}
}
