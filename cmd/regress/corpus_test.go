package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// crea file vuoti (con una riga) sotto root; i percorsi usano '/'.
func creaFile(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, r := range rels {
		p := filepath.Join(root, filepath.FromSlash(r))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscoverCorpus(t *testing.T) {
	root := t.TempDir()
	creaFile(t, root,
		"scans/a.png", "scans/B.PNG", "scans/a-pagina.png", "scans/a-pagina2.png", "scans/c.jpg", "scans/c.json", "scans/c.err",
		"scans/dbg/x.png", // cartella di debug: non si visita
		"scans/vecchi test WIA/d.png", "scans/vecchi test WIA/d-pagina.png", "scans/vecchi test WIA/d-pagina2.png",
		"scans/vecchi test WIA/d-grezza.png", "scans/vecchi test WIA/sub/z.png",
		"facsimili/panama/e.jpg", "facsimili/panama/f.png", "facsimili/panama/g.md", "facsimili/panama/fonti/h.jpg",
		"altro/i.png")
	got, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"facsimili/panama/e.jpg", "facsimili/panama/f.png",
		"scans/B.PNG", "scans/a.png",
		"scans/vecchi test WIA/d-grezza.png", "scans/vecchi test WIA/d.png",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("corpus:\n got  %v\n want %v", got, want)
	}
}

func TestDiscoverCartelleAssenti(t *testing.T) {
	got, err := discover(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Errorf("radice vuota: %v, %v", got, err)
	}
}

func TestFilterFiles(t *testing.T) {
	files := []string{"scans/scana.png", "scans/vero8.png", "scans/vecchi test WIA/vero5.png", "facsimili/panama/x.jpg"}
	cases := []struct {
		only, skip string
		want       []string
	}{
		{"", "", files},
		{"vero", "", []string{"scans/vero8.png", "scans/vecchi test WIA/vero5.png"}},
		{"VERO", "vecchi", []string{"scans/vero8.png"}},
		{"scana, x.jpg", "", []string{"scans/scana.png", "facsimili/panama/x.jpg"}},
		{"", "scans/,facsimili", nil},
	}
	for _, c := range cases {
		if got := filterFiles(files, c.only, c.skip); !reflect.DeepEqual(got, c.want) {
			t.Errorf("only=%q skip=%q: %v, atteso %v", c.only, c.skip, got, c.want)
		}
	}
}

func TestRelToRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "scans", "a.png")
	if got := relToRoot(root, inside); got != "scans/a.png" {
		t.Errorf("assoluto dentro la radice: %q", got)
	}
	if got := relToRoot(root, `scans\a.png`); got != "scans/a.png" {
		t.Errorf("relativo con backslash: %q", got)
	}
	outside := filepath.Join(filepath.Dir(root), "fuori.png")
	if got := relToRoot(root, outside); got != filepath.ToSlash(outside) {
		t.Errorf("assoluto fuori dalla radice: %q", got)
	}
	if p := absPath(root, "scans/a.png"); p != inside {
		t.Errorf("absPath: %q", p)
	}
	if p := absPath(root, filepath.ToSlash(outside)); filepath.Clean(p) != filepath.Clean(outside) {
		t.Errorf("absPath di un assoluto: %q", p)
	}
}
