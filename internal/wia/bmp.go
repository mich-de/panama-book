package wia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
)

// decodeBMP decodifica un BMP non compresso (BI_RGB) a 8 (con tavolozza), 24 o 32 bit.
// Serve come ripiego se il driver WIA non sa produrre direttamente PNG (formato preferito: BMP).
func decodeBMP(data []byte) (image.Image, error) {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return nil, errors.New("bmp: intestazione non valida")
	}
	le := binary.LittleEndian
	off := int(le.Uint32(data[10:]))
	hs := int(le.Uint32(data[14:]))
	if hs < 40 {
		return nil, fmt.Errorf("bmp: intestazione da %d byte non supportata", hs)
	}
	w := int(int32(le.Uint32(data[18:])))
	h := int(int32(le.Uint32(data[22:])))
	bpp := int(le.Uint16(data[28:]))
	comp := le.Uint32(data[30:])
	used := int(le.Uint32(data[46:]))
	topDown := h < 0
	if topDown {
		h = -h
	}
	if w <= 0 || h <= 0 || w > 1<<15 || h > 1<<15 {
		return nil, fmt.Errorf("bmp: dimensioni non valide %dx%d", w, h)
	}
	if comp != 0 {
		return nil, fmt.Errorf("bmp: compressione %d non supportata", comp)
	}
	if bpp != 8 && bpp != 24 && bpp != 32 {
		return nil, fmt.Errorf("bmp: %d bit per pixel non supportati", bpp)
	}
	stride := (w*bpp + 31) / 32 * 4
	if off < 0 || off+stride*h > len(data) {
		return nil, errors.New("bmp: dati pixel incompleti")
	}
	var pal [256]color.RGBA
	if bpp == 8 {
		n := used
		if n == 0 || n > 256 {
			n = 256
		}
		p := 14 + hs
		if p+4*n > len(data) {
			return nil, errors.New("bmp: tavolozza incompleta")
		}
		for i := 0; i < n; i++ {
			pal[i] = color.RGBA{data[p+4*i+2], data[p+4*i+1], data[p+4*i], 255}
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := h - 1 - y
		if topDown {
			sy = y
		}
		row := data[off+sy*stride:]
		for x := 0; x < w; x++ {
			var c color.RGBA
			switch bpp {
			case 8:
				c = pal[row[x]]
			case 24:
				c = color.RGBA{row[3*x+2], row[3*x+1], row[3*x], 255}
			case 32:
				c = color.RGBA{row[4*x+2], row[4*x+1], row[4*x], 255}
			}
			dst.SetRGBA(x, y, c)
		}
	}
	return dst, nil
}
