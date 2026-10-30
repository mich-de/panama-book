package qrcode

// Reed-Solomon su GF(256) con polinomio 0x11D, radici alfa^0..alfa^(ec-1) (come nello standard dei QR).

var (
	gfExp [512]byte
	gfLog [256]int
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		gfExp[i] = byte(x)
		gfLog[x] = i
		if x <<= 1; x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	for i := 255; i < 512; i++ {
		gfExp[i] = gfExp[i-255]
	}
}

func gmul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExp[gfLog[a]+gfLog[b]]
}

func gdiv(a, b byte) byte {
	if a == 0 {
		return 0
	}
	return gfExp[(gfLog[a]+255-gfLog[b])%255]
}

// rsCorrect corregge sul posto un blocco (dati + correzione). Restituisce falso se gli errori sono troppi.
func rsCorrect(msg []byte, ec int) bool {
	n := len(msg)
	syn := make([]byte, ec)
	clean := true
	for j := 0; j < ec; j++ {
		var s byte
		for i := 0; i < n; i++ {
			s = gmul(s, gfExp[j]) ^ msg[i]
		}
		syn[j] = s
		if s != 0 {
			clean = false
		}
	}
	if clean {
		return true
	}
	// Berlekamp-Massey: polinomio localizzatore lam
	lam := []byte{1}
	prev := []byte{1}
	l, m := 0, 1
	b := byte(1)
	for i := 0; i < ec; i++ {
		d := syn[i]
		for k := 1; k <= l && k < len(lam); k++ {
			d ^= gmul(lam[k], syn[i-k])
		}
		if d == 0 {
			m++
			continue
		}
		t := append([]byte(nil), lam...)
		coef := gdiv(d, b)
		need := len(prev) + m
		for len(lam) < need {
			lam = append(lam, 0)
		}
		for k, v := range prev {
			lam[k+m] ^= gmul(coef, v)
		}
		if 2*l <= i {
			l, prev, b, m = i+1-l, t, d, 1
		} else {
			m++
		}
	}
	for len(lam) > 0 && lam[len(lam)-1] == 0 {
		lam = lam[:len(lam)-1]
	}
	if l == 0 || l > ec/2 || len(lam) != l+1 {
		return false
	}
	// ricerca delle posizioni (Chien): errore in posizione i se lam(alfa^-(n-1-i)) = 0
	var pos []int
	for i := 0; i < n; i++ {
		x := gfExp[(255-(n-1-i)%255)%255]
		var v, xp byte = 0, 1
		for _, c := range lam {
			v ^= gmul(c, xp)
			xp = gmul(xp, x)
		}
		if v == 0 {
			pos = append(pos, i)
		}
	}
	if len(pos) != l {
		return false
	}
	// omega = syn * lam mod x^ec
	omega := make([]byte, ec)
	for i := 0; i < ec; i++ {
		for k := 0; k <= i && k < len(lam); k++ {
			omega[i] ^= gmul(lam[k], syn[i-k])
		}
	}
	for _, p := range pos {
		xinv := gfExp[(255-(n-1-p)%255)%255]
		var num, xp byte = 0, 1
		for _, c := range omega {
			num ^= gmul(c, xp)
			xp = gmul(xp, xinv)
		}
		var den byte // derivata formale di lam in xinv: solo i termini dispari
		xp = 1
		for k := 1; k < len(lam); k += 2 {
			den ^= gmul(lam[k], xp)
			xp = gmul(xp, gmul(xinv, xinv))
		}
		if den == 0 {
			return false
		}
		msg[p] ^= gmul(gfExp[(n-1-p)%255], gdiv(num, den))
	}
	// controllo finale: sindromi tutte zero
	for j := 0; j < ec; j++ {
		var s byte
		for i := 0; i < n; i++ {
			s = gmul(s, gfExp[j]) ^ msg[i]
		}
		if s != 0 {
			return false
		}
	}
	return true
}
