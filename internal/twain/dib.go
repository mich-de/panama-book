package twain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
)

// decodeDIB decodifica una DIB (BITMAPINFOHEADER + tavolozza + pixel, senza BITMAPFILEHEADER) non compressa a
// 1, 8, 24 o 32 bit, come restituita dal trasferimento nativo TWAIN. Le DIB a righe dal basso (altezza > 0) sono
// capovolte; quelle con altezza negativa sono dall'alto.
// (Stessa logica di wia.decodeBMP, ma senza intestazione di file: duplicazione nota, vedi documentazione.)
func decodeDIB(data []byte) (image.Image, error) {
	le := binary.LittleEndian
	if len(data) < 40 {
		return nil, errors.New("dib: dati troppo corti")
	}
	hs := int(le.Uint32(data[0:]))
	if hs < 40 || hs > len(data) {
		return nil, fmt.Errorf("dib: intestazione da %d byte non supportata", hs)
	}
	w := int(int32(le.Uint32(data[4:])))
	h := int(int32(le.Uint32(data[8:])))
	bpp := int(le.Uint16(data[14:]))
	comp := le.Uint32(data[16:])
	used := int(le.Uint32(data[32:]))
	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 1<<15 || h > 1<<16 {
		return nil, fmt.Errorf("dib: dimensioni non valide %dx%d", w, h)
	}
	if comp != 0 {
		return nil, fmt.Errorf("dib: compressione %d non supportata", comp)
	}
	if bpp != 1 && bpp != 8 && bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("dib: %d bit per pixel non supportati", bpp)
	}
	palN := 0
	if bpp <= 8 {
		palN = used
		if palN == 0 {
			palN = 1 << bpp
		}
	}
	palOff := hs
	pixOff := hs + 4*palN
	stride := ((w*bpp + 31) / 32) * 4
	if pixOff+stride*h > len(data) {
		return nil, fmt.Errorf("dib: pixel incompleti (servono %d byte, ce ne sono %d)", pixOff+stride*h-len(data), len(data)-pixOff)
	}
	pal := make([]color.RGBA, palN)
	gray := bpp <= 8
	for i := range pal {
		o := palOff + 4*i
		pal[i] = color.RGBA{data[o+2], data[o+1], data[o], 255}
		if pal[i].R != pal[i].G || pal[i].G != pal[i].B {
			gray = false
		}
	}
	row := func(y int) []byte {
		sy := y
		if !topDown {
			sy = h - 1 - y
		}
		return data[pixOff+sy*stride : pixOff+sy*stride+stride]
	}
	if gray {
		img := image.NewGray(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			r := row(y)
			for x := 0; x < w; x++ {
				var idx int
				if bpp == 8 {
					idx = int(r[x])
				} else {
					idx = int(r[x/8]>>(7-uint(x%8))) & 1
				}
				if idx >= len(pal) {
					idx = len(pal) - 1
				}
				img.Pix[y*img.Stride+x] = pal[idx].R
			}
		}
		return img, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		r := row(y)
		for x := 0; x < w; x++ {
			var c color.RGBA
			switch bpp {
			case 24:
				c = color.RGBA{r[3*x+2], r[3*x+1], r[3*x], 255}
			case 32:
				c = color.RGBA{r[4*x+2], r[4*x+1], r[4*x], 255}
			default: // 8 o 1 bit con tavolozza a colori
				idx := 0
				if bpp == 8 {
					idx = int(r[x])
				} else {
					idx = int(r[x/8]>>(7-uint(x%8))) & 1
				}
				if idx >= len(pal) {
					idx = len(pal) - 1
				}
				c = pal[idx]
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img, nil
}
