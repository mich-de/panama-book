//go:build vizlab

package libreta

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"scanlibretto/internal/imgproc"
)

// vizRowAutocorr: autocorrelazione normalizzata del profilo di colonne di una riga, lag da lo a hi.
func vizRowAutocorr(ink []bool, w, x0, x1, y0, y1, lo, hi int) (best int, corr []float64) {
	h := len(ink) / w
	n := x1 - x0
	prof := make([]float64, n)
	for y := max(0, y0); y < min(h, y1); y++ {
		for x := 0; x < n; x++ {
			if ink[y*w+x0+x] {
				prof[x]++
			}
		}
	}
	var mean float64
	for _, v := range prof {
		mean += v
	}
	mean /= float64(n)
	var den float64
	for _, v := range prof {
		den += (v - mean) * (v - mean)
	}
	corr = make([]float64, hi+2)
	if den == 0 {
		return 0, corr
	}
	for lag := lo; lag <= hi && lag < n/2; lag++ {
		var c float64
		for x := 0; x+lag < n; x++ {
			c += (prof[x] - mean) * (prof[x+lag] - mean)
		}
		corr[lag] = c / (den * float64(n-lag) / float64(n))
	}
	bm := -1.0
	for lag := lo; lag <= hi; lag++ {
		if corr[lag] > bm {
			bm, best = corr[lag], lag
		}
	}
	return best, corr
}

func TestLabRows(t *testing.T) {
	for _, li := range labList(t, "") {
		if !strings.Contains(","+os.Getenv("LAB_IMGS")+",", ","+li.Name+",") {
			continue
		}
		src, _ := imgproc.Load(li.Path)
		base := imgproc.CropContent(src, 10)
		base, _ = imgproc.Deskew(base)
		g := vizToGray(imgproc.Rotate(base, li.Rot))
		k := max(1, g.Bounds().Dx()/1200)
		ink, w, _ := vizInk(g, k)
		rows := vizBinRows(g)
		fmt.Printf("== %s (%dx%d) k=%d, %d righe imgproc\n", li.Name, g.Bounds().Dx(), g.Bounds().Dy(), k, len(rows))
		for i := max(0, len(rows)-9); i < len(rows); i++ {
			r := rows[i]
			hh := (r.Y1 - r.Y0) / k
			if r.X1-r.X0 < 300 || hh < 8 {
				fmt.Printf("  riga %d %+v (scartata)\n", i, r)
				continue
			}
			lo, hi := int(0.4*float64(hh)), int(1.3*float64(hh))
			best, corr := vizRowAutocorr(ink, w, r.X0/k, r.X1/k, r.Y0/k, r.Y1/k, lo, hi)
			// primo massimo locale con corr >= 0.8 del massimo
			first := 0
			for lag := lo + 1; lag < hi; lag++ {
				if corr[lag] >= 0.8*corr[best] && corr[lag] >= corr[lag-1] && corr[lag] >= corr[lag+1] {
					first = lag
					break
				}
			}
			fmt.Printf("  riga %d y %d-%d x %d-%d (alt %d): lag max %d (%.2f) primo %d (%.2f) -> larghezza %.0f\n", i, r.Y0, r.Y1, r.X0, r.X1, hh*k, best*k, corr[best], first*k, corr[first], 43.5*float64(first*k))
		}
	}
}

func TestLabCand(t *testing.T) {
	for _, li := range labList(t, "") {
		if !strings.Contains(","+os.Getenv("LAB_IMGS")+",", ","+li.Name+",") {
			continue
		}
		src, _ := imgproc.Load(li.Path)
		base := imgproc.CropContent(src, 10)
		base, _ = imgproc.Deskew(base)
		g := vizToGray(imgproc.Rotate(base, li.Rot))
		k := max(1, g.Bounds().Dx()/1200)
		ink, w, _ := vizInk(g, k)
		rows := vizCandidateRows(g, ink, w, k)
		fmt.Printf("== %s: %d candidate\n", li.Name, len(rows))
		for _, r := range rows {
			fmt.Printf("   y %d-%d (h %d) x %d-%d: passo %.1f forza %.2f larghezza/passo %.1f\n", r.y0, r.y1, r.y1-r.y0, r.x0, r.x1, r.pitch, r.strength, float64(r.x1-r.x0)/max(1, r.pitch))
		}
	}
}

func TestLabCorr(t *testing.T) {
	for _, li := range labList(t, "") {
		if li.Name != os.Getenv("LAB_IMG") {
			continue
		}
		src, _ := imgproc.Load(li.Path)
		base := imgproc.CropContent(src, 10)
		base, _ = imgproc.Deskew(base)
		g := vizToGray(imgproc.Rotate(base, li.Rot))
		k := max(1, g.Bounds().Dx()/1200)
		ink, w, _ := vizInk(g, k)
		for _, r := range vizBinRows(g) {
			hh := r.Y1 - r.Y0
			if r.X1-r.X0 < g.Bounds().Dx()/5 || hh < 8 || hh > g.Bounds().Dy()/8 {
				continue
			}
			_, corr := vizRowAutocorr(ink, w, r.X0/k, r.X1/k, r.Y0/k, r.Y1/k, 4, 140)
			var parts []string
			for lag := 8; lag <= 140; lag += 1 {
				if corr[lag] > 0.2 && corr[lag] >= corr[lag-1] && corr[lag] >= corr[lag+1] {
					parts = append(parts, fmt.Sprintf("%d:%.2f", lag*k, corr[lag]))
				}
			}
			fmt.Printf("riga y %d-%d: picchi %s\n", r.Y0, r.Y1, strings.Join(parts, " "))
		}
	}
}
