// Package twain e' un client TWAIN minimo (DSM_Entry) per lo scanner fi-800R con driver PaperStream IP.
//
// Perche' TWAIN: con WIA il driver ignora l'inizio verticale dell'area di scansione. Il data source TWAIN
// espone ICAP_FRAMES (sinistra/alto/destra/basso) e capability proprie: qui si verifica se l'alto viene rispettato.
//
// Architettura: il codice e' indipendente dal numero di bit. Le strutture TW_* usano packing a 2 byte (come twain.h
// per Windows, 32 e 64 bit); i campi grandi come un puntatore (TW_HANDLE, TW_MEMREF) cambiano dimensione, quindi le
// strutture si costruiscono a mano in []byte con layout{ptr: 4|8} (testato per entrambe le architetture).
// Serve il DSM e il data source dello stesso numero di bit del processo: oggi e' installato solo il driver a 32 bit
// (C:\Windows\twain_32\Fjicube), quindi l'exe funzionante e' quello GOARCH=386; quello a 64 bit richiede
// 'PaperStream IP (TWAIN x64)' (C:\Windows\twain_64 e C:\Windows\System32\TWAINDSM.dll).
//
// Messaggi in italiano, come internal/wia. Nessuna acquisizione parte da Probe: il motore dello scanner si avvia
// solo con DAT_USERINTERFACE/MSG_ENABLEDS, che esiste in un unico punto (enableAndTransfer, usato da Scan).
package twain

import (
	"encoding/binary"
	"math"
)

// Gruppi, DAT e messaggi TWAIN (twain.h 2.x).
const (
	dgControl = 0x0001
	dgImage   = 0x0002
	dfApp2    = 0x20000000 // l'applicazione parla TWAIN 2.x (DSM 2.x)

	datCapability    = 0x0001
	datEvent         = 0x0002
	datIdentity      = 0x0003
	datParent        = 0x0004
	datPendingXfers  = 0x0005
	datStatus        = 0x0008
	datUserInterface = 0x0009
	datImageInfo     = 0x0101
	datImageLayout   = 0x0102
	datImageNative   = 0x0104

	msgGet          = 0x0001
	msgGetCurrent   = 0x0002
	msgGetDefault   = 0x0003
	msgGetFirst     = 0x0004
	msgGetNext      = 0x0005
	msgSet          = 0x0006
	msgReset        = 0x0007
	msgQuerySupport = 0x0008
	msgXferReady    = 0x0101
	msgCloseDSReq   = 0x0102
	msgCloseDSOK    = 0x0103
	msgDeviceEvent  = 0x0104
	msgOpenDSM      = 0x0301
	msgCloseDSM     = 0x0302
	msgOpenDS       = 0x0401
	msgCloseDS      = 0x0402
	msgDisableDS    = 0x0501
	msgEnableDS     = 0x0502
	msgProcessEvent = 0x0601
	msgEndXfer      = 0x0701
)

// Codici di ritorno (TWRC_*) e di condizione (TWCC_*).
const (
	rcSuccess         = 0
	rcFailure         = 1
	rcCheckStatus     = 2
	rcCancel          = 3
	rcDSEvent         = 4
	rcNotDSEvent      = 5
	rcXferDone        = 6
	rcEndOfList       = 7
	rcInfoNotSupport  = 8
	rcDataNotAvail    = 9
	rcBusy            = 10
	rcScannerLocked   = 11
	ccNoMedia         = 29
	ccPaperJam        = 20
	ccPaperDoubleFeed = 21
)

// Tipi di container e di elemento.
const (
	conArray       = 3
	conEnumeration = 4
	conOneValue    = 5
	conRange       = 6
	conDontCare    = 0xFFFF

	tyInt8   = 0
	tyInt16  = 1
	tyInt32  = 2
	tyUint8  = 3
	tyUint16 = 4
	tyUint32 = 5
	tyBool   = 6
	tyFix32  = 7
	tyFrame  = 8
	tyStr32  = 9
	tyStr64  = 10
	tyStr128 = 11
	tyStr255 = 12
	tyStr1k  = 13
)

// Capability usate (ids confermati dalle tabelle del driver, vedi capnames.go).
const (
	capXferCount      = 0x0001
	icapPixelType     = 0x0101
	icapUnits         = 0x0102
	icapXferMech      = 0x0103
	capFeederEnabled  = 0x1002
	capFeederLoaded   = 0x1003
	capSupportedCaps  = 0x1005
	capAutoFeed       = 0x1007
	capIndicators     = 0x100B
	capDuplexEnabled  = 0x1013
	icapPhysicalWidth = 0x1111
	icapPhysicalHeigh = 0x1112
	icapFrames        = 0x1114
	icapXResolution   = 0x1118
	icapYResolution   = 0x1119
	icapMaxFrames     = 0x111A
	icapSupportedSize = 0x1122
	icapUndefImgSize  = 0x112D
	icapAutoBorder    = 0x1150
	icapAutoDeskew    = 0x1151
	icapAutoRotate    = 0x1152
	// capability proprie PaperStream IP (non standard)
	icapExtFrameUnit    = 0xA01F
	icapExtFramesIndUn  = 0xA020
	icapExtFrameMode    = 0xA069
	icapExtTrimSize     = 0xA006
	icapExtTrimSizeUnit = 0xA007
	icapExtOverscan     = 0x80F0
	capExtPaperProtect  = 0x80FE
	capExtManualFeeder  = 0xA063
	capExtSyncEject     = 0x8103
	icapExtAdfPhysH     = 0xA010
	icapExtAdfPhysW     = 0xA011
	capExtFrame         = 0x80CE

	pixelGray = 1
	pixelRGB  = 2
	unitInch  = 0
	xferMechN = 0 // TWSX_NATIVE
)

// layout descrive le dimensioni delle strutture che contengono puntatori: ptr e' 4 (32 bit) o 8 (64 bit).
// Packing a 2 byte, come twain.h su Windows.
type layout struct{ ptr int }

// Dimensioni delle strutture (byte).
const (
	identitySize    = 156 // TW_IDENTITY: nessun puntatore, uguale a 32 e 64 bit
	statusSize      = 4   // TW_STATUS
	pendingSize     = 6   // TW_PENDINGXFERS
	imageInfoSize   = 42  // TW_IMAGEINFO
	imageLayoutSize = 28  // TW_IMAGELAYOUT: TW_FRAME(16) + 3 x u32
	frameSize       = 16  // TW_FRAME: 4 x TW_FIX32
)

func (l layout) capabilitySize() int { return 4 + l.ptr } // Cap u16, ConType u16, hContainer
func (l layout) uiSize() int         { return 4 + l.ptr } // ShowUI u16, ModalUI u16, hParent
func (l layout) eventSize() int      { return l.ptr + 2 } // pEvent, TWMessage u16

func putPtr(b []byte, off int, v uintptr, ptr int) {
	if ptr == 4 {
		binary.LittleEndian.PutUint32(b[off:], uint32(v))
		return
	}
	binary.LittleEndian.PutUint64(b[off:], uint64(v))
}

func getPtr(b []byte, off int, ptr int) uintptr {
	if ptr == 4 {
		return uintptr(binary.LittleEndian.Uint32(b[off:]))
	}
	return uintptr(binary.LittleEndian.Uint64(b[off:]))
}

func (l layout) encodeCapability(capID, con uint16, h uintptr) []byte {
	b := make([]byte, l.capabilitySize())
	binary.LittleEndian.PutUint16(b[0:], capID)
	binary.LittleEndian.PutUint16(b[2:], con)
	putPtr(b, 4, h, l.ptr)
	return b
}

func (l layout) decodeCapability(b []byte) (capID, con uint16, h uintptr) {
	return binary.LittleEndian.Uint16(b[0:]), binary.LittleEndian.Uint16(b[2:]), getPtr(b, 4, l.ptr)
}

func (l layout) encodeEvent(pEvent uintptr) []byte {
	b := make([]byte, l.eventSize())
	putPtr(b, 0, pEvent, l.ptr)
	return b
}

func (l layout) eventMessage(b []byte) uint16 { return binary.LittleEndian.Uint16(b[l.ptr:]) }

func (l layout) encodeUI(show, modal bool, hParent uintptr) []byte {
	b := make([]byte, l.uiSize())
	if show {
		binary.LittleEndian.PutUint16(b[0:], 1)
	}
	if modal {
		binary.LittleEndian.PutUint16(b[2:], 1)
	}
	putPtr(b, 4, hParent, l.ptr)
	return b
}

// newIdentity costruisce l'identita' dell'applicazione (TW_IDENTITY, 156 byte).
func newIdentity(manufacturer, family, product string) []byte {
	b := make([]byte, identitySize)
	le := binary.LittleEndian
	le.PutUint16(b[4:], 1) // Version.MajorNum
	le.PutUint16(b[6:], 0) // MinorNum
	le.PutUint16(b[8:], 8) // Language: TWLG_ITALIAN
	le.PutUint16(b[10:], 39)
	putStr(b[12:46], "1.0")
	le.PutUint16(b[46:], 2) // ProtocolMajor
	le.PutUint16(b[48:], 3) // ProtocolMinor
	le.PutUint32(b[50:], dgControl|dgImage|dfApp2)
	putStr(b[54:88], manufacturer)
	putStr(b[88:122], family)
	putStr(b[122:156], product)
	return b
}

func putStr(dst []byte, s string) {
	n := copy(dst[:len(dst)-1], s)
	for i := n; i < len(dst); i++ {
		dst[i] = 0
	}
}

func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func identityID(b []byte) uint32      { return binary.LittleEndian.Uint32(b[0:]) }
func identityProduct(b []byte) string { return cString(b[122:156]) }
func identityFamily(b []byte) string  { return cString(b[88:122]) }
func identityMaker(b []byte) string   { return cString(b[54:88]) }
func identityProtocol(b []byte) (int, int) {
	return int(binary.LittleEndian.Uint16(b[46:])), int(binary.LittleEndian.Uint16(b[48:]))
}

// fix32Encode converte un reale in TW_FIX32 (Whole int16, Frac uint16) impacchettato in un uint32 little endian.
func fix32Encode(f float64) uint32 {
	v := int32(math.Floor(f*65536 + 0.5))
	whole := int16(v >> 16)
	frac := uint16(v & 0xFFFF)
	return uint32(uint16(whole)) | uint32(frac)<<16
}

func fix32Decode(u uint32) float64 {
	return float64(int16(u&0xFFFF)) + float64(u>>16)/65536
}

// Frame e' un riquadro TWAIN nelle unita' di ICAP_UNITS (qui sempre pollici).
type Frame struct{ Left, Top, Right, Bottom float64 }

func (f Frame) encode() []byte {
	b := make([]byte, frameSize)
	binary.LittleEndian.PutUint32(b[0:], fix32Encode(f.Left))
	binary.LittleEndian.PutUint32(b[4:], fix32Encode(f.Top))
	binary.LittleEndian.PutUint32(b[8:], fix32Encode(f.Right))
	binary.LittleEndian.PutUint32(b[12:], fix32Encode(f.Bottom))
	return b
}

func decodeFrame(b []byte) Frame {
	le := binary.LittleEndian
	return Frame{fix32Decode(le.Uint32(b[0:])), fix32Decode(le.Uint32(b[4:])), fix32Decode(le.Uint32(b[8:])), fix32Decode(le.Uint32(b[12:]))}
}
