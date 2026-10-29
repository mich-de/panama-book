package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

// writeRaw scrive una scansione GREZZA sintetica (nessun dato reale): pagina chiara con un "documento" crema con strisce e un riquadro
// scuro, posizionato secondo FAKE_RAW: centered (predefinito), right/left (decentrato, tocca il bordo), short (alto solo 50 mm),
// tilt (inclinato di 4 gradi), blank (nessun documento). dpi e lunghezza letta vengono dagli argomenti dello scan.
func writeRaw(path string, args []string) {
	dpi := 300.0
	if v, err := strconv.ParseFloat(arg(args, "-dpi"), 64); err == nil && v > 0 {
		dpi = v
	}
	lenMM := 190.0
	if v, err := strconv.ParseFloat(arg(args, "-len"), 64); err == nil && v > 0 {
		lenMM = v
	}
	ppm := dpi / 25.4
	W, H := int(216*ppm), int(lenMM*ppm)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	bg := color.RGBA{R: 247, G: 247, B: 247, A: 255}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	mode := os.Getenv("FAKE_RAW")
	if mode == "blank" {
		save(path, img)
		return
	}
	docW, docH := 125*ppm, math.Min(176, lenMM-6)*ppm
	cx, cy, ang := float64(W)/2, float64(H)/2, 0.0
	switch mode {
	case "right": // il centro e' a 40 mm dal bordo destro della grezza: il documento esce dal campo
		cx = float64(W) - 40*ppm
	case "left":
		cx = 40 * ppm
	case "short":
		docH = 50 * ppm
		cy = docH/2 + 4*ppm
	case "tilt":
		ang = 4
	}
	sin, cos := math.Sincos(ang * math.Pi / 180)
	reach := int(math.Hypot(docW, docH)/2) + 2
	for y := int(cy) - reach; y <= int(cy)+reach; y++ {
		for x := int(cx) - reach; x <= int(cx)+reach; x++ {
			if x < 0 || y < 0 || x >= W || y >= H {
				continue
			}
			// coordinate nel documento (rotazione inversa)
			dx, dy := float64(x)-cx, float64(y)-cy
			u, v := dx*cos+dy*sin, -dx*sin+dy*cos
			if math.Abs(u) > docW/2 || math.Abs(v) > docH/2 {
				continue
			}
			c := color.RGBA{R: 232, G: 222, B: 186, A: 255}
			row := int(v + docH/2)
			if row%48 < 8 && math.Abs(u) < docW*0.4 { // strisce di "testo"
				c = color.RGBA{R: 40, G: 40, B: 60, A: 255}
			}
			if u > -docW/2+30*ppm && u < -docW/2+60*ppm && v > -docH/2+20*ppm && v < -docH/2+60*ppm { // "foto"
				c = color.RGBA{R: 90, G: 100, B: 120, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	save(path, img)
}

func save(path string, img image.Image) {
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}
