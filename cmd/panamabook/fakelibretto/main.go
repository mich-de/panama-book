// Comando fakelibretto: finto "libretto.exe" per provare PanamaBook senza scanner e senza dati reali.
// Serve SOLO ai test e agli screenshot: non tocca nessuno scanner, scrive solo dove gli si dice e stampa JSON SINTETICO
// (nomi e numeri inventati, MRZ dell'esempio di docs/campi.md).
//
//	fakelibretto scan -o FILE.png ...   scrive un PNG sintetico (oppure simula un errore, vedi FAKE_SCAN)
//	fakelibretto extract -i FILE.png    stampa un JSON di esempio (variante scelta da FAKE_EXTRACT)
//
// Variabili d'ambiente: FAKE_SCAN = ok (predefinito) | paper | jam; FAKE_EXTRACT = ok (predefinito) | mrz_invalid |
// no_mrz | slow | fail | legacy; FAKE_SURNAME, FAKE_GIVEN = cognome e nomi sintetici; FAKE_LINE1 = uncertain aggiunge MRZ_LINE1_UNCERTAIN; FAKE_ARGS = file dove registrare gli argomenti ricevuti (uno per riga, se impostato).
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if p := os.Getenv("FAKE_ARGS"); p != "" {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			_ = f.Close()
		}
	}
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "pdf":
		pdf(os.Args[2:])
	case "scan":
		scan(os.Args[2:])
	case "extract":
		extract(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "comando sconosciuto")
		os.Exit(2)
	}
}

// usage stampa la guida come libretto.exe senza argomenti; se il nome dell'eseguibile contiene "nopdf" la riga del comando
// pdf manca (simula un libretto.exe vecchio).
func usage() {
	exe := strings.ToLower(os.Args[0])
	fmt.Fprintln(os.Stderr, "uso:")
	fmt.Fprintln(os.Stderr, "  libretto info")
	if !strings.Contains(exe, "nopdf") {
		fmt.Fprintln(os.Stderr, "  libretto pdf     -i scan.png [-o scan.pdf] [-max 1000000]")
	}
	fmt.Fprintln(os.Stderr, "  libretto extract -i scan.png")
	os.Exit(2)
}

// pdf scrive un finto PDF di FAKE_PDF_SIZE byte (predefinito 2000) e stampa una riga come libretto.exe pdf.
func pdf(args []string) {
	if strings.Contains(strings.ToLower(os.Args[0]), "nopdf") {
		usage()
	}
	in, out := arg(args, "-i"), arg(args, "-o")
	force := false
	for _, a := range args {
		if a == "-force" {
			force = true
		}
	}
	if in == "" || out == "" {
		usage()
	}
	if _, err := os.Stat(out); err == nil && !force {
		fmt.Fprintf(os.Stderr, "errore: %q esiste gia' (usa -force)\n", out)
		os.Exit(1)
	}
	size := 2000
	if v := os.Getenv("FAKE_PDF_SIZE"); v != "" {
		fmt.Sscan(v, &size)
	}
	body := append([]byte("%PDF-1.4\n"), make([]byte, size)...)
	if err := os.WriteFile(out, body, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
	fmt.Printf("%s: %d KB (limite 1000 KB), qualita' JPEG 90, immagine al 100%% (10x10 px)\n", out, (size+512)/1024)
}

func arg(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func scan(args []string) {
	switch os.Getenv("FAKE_SCAN") {
	case "paper":
		fmt.Fprintln(os.Stderr, "errore: carta assente (TWAIN)")
		os.Exit(3)
	case "jam":
		fmt.Fprintln(os.Stderr, "errore: inceppamento (TWAIN)")
		os.Exit(4)
	}
	out := arg(args, "-o")
	if out == "" {
		fmt.Fprintln(os.Stderr, "errore: manca -o")
		os.Exit(2)
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o700)
	if _, err := os.Stat(out); err == nil {
		fmt.Fprintln(os.Stderr, "errore: il file esiste gia'")
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "area richiesta: da 0 mm, lunghezza 97 mm, larghezza 0 mm (0 = tutto il campo)")
	time.Sleep(300 * time.Millisecond)
	w, h := 120, 80
	if v := os.Getenv("FAKE_SIZE"); v != "" {
		fmt.Sscanf(v, "%dx%d", &w, &h)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 235, G: 238, B: 242, A: 255})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "errore:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		os.Exit(1)
	}
	if raw := arg(args, "-raw"); raw != "" { // come libretto.exe: la scansione grezza, prima di ritaglio e controllo del vuoto
		writeRaw(raw, args)
	}
	fmt.Println("salvato", out)
}

func extract(args []string) {
	if arg(args, "-i") == "" {
		fmt.Fprintln(os.Stderr, "uso: extract -i FILE")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stderr, "lettura in corso (finto)")
	// -page-out: la sola pagina dati ritagliata (come libretto.exe, che la salva solo se trova le righe MRZ)
	if po := arg(args, "-page-out"); po != "" {
		if os.Getenv("FAKE_PAGE") == "none" || os.Getenv("FAKE_EXTRACT") == "empty" || os.Getenv("FAKE_EXTRACT") == "no_mrz" {
			fmt.Fprintln(os.Stderr, "avviso: pagina dati non ritagliata: righe MRZ non trovate")
		} else if _, err := os.Stat(po); err == nil && !hasFlag(args, "-force") {
			fmt.Fprintf(os.Stderr, "avviso: pagina dati non salvata: %q esiste gia' (usa -force)\n", po)
		} else {
			pg := image.NewRGBA(image.Rect(0, 0, 200, 140))
			for y := 0; y < 140; y++ {
				for x := 0; x < 200; x++ {
					pg.Set(x, y, color.RGBA{R: 226, G: 232, B: 214, A: 255})
				}
			}
			save(po, pg)
		}
	}
	mode := os.Getenv("FAKE_EXTRACT")
	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "errore: OCR non disponibile")
		os.Exit(1)
	case "slow":
		time.Sleep(60 * time.Second)
	case "slowish":
		time.Sleep(1500 * time.Millisecond)
	}
	if mode == "empty" { // come una scansione fallita: solo l'informativa, nessuna MRZ, nessun campo
		fmt.Println(`{"schema_version": 1, "rotation_deg": 0, "fields": {}, "warnings": [{"code": "MRZ_NOT_FOUND"}, {"code": "PRINTED_FIELDS_NOT_READ", "fields": ["capacity", "certificate", "country_code", "date_of_birth", "date_of_expiry", "date_of_issue", "id_number", "level", "nationality", "place_of_birth", "regional", "seafarers_id", "seamans_book", "type"]}]}`)
		return
	}
	checks := `"doc_number": true, "birth": true, "expiry": true, "optional": true, "composite": true`
	mrz := true
	warnings := `{"code": "UNCERTAIN_FIELD", "field": "capacity"}, {"code": "PRINTED_FIELDS_NOT_READ", "fields": ["certificate"]}`
	capacity := "ORDINARY SEAMEN"
	if v := os.Getenv("FAKE_CAPACITY"); v != "" {
		capacity = v
	}
	cert := ""
	switch mode {
	case "mrz_invalid":
		checks = `"doc_number": true, "birth": false, "expiry": true, "optional": true, "composite": false`
		warnings = `{"code": "MRZ_CHECK_DIGITS_INVALID"}`
	case "no_mrz":
		mrz = false
		warnings = `{"code": "MRZ_NOT_FOUND"}, {"code": "PRINTED_FIELDS_NOT_READ", "fields": ["certificate", "regional"]}`
	case "legacy":
		cert = `"endorsement": "123456789",`
	}
	var b strings.Builder
	b.WriteString(`{"schema_version": 1, "rotation_deg": 0,`)
	if mrz {
		fmt.Fprintf(&b, `"mrz": {"doc_code": "IS", "issuer": "PAN", "surname": "ROSSI", "given_names": "MARIO", "doc_number": "P1234567<", "nationality": "ITA", "birth_date": "1990-01-01", "sex": "M", "expiry_date": "2030-12-31", "optional": "PANAMA", "line1": "ISPANROSSI<<MARIO<<<<<<<<<<<<<<<<<<<<<<<<<<<", "line2": "P1234567<1ITA9001011M3012316PANAMA<<<<<<<<44", "checks": {%s}},`, checks)
		b.WriteString(`"mrz_corrections": ["line2 pos 14: S->5"],`)
	}
	fmt.Fprintf(&b, `"fields": {%s "type": "SI", "country_code": "PAN", "nationality": "ITALIAN", "place_of_birth": "GENOVA ITA", "date_of_issue": "01-02-2026", "date_of_expiry": "31-12-2030", "date_of_birth": "01-01-1990", "level": "SUPPORT", "capacity": %q, "seamans_book": "P1234567", "id_number": "C12345678", "seafarers_id": "123456789"`, cert, capacity)
	if mrz {
		b.WriteString(`, "surname": "ROSSI", "given_names": "MARIO", "nationality_iso": "ITA", "sex": "M", "date_of_birth_mrz": "1990-01-01", "date_of_expiry_mrz": "2030-12-31", "seamans_book_mrz": "P1234567", "regional_mrz": "PANAMA", "regional": "PANAMA"`)
	}
	b.WriteString(`}, "warnings": [` + warnings + `]}`)
	out := b.String()
	// nomi sintetici a scelta (anche con accenti e caratteri speciali) per provare la rinomina dei file
	for env, key := range map[string]string{"FAKE_SURNAME": `"surname": "ROSSI"`, "FAKE_GIVEN": `"given_names": "MARIO"`} {
		if v := os.Getenv(env); v != "" {
			q, _ := json.Marshal(v)
			out = strings.ReplaceAll(out, key, key[:strings.Index(key, ":")+2]+string(q))
		}
	}
	if os.Getenv("FAKE_LINE1") == "uncertain" {
		out = strings.Replace(out, `"warnings": [`, `"warnings": [{"code": "MRZ_LINE1_UNCERTAIN"}, `, 1)
	}
	fmt.Println(out)
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}
