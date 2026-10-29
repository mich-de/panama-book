// Package mrz legge e valida la zona a lettura ottica (ICAO 9303, formato TD3: 2 righe da 44 caratteri).
// La Libreta de embarque di Panama usa questo formato con codice documento "IS".
package mrz

import (
	"fmt"
	"strings"
	"time"
)

const lineLen = 44

// now e' l'orologio usato per decidere il secolo della data di nascita: iniettabile nei test.
var now = time.Now

// Data contiene i campi letti dall'MRZ e l'esito dei controlli ICAO.
type Data struct {
	DocCode     string `json:"doc_code"`
	Issuer      string `json:"issuer"`
	Surname     string `json:"surname"`
	GivenNames  string `json:"given_names"`
	DocNumber   string `json:"doc_number"`
	Nationality string `json:"nationality"`
	BirthDate   string `json:"birth_date"` // YYYY-MM-DD
	Sex         string `json:"sex"`
	ExpiryDate  string `json:"expiry_date"` // YYYY-MM-DD
	Optional    string `json:"optional"`

	Line1 string `json:"line1"`
	Line2 string `json:"line2"`

	Checks Checks `json:"checks"`
}

// Checks riporta quali check digit tornano.
type Checks struct {
	DocNumber bool `json:"doc_number"`
	Birth     bool `json:"birth"`
	Expiry    bool `json:"expiry"`
	Optional  bool `json:"optional"`
	Composite bool `json:"composite"`
}

// AllOK e' vero se tutti i controlli sono validi.
func (c Checks) AllOK() bool {
	return c.DocNumber && c.Birth && c.Expiry && c.Optional && c.Composite
}

var weights = [3]int{7, 3, 1}

// CheckDigit calcola il check digit ICAO (pesi 7-3-1) di s.
func CheckDigit(s string) byte {
	sum := 0
	for i, r := range s {
		var v int
		switch {
		case r >= '0' && r <= '9':
			v = int(r - '0')
		case r >= 'A' && r <= 'Z':
			v = int(r-'A') + 10
		default: // '<' e qualunque altro carattere valgono 0
			v = 0
		}
		sum += v * weights[i%3]
	}
	return byte('0' + sum%10)
}

// Parse legge le due righe MRZ. Restituisce i dati anche se i check digit non tornano:
// e' il chiamante a decidere cosa fare guardando Data.Checks.
func Parse(line1, line2 string) (Data, error) {
	l1, l2 := clean(line1), clean(line2)
	if len(l1) != lineLen || len(l2) != lineLen {
		return Data{}, fmt.Errorf("mrz: servono 2 righe da %d caratteri, trovate %d e %d", lineLen, len(l1), len(l2))
	}
	d := Data{Line1: l1, Line2: l2}
	d.DocCode = strip(l1[0:2])
	d.Issuer = strip(l1[2:5])
	d.Surname, d.GivenNames = splitNames(l1[5:44])

	d.DocNumber = strip(l2[0:9])
	d.Nationality = strip(l2[10:13])
	d.BirthDate = date(l2[13:19], true)
	d.Sex = strip(l2[20:21])
	d.ExpiryDate = date(l2[21:27], false)
	d.Optional = strip(l2[28:42])

	d.Checks = Checks{
		DocNumber: CheckDigit(l2[0:9]) == l2[9],
		Birth:     CheckDigit(l2[13:19]) == l2[19],
		Expiry:    CheckDigit(l2[21:27]) == l2[27],
		Optional:  optionalOK(l2[28:42], l2[42]),
		Composite: CheckDigit(l2[0:10]+l2[13:20]+l2[21:43]) == l2[43],
	}
	return d, nil
}

// optionalOK: se il campo e' tutto '<' il check digit puo' essere '<' o '0'.
func optionalOK(field string, cd byte) bool {
	if strings.Trim(field, "<") == "" && (cd == '<' || cd == '0') {
		return true
	}
	return CheckDigit(field) == cd
}

func clean(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	return s
}

func strip(s string) string { return strings.TrimRight(strings.ReplaceAll(s, "<", " "), " ") }

func splitNames(s string) (surname, given string) {
	s = strings.TrimRight(s, "<")
	parts := strings.SplitN(s, "<<", 2)
	surname = strings.TrimSpace(strings.ReplaceAll(parts[0], "<", " "))
	if len(parts) > 1 {
		given = strings.TrimSpace(strings.ReplaceAll(parts[1], "<", " "))
	}
	return
}

// date converte YYMMDD in YYYY-MM-DD. Per la nascita un anno nel futuro e' del secolo scorso;
// la scadenza e' sempre letta nel 2000. Se la data non e' valida restituisce la stringa grezza.
func date(yymmdd string, birth bool) string {
	if len(yymmdd) != 6 {
		return yymmdd
	}
	var yy, mm, dd int
	if _, err := fmt.Sscanf(yymmdd, "%02d%02d%02d", &yy, &mm, &dd); err != nil {
		return yymmdd
	}
	year := 2000 + yy
	if birth && year > now().Year() {
		year -= 100
	}
	if _, err := time.Parse("2006-01-02", fmt.Sprintf("%04d-%02d-%02d", year, mm, dd)); err != nil {
		return yymmdd
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, mm, dd)
}
