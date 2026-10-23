//go:build windows

package twain

import (
	"strings"
	"testing"
	"unsafe"
)

func TestPtrSizeMatchesArchitecture(t *testing.T) {
	if ptrSize != int(unsafe.Sizeof(uintptr(0))) || (ptrSize != 4 && ptrSize != 8) {
		t.Fatalf("ptrSize = %d", ptrSize)
	}
	// MSG di Windows: 28 byte a 32 bit, 48 a 64 bit
	want := map[int]uintptr{4: 28, 8: 48}[ptrSize]
	if got := unsafe.Sizeof(winMSG{}); got != want {
		t.Errorf("sizeof(MSG) = %d, atteso %d", got, want)
	}
	// i campi grandi come un puntatore seguono l'architettura del processo
	l := layout{ptr: ptrSize}
	if l.capabilitySize() != 4+ptrSize || l.eventSize() != ptrSize+2 {
		t.Errorf("layout per l'architettura corrente: %d %d", l.capabilitySize(), l.eventSize())
	}
}

func TestDSMCandidatesAndDirs(t *testing.T) {
	c32 := dsmCandidates(`C:\Windows`, 4)
	if len(c32) != 3 || !strings.HasSuffix(c32[0], `SysWOW64\TWAINDSM.dll`) {
		t.Errorf("DSM a 32 bit: %v", c32)
	}
	c64 := dsmCandidates(`C:\Windows`, 8)
	if len(c64) != 1 || !strings.HasSuffix(c64[0], `System32\TWAINDSM.dll`) {
		t.Errorf("DSM a 64 bit: %v", c64)
	}
	if !strings.HasSuffix(dsDir(`C:\Windows`, 4), "twain_32") || !strings.HasSuffix(dsDir(`C:\Windows`, 8), "twain_64") {
		t.Error("cartelle dei data source errate")
	}
	if h := ArchitectureHint(); !strings.Contains(h, "bit") || !strings.Contains(h, "TWAINDSM.dll") {
		t.Errorf("ArchitectureHint = %q", h)
	}
}

func TestMissingDSMMessages(t *testing.T) {
	c := dsmCandidates(`C:\Windows`, 8)
	m := dsmMissingMessage(`C:\Windows`, 8, c, nil)
	for _, want := range []string{"64 bit", `System32\TWAINDSM.dll`, "twain_64", "PaperStream IP (TWAIN x64)", "twainprobe32.exe"} {
		if !strings.Contains(m, want) {
			t.Errorf("manca %q in: %s", want, m)
		}
	}
	m = dsmMissingMessage(`C:\Windows`, 4, dsmCandidates(`C:\Windows`, 4), nil)
	if !strings.Contains(m, "32 bit") || !strings.Contains(m, "SysWOW64") || strings.Contains(m, "x64") {
		t.Errorf("messaggio a 32 bit: %s", m)
	}
	n := noDSMessage(`C:\Windows`, 8)
	if !strings.Contains(n, "twain_64") || !strings.Contains(n, "32 bit") {
		t.Errorf("noDSMessage: %s", n)
	}
	if strings.Contains(noDSMessage(`C:\Windows`, 4), "x64") || !strings.Contains(noDSMessage(`C:\Windows`, 4), "twain_32") {
		t.Errorf("noDSMessage a 32 bit: %s", noDSMessage(`C:\Windows`, 4))
	}
}
