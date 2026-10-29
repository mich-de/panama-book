package libreta

// Ritaglio dei campi dalla scansione: funzioni usate da vizBuildSheet (foglio, ritagli aggiuntivi, jitter) e dai
// secondi passaggi mirati. Una variante (vizVariant) fissa scala, finestra verticale, assottigliamento e soglia.

import "image"

// vizBandRect e' il rettangolo del campo b nella variante v, in pixel della scansione. ox, oy (in unita' di W) lo
// spostano (varianti "jitter"). L'inclinazione della scansione sposta il ritaglio in verticale in proporzione alla
// distanza dal margine sinistro della MRZ; la calibrazione per colonna (viz_calib.go) lo sposta del resto.
func vizBandRect(geo vizGeom, b vizBand, v vizVariant, ox, oy float64) image.Rectangle {
	up, down := vizWindow(b, v)
	dy := geo.skew*(b.xa+b.xb)/2*geo.w + (oy+vizCalOffset(geo, b))*geo.w
	dx := ox * geo.w
	return image.Rect(
		int(geo.x0+b.xa*geo.w+dx), int(geo.y0+(b.yc-up)*geo.w+dy),
		int(geo.x0+b.xb*geo.w+dx), int(geo.y0+(b.yc+down)*geo.w+dy))
}

// vizPrepCrop ritaglia r da g, lo porta alla scala della variante v (v.unit pixel per W) e applica assottigliamento e
// rampa di binarizzazione. maxW e maxH limitano il risultato (0 = nessun limite).
func vizPrepCrop(g *image.Gray, geo vizGeom, r image.Rectangle, v vizVariant, maxW, maxH int) (vals []uint8, w, h int) {
	s := min(max(v.unit/geo.w, 0.3), 6)
	vals, w, h = vizCropVals(g, r, s, maxW, maxH)
	if vals == nil {
		return nil, 0, 0
	}
	if v.thin {
		vals = vizThin(vals, w, h)
	}
	if v.thr > 0 {
		vizSoftRamp(vals, v.thr)
	}
	return vals, w, h
}

// vizPadGray mette i valori in un'immagine con un margine bianco (vizCropMargin) per l'OCR separato del ritaglio.
func vizPadGray(vals []uint8, w, h int) *image.Gray {
	cg := image.NewGray(image.Rect(0, 0, w+2*vizCropMargin, h+2*vizCropMargin))
	for i := range cg.Pix {
		cg.Pix[i] = 255
	}
	for y := 0; y < h; y++ {
		copy(cg.Pix[(y+vizCropMargin)*cg.Stride+vizCropMargin:], vals[y*w:(y+1)*w])
	}
	return cg
}
