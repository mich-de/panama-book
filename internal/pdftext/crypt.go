package pdftext

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

// PDF «cifrati» con la sola password del proprietario (utente vuota): e' il caso dei certificati AMP, che ogni lettore apre
// senza chiedere nulla (la cifratura serve a vietare copia e modifica). Si implementa il gestore standard con RC4 (V1, V2) e
// AES-128 (V4, AESV2), come da specifica PDF 1.7, algoritmo 2 (chiave) e 1 (chiave per oggetto). Con una password utente vera
// o con AES-256 si restituisce ErrUnsupported.

var (
	reEncRef = regexp.MustCompile(`/Encrypt\s+(\d+)\s+\d+\s+R`)
	reID     = regexp.MustCompile(`/ID\s*\[\s*<([0-9a-fA-F]+)>`)
	reV      = regexp.MustCompile(`/V\s+(\d+)`)
	reR      = regexp.MustCompile(`/R\s+(\d+)`)
	reLen    = regexp.MustCompile(`/Length\s+(\d+)`)
	reP      = regexp.MustCompile(`/P\s+(-?\d+)`)
	reOkey   = regexp.MustCompile(`/O\s*[(<]`)
)

var pad = []byte{0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80, 0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A}

type cryptor struct {
	key []byte
	aes bool
}

// newCryptor restituisce nil se il PDF non e' cifrato.
func newCryptor(data []byte, objs map[int]*object) (*cryptor, error) {
	m := reEncRef.FindSubmatch(data)
	if m == nil {
		if bytes.Contains(data, []byte("/Encrypt")) {
			return nil, ErrUnsupported
		}
		return nil, nil
	}
	n, _ := strconv.Atoi(string(m[1]))
	eo := objs[n]
	idm := reID.FindSubmatch(data)
	if eo == nil || idm == nil {
		return nil, ErrUnsupported
	}
	d := eo.dict
	v, r := atoi(reV, d), atoi(reR, d)
	if v == 5 || r >= 5 || !strings.Contains(d, "/Standard") {
		return nil, ErrUnsupported
	}
	bits := atoi(reLen, d)
	cr := &cryptor{aes: strings.Contains(d, "AESV2")}
	if cr.aes && v < 4 || v == 4 && !cr.aes && !strings.Contains(d, "/V2") {
		return nil, ErrUnsupported
	}
	klen := 5
	if r >= 3 {
		klen = 16
		if bits >= 40 && bits <= 128 && bits%8 == 0 {
			klen = bits / 8
		}
		if v == 4 {
			klen = 16
		}
	}
	pm := reP.FindStringSubmatch(d)
	if pm == nil {
		return nil, ErrUnsupported
	}
	pv, err := strconv.ParseInt(pm[1], 10, 64)
	if err != nil {
		return nil, ErrUnsupported
	}
	o := stringAfter(d, reOkey)
	id0, _ := hex.DecodeString(string(idm[1]))
	if len(o) != 32 || len(id0) == 0 {
		return nil, ErrUnsupported
	}
	h := md5.New()
	h.Write(pad)
	h.Write(o)
	var p4 [4]byte
	binary.LittleEndian.PutUint32(p4[:], uint32(int32(pv)))
	h.Write(p4[:])
	h.Write(id0)
	if r >= 4 && strings.Contains(d, "/EncryptMetadata false") {
		h.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}
	sum := h.Sum(nil)
	if r >= 3 {
		for i := 0; i < 50; i++ {
			x := md5.Sum(sum[:klen])
			sum = x[:]
		}
	}
	cr.key = append([]byte(nil), sum[:klen]...)
	return cr, nil
}

// stringAfter legge la stringa (letterale o esadecimale) che segue la chiave trovata da re.
func stringAfter(d string, re *regexp.Regexp) []byte {
	loc := re.FindStringIndex(d)
	if loc == nil {
		return nil
	}
	t := &tokenizer{b: []byte(d), i: loc[1] - 1}
	x, ok := t.next()
	if !ok || x.kind != 's' {
		return nil
	}
	return []byte(x.str)
}

// decrypt decifra i byte di uno stream dell'oggetto num gen. Restituisce nil se non si riesce (dati corrotti).
func (c *cryptor) decrypt(num, gen int, b []byte) []byte {
	k := append(append([]byte(nil), c.key...), byte(num), byte(num>>8), byte(num>>16), byte(gen), byte(gen>>8))
	if c.aes {
		k = append(k, 's', 'A', 'l', 'T')
	}
	sum := md5.Sum(k)
	n := len(c.key) + 5
	if n > 16 {
		n = 16
	}
	ok := sum[:n]
	if !c.aes {
		rc, err := rc4.NewCipher(ok)
		if err != nil {
			return nil
		}
		out := make([]byte, len(b))
		rc.XORKeyStream(out, b)
		return out
	}
	if len(b) < 32 || len(b)%16 != 0 {
		return nil
	}
	blk, err := aes.NewCipher(ok)
	if err != nil {
		return nil
	}
	out := make([]byte, len(b)-16)
	cipher.NewCBCDecrypter(blk, b[:16]).CryptBlocks(out, b[16:])
	if pn := int(out[len(out)-1]); pn >= 1 && pn <= 16 && pn <= len(out) {
		out = out[:len(out)-pn]
	}
	return out
}
