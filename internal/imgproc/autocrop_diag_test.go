package imgproc

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Diagnostica opt-in (sola lettura): AUTOCROP_FILES="a.png;b.png" go test -run TestAutoCropFilesDiag -v
// Stampa solo misure (px), nessun dato personale.
func TestAutoCropFilesDiag(t *testing.T) {
	list := os.Getenv("AUTOCROP_FILES")
	if list == "" {
		t.Skip("AUTOCROP_FILES non impostata")
	}
	var files []string
	for _, pat := range strings.Split(list, ";") {
		m, _ := filepath.Glob(pat)
		files = append(files, m...)
	}
	for _, p := range files {
		if strings.Contains(p, "pagina") {
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Errorf("%s: %v", p, err)
			continue
		}
		out, info := AutoCropInfo(img)
		b := img.Bounds()
		t.Logf("%s: %dx%d blank=%v box=%v skew=%.1f out=%dx%d", p[strings.LastIndexAny(p, `\/`)+1:], b.Dx(), b.Dy(), info.Blank, info.Box, info.SkewDeg, out.Bounds().Dx(), out.Bounds().Dy())
	}
}
