// Package pdftext legge il testo della prima pagina di un PDF DIGITALE (quello con il testo incorporato, come i certificati
// dell'Autorita' Marittima di Panama) senza programmi esterni: niente PowerShell, niente OCR, niente finestre. Restituisce le
// righe fisiche della pagina (stesso formato di certif.RowsFromOCR), cosi' il lettore dei certificati usa lo stesso percorso.
//
// Copre i PDF con font semplici (Type1/TrueType con codifica WinAnsi, come quelli di iText e JasperReports), stream FlateDecode e
// object stream. Per il resto (font Type0/CID, PDF cifrati, scansioni dentro un PDF) restituisce ErrUnsupported e chi chiama
// ripiega sul rendering in immagine con OCR. Il file e' trattato come NON fidato: si legge solo, limiti su dimensioni e profondita'.
package pdftext

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrUnsupported dice che il PDF non si legge qui (font composti, cifratura, nessun testo): serve il percorso con l'OCR.
var ErrUnsupported = errors.New("pdf: struttura non supportata")

const (
	maxFile   = 25 << 20
	maxStream = 32 << 20 // dimensione massima di uno stream decompresso
	maxItems  = 20000
)

type object struct {
	dict   string
	gen    int
	raw    []byte // stream cosi' come sta nel file (forse cifrato e compresso)
	stream []byte // stream decifrato e decompresso; nil se l'oggetto non ha stream o non si legge
}

var (
	reObj     = regexp.MustCompile(`(\d{1,9})\s+(\d{1,5})\s+obj\b`)
	reStream  = regexp.MustCompile(`>>\s*stream\r?\n`)
	reRef     = regexp.MustCompile(`(\d{1,9})\s+\d{1,5}\s+R`)
	reTypePg  = regexp.MustCompile(`/Type\s*/Page\b`)
	reTypePgs = regexp.MustCompile(`/Type\s*/Pages\b`)
	reKids    = regexp.MustCompile(`(?s)/Kids\s*\[(.*?)\]`)
	reParent  = regexp.MustCompile(`/Parent\s+\d`)
)

// parseObjects legge tutti gli oggetti del file nell'ordine in cui compaiono (se un oggetto e' ridefinito, vale l'ultimo) e apre
// gli object stream.
func parseObjects(data []byte) map[int]*object {
	objs := map[int]*object{}
	pos := 0
	for pos < len(data) {
		loc := reObj.FindSubmatchIndex(data[pos:])
		if loc == nil {
			break
		}
		num, _ := strconv.Atoi(string(data[pos+loc[2] : pos+loc[3]]))
		gen, _ := strconv.Atoi(string(data[pos+loc[4] : pos+loc[5]]))
		start := pos + loc[1]
		rest := data[start:]
		end := bytes.Index(rest, []byte("endobj"))
		o := &object{gen: gen}
		if sl := reStream.FindIndex(rest); sl != nil && (end < 0 || sl[0] < end) {
			o.dict = string(rest[:sl[0]+2])
			body := rest[sl[1]:]
			ln := bytes.Index(body, []byte("endstream"))
			if ln < 0 {
				break
			}
			o.raw = bytes.TrimRight(body[:ln], "\r\n")
			if e2 := bytes.Index(body[ln:], []byte("endobj")); e2 >= 0 {
				end = (len(rest) - len(body)) + ln + e2
			} else {
				end = len(rest)
			}
		} else if end >= 0 {
			o.dict = string(rest[:end])
		} else {
			break
		}
		objs[num] = o
		pos = start + end + len("endobj")
	}
	return objs
}

// open decifra e decomprime gli stream e apre gli object stream (che contengono altri oggetti).
func open(data []byte, objs map[int]*object) error {
	cr, err := newCryptor(data, objs)
	if err != nil {
		return err
	}
	for n, o := range objs {
		if o.raw == nil {
			continue
		}
		isImg := strings.Contains(o.dict, "/Image")
		if isImg && !squareImage(o.dict) { // servono solo le immagini quadrate di dimensione da QR
			continue
		}
		raw := o.raw
		if cr != nil && !strings.Contains(o.dict, "/XRef") {
			if raw = cr.decrypt(n, o.gen, raw); raw == nil {
				continue
			}
		}
		if isImg && strings.Contains(o.dict, "/DCTDecode") {
			o.stream = raw // JPEG: lo decodifica image/jpeg
			continue
		}
		o.stream = decode(o.dict, raw)
	}
	for _, o := range objs {
		if o.stream != nil && strings.Contains(o.dict, "/ObjStm") {
			openObjStm(o, objs)
		}
	}
	return nil
}

var (
	reN     = regexp.MustCompile(`/N\s+(\d+)`)
	reFirst = regexp.MustCompile(`/First\s+(\d+)`)
)

func openObjStm(o *object, objs map[int]*object) {
	n, first := atoi(reN, o.dict), atoi(reFirst, o.dict)
	if n <= 0 || n > 100000 || first <= 0 || first > len(o.stream) {
		return
	}
	hdr := strings.Fields(string(o.stream[:first]))
	type ent struct{ num, off int }
	var es []ent
	for i := 0; i+1 < len(hdr) && len(es) < n; i += 2 {
		a, err1 := strconv.Atoi(hdr[i])
		b, err2 := strconv.Atoi(hdr[i+1])
		if err1 != nil || err2 != nil {
			return
		}
		es = append(es, ent{a, b})
	}
	for i, e := range es {
		s, t := first+e.off, len(o.stream)
		if i+1 < len(es) {
			t = first + es[i+1].off
		}
		if s < 0 || s > t || t > len(o.stream) {
			continue
		}
		if _, ok := objs[e.num]; !ok {
			objs[e.num] = &object{dict: string(o.stream[s:t])}
		}
	}
}

func atoi(re *regexp.Regexp, s string) int {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// decode toglie FlateDecode (l'unico filtro che serve); altri filtri: lo stream si lascia com'e' solo se non ce ne sono.
func decode(dict string, raw []byte) []byte {
	if !strings.Contains(dict, "/Filter") {
		return raw
	}
	if !strings.Contains(dict, "/FlateDecode") || strings.Contains(dict, "/DecodeParms") && strings.Contains(dict, "/Predictor") {
		return nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, maxStream+1))
	if err != nil && len(out) == 0 || len(out) > maxStream {
		return nil
	}
	return out
}

// firstPage trova la prima pagina seguendo l'albero /Pages (ripiego: la pagina con il numero di oggetto piu' basso).
func firstPage(objs map[int]*object) (int, *object) {
	var root int
	for n, o := range objs {
		if reTypePgs.MatchString(o.dict) && !reParent.MatchString(o.dict) && (root == 0 || n < root) {
			root = n
		}
	}
	for depth, cur := 0, root; cur != 0 && depth < 20; depth++ {
		o := objs[cur]
		if o == nil {
			break
		}
		if reTypePg.MatchString(o.dict) {
			return cur, o
		}
		k := reKids.FindStringSubmatch(o.dict)
		if k == nil {
			break
		}
		r := reRef.FindStringSubmatch(k[1])
		if r == nil {
			break
		}
		cur, _ = strconv.Atoi(r[1])
	}
	low := 0
	for n, o := range objs {
		if reTypePg.MatchString(o.dict) && (low == 0 || n < low) {
			low = n
		}
	}
	return low, objs[low]
}

// pageContent unisce gli stream del contenuto della pagina.
func pageContent(objs map[int]*object, page *object) ([]byte, error) {
	i := strings.Index(page.dict, "/Contents")
	if i < 0 {
		return nil, ErrUnsupported
	}
	rest := strings.TrimLeft(page.dict[i+len("/Contents"):], " \r\n\t")
	var refs []string
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return nil, ErrUnsupported
		}
		for _, m := range reRef.FindAllStringSubmatch(rest[:end], -1) {
			refs = append(refs, m[1])
		}
	} else if m := reRef.FindStringSubmatch(rest); m != nil && strings.HasPrefix(rest, m[0]) {
		refs = append(refs, m[1])
	}
	var buf bytes.Buffer
	for _, r := range refs {
		n, _ := strconv.Atoi(r)
		if o := objs[n]; o != nil && o.stream != nil {
			buf.Write(o.stream)
			buf.WriteByte('\n')
		}
	}
	if buf.Len() == 0 {
		return nil, ErrUnsupported
	}
	return buf.Bytes(), nil
}

// Rows restituisce le righe di testo della prima pagina, dall'alto in basso e da sinistra a destra.
func Rows(data []byte) ([]string, error) {
	if len(data) > maxFile || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, ErrUnsupported
	}
	objs := parseObjects(data)
	if err := open(data, objs); err != nil {
		return nil, err
	}
	for _, o := range objs { // font composti (CID): il testo non e' leggibile senza le loro tabelle
		if strings.Contains(o.dict, "/Type0") || strings.Contains(o.dict, "/Identity-H") {
			return nil, ErrUnsupported
		}
	}
	_, page := firstPage(objs)
	if page == nil {
		return nil, ErrUnsupported
	}
	content, err := pageContent(objs, page)
	if err != nil {
		return nil, err
	}
	items := runContent(content)
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: nessun testo", ErrUnsupported)
	}
	return toRows(items), nil
}

type item struct {
	x, y, size float64
	text       string
}

// toRows raggruppa i frammenti per riga (stessa quota entro mezza altezza) e li ordina da sinistra a destra.
func toRows(items []item) []string {
	sort.SliceStable(items, func(i, j int) bool { return items[i].y > items[j].y })
	var rows [][]item
	var ys []float64
	for _, it := range items {
		if n := len(rows); n > 0 && ys[n-1]-it.y <= 0.5*maxf(it.size, 4) {
			rows[n-1] = append(rows[n-1], it)
			continue
		}
		rows = append(rows, []item{it})
		ys = append(ys, it.y)
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		sort.SliceStable(r, func(i, j int) bool { return r[i].x < r[j].x })
		parts := make([]string, 0, len(r))
		for _, it := range r {
			if t := strings.TrimSpace(it.text); t != "" {
				parts = append(parts, t)
			}
		}
		if s := strings.Join(parts, " "); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
