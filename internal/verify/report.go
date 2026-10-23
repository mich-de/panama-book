package verify

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"scanlibretto/internal/outsum"
)

// Le funzioni di questo file scrivono solo nomi di campo, conteggi e classi di errore: mai un valore letto o una
// verita' (vedi la regola di privacy nel commento del pacchetto).

func mrzSummary(s outsum.Summary) string {
	if !s.HasMRZ {
		return "MRZ no"
	}
	return fmt.Sprintf("MRZ si, check digit %d/%d", s.ChecksOK(), outsum.NChecks)
}

func fixText(m MRZFix) string {
	switch m {
	case MRZFixes:
		return "l'MRZ lo avrebbe corretto"
	case MRZNoFix:
		return "l'MRZ da solo non lo corregge"
	case MRZAbsent:
		return "il campo e' nell'MRZ ma l'MRZ non c'e' o non e' valido"
	}
	return ""
}

// WriteResult scrive l'esito di un file: un rigo per campo (con la verita') e l'elenco delle incoerenze.
func WriteResult(w io.Writer, r Result) {
	fmt.Fprintf(w, "%s: rotazione %d, %s, campi letti %d/%d\n", r.Name, r.Sum.Rotation, mrzSummary(r.Sum), r.Sum.Found(), outsum.NFields)
	if r.HasTruth {
		for _, fr := range r.Fields {
			line := fmt.Sprintf("  %-16s %s", Canonical(fr.Field), fr.Status)
			var extra []string
			if fr.Status == StatusWrong {
				extra = append(extra, fr.Class.String())
			}
			if t := fixText(fr.MRZ); t != "" && (fr.Status == StatusWrong || fr.Status == StatusMissing) {
				extra = append(extra, t)
			}
			if len(extra) > 0 {
				line += "  (" + strings.Join(extra, "; ") + ")"
			}
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
		if r.Names != NameNA {
			fmt.Fprintf(w, "  %-16s %s  (cognome e nomi dell'MRZ contro la riga del nome stampata)\n", "riga del nome", r.Names)
		}
		c := r.Counts()
		fmt.Fprintf(w, "  esito: %d giusti su %d con verita'; sbagliati %d, mancanti %d, non stampati %d, in piu' %d\n",
			c.OK, c.WithTruth(), c.Wrong, c.Missing, c.NotPrinted, c.Extra)
		if len(r.Absent) > 0 {
			fmt.Fprintf(w, "  attenzione: la verita' non ha le chiavi %s (trattate come vuote)\n", strings.Join(canonicalAll(r.Absent), ", "))
		}
	} else {
		fmt.Fprintln(w, "  nessuna verita': solo regole di coerenza")
	}
	errs, warns, infos := r.Severities()
	fmt.Fprintf(w, "  coerenza: %d errori, %d avvisi, %d note\n", errs, warns, infos)
	for _, f := range r.Findings {
		fmt.Fprintf(w, "    %-6s %s%s: %s\n", f.Severity, f.Code, fieldsSuffix(f.Fields), f.Text)
	}
}

func fieldsSuffix(fs []string) string {
	if len(fs) == 0 {
		return ""
	}
	return " [" + strings.Join(fs, ", ") + "]"
}

// WriteDir scrive un rigo per immagine e poi la tabella di accuratezza per campo, le famiglie di errore e le
// incoerenze piu' frequenti. Con verbose scrive per ogni immagine anche il dettaglio dei campi (WriteResult).
func WriteDir(w io.Writer, d DirReport, verbose bool) {
	for _, r := range d.Items {
		if verbose {
			WriteResult(w, r)
			fmt.Fprintln(w)
			continue
		}
		errs, warns, _ := r.Severities()
		if !r.HasTruth {
			fmt.Fprintf(w, "%-42s %-24s letti %2d/%d  senza verita'  coerenza E%d A%d\n", r.Name, mrzSummary(r.Sum), r.Sum.Found(), outsum.NFields, errs, warns)
			continue
		}
		c := r.Counts()
		fmt.Fprintf(w, "%-42s %-24s letti %2d/%d  giusti %2d/%-2d  coerenza E%d A%d%s\n", r.Name, mrzSummary(r.Sum), r.Sum.Found(), outsum.NFields,
			c.OK, c.WithTruth(), errs, warns, badFields(r))
	}
	if len(d.Skipped) > 0 {
		fmt.Fprintf(w, "\nsaltati (non sono uscite di libretto): %d\n", len(d.Skipped))
	}
	if len(d.NoTruth) > 0 {
		fmt.Fprintf(w, "senza file di verita': %d (%s)\n", len(d.NoTruth), strings.Join(d.NoTruth, ", "))
	}
	WriteStats(w, d.Stats())
	withMRZ := d.Filter(func(r Result) bool { return r.Readable() && r.MRZValid() })
	without := d.Filter(func(r Result) bool { return r.Readable() && !r.MRZValid() })
	if len(withMRZ.Items) > 0 && len(without.Items) > 0 {
		WriteSplit(w, withMRZ.Stats(), without.Stats())
	}
}

// badFields elenca i campi sbagliati e mancanti di un'immagine, per nome.
func badFields(r Result) string {
	var wrong, missing []string
	for _, fr := range r.Fields {
		switch fr.Status {
		case StatusWrong:
			wrong = append(wrong, Canonical(fr.Field))
		case StatusMissing:
			missing = append(missing, Canonical(fr.Field))
		}
	}
	var parts []string
	if len(wrong) > 0 {
		parts = append(parts, "  SBAGLIATO: "+strings.Join(wrong, ","))
	}
	if len(missing) > 0 {
		parts = append(parts, "  MANCANTE: "+strings.Join(missing, ","))
	}
	return strings.Join(parts, "")
}

// WriteStats scrive la tabella di accuratezza per campo, le famiglie di errore e le incoerenze.
func WriteStats(w io.Writer, a Stats) {
	fmt.Fprintf(w, "\nACCURATEZZA PER CAMPO (%d immagini con pagina dati leggibile, %d senza pagina dati; \"con verita'\" = giusti + sbagliati + mancanti)\n", a.Images, a.NoData)
	fmt.Fprintf(w, "%-16s %7s %9s %9s %12s %9s %7s %13s %7s\n", "campo", "giusti", "sbagliati", "mancanti", "con verita'", "giusti %", "letti", "non stampati", "in piu'")
	for _, s := range a.Fields {
		fmt.Fprintf(w, "%-16s %7d %9d %9d %12d %9s %7d %13d %7d\n", Canonical(s.Field), s.OK, s.Wrong, s.Missing, s.WithTruth(), pct(s.OK, s.WithTruth()), s.Found, s.NotPrinted, s.Extra)
	}
	t := a.Total
	found := 0
	for _, s := range a.Fields {
		found += s.Found
	}
	fmt.Fprintf(w, "%-16s %7d %9d %9d %12d %9s %7d %13d %7d\n", "TOTALE", t.OK, t.Wrong, t.Missing, t.WithTruth(), pct(t.OK, t.WithTruth()), found, t.NotPrinted, t.Extra)
	fmt.Fprintln(w, "(letti = campi con un valore scritto dal programma, la vecchia metrica \"campi trovati\"; giusti = letti e uguali alla verita')")
	if n := a.NamesOK + a.NamesWrong + a.NamesMissing; n > 0 {
		fmt.Fprintf(w, "riga 1 dell'MRZ (cognome e nomi contro la riga stampata), %d immagini con la riga in verita': giusti %d, sbagliati %d, senza MRZ %d\n",
			n, a.NamesOK, a.NamesWrong, a.NamesMissing)
	}

	any := false
	for _, s := range a.Fields {
		if len(s.Kinds) > 0 || s.MRZFixes > 0 || s.MRZNoFix > 0 || s.MRZAbsent > 0 {
			any = true
		}
	}
	if any {
		fmt.Fprintln(w, "\nFAMIGLIE DI ERRORE (campi sbagliati) E CORREZIONE CON L'MRZ (campi sbagliati o mancanti che stanno nell'MRZ)")
		for _, s := range a.Fields {
			if len(s.Kinds) == 0 && s.MRZFixes == 0 && s.MRZNoFix == 0 && s.MRZAbsent == 0 {
				continue
			}
			var kinds []string
			for k, n := range s.Kinds {
				kinds = append(kinds, fmt.Sprintf("%s %d", k, n))
			}
			sort.Strings(kinds)
			line := fmt.Sprintf("%-16s %s", Canonical(s.Field), strings.Join(kinds, "; "))
			if s.MRZFixes > 0 || s.MRZNoFix > 0 || s.MRZAbsent > 0 {
				line += fmt.Sprintf("  [MRZ corregge %d, non corregge %d, MRZ assente %d]", s.MRZFixes, s.MRZNoFix, s.MRZAbsent)
			}
			fmt.Fprintln(w, strings.TrimRight(line, " "))
		}
	}

	if len(a.Findings) > 0 {
		fmt.Fprintln(w, "\nINCOERENZE (in quante immagini compare ogni codice)")
		codes := make([]string, 0, len(a.Findings))
		for c := range a.Findings {
			codes = append(codes, c)
		}
		sort.Slice(codes, func(i, j int) bool {
			si, sj := a.Severity[codes[i]], a.Severity[codes[j]]
			if si != sj {
				return si > sj
			}
			if a.Findings[codes[i]] != a.Findings[codes[j]] {
				return a.Findings[codes[i]] > a.Findings[codes[j]]
			}
			return codes[i] < codes[j]
		})
		for _, c := range codes {
			fmt.Fprintf(w, "  %-6s %-34s %d\n", a.Severity[c], c, a.Findings[c])
		}
	}
}

// WriteSplit confronta l'accuratezza per campo delle immagini con MRZ valido (5 check digit su 5) con quella delle
// altre: dice quanto del risultato dipende dal trovare l'MRZ.
func WriteSplit(w io.Writer, with, without Stats) {
	fmt.Fprintf(w, "\nCON E SENZA MRZ VALIDO (giusti/con verita'); immagini: %d con MRZ valido, %d senza\n", with.Images, without.Images)
	fmt.Fprintf(w, "%-16s %18s %18s\n", "campo", "con MRZ valido", "senza MRZ valido")
	cell := func(s FieldStats) string {
		return fmt.Sprintf("%d/%d %s", s.OK, s.WithTruth(), pct(s.OK, s.WithTruth()))
	}
	for i := range with.Fields {
		fmt.Fprintf(w, "%-16s %18s %18s\n", Canonical(with.Fields[i].Field), cell(with.Fields[i]), cell(without.Fields[i]))
	}
	fmt.Fprintf(w, "%-16s %18s %18s\n", "TOTALE", fmt.Sprintf("%d/%d %s", with.Total.OK, with.Total.WithTruth(), pct(with.Total.OK, with.Total.WithTruth())),
		fmt.Sprintf("%d/%d %s", without.Total.OK, without.Total.WithTruth(), pct(without.Total.OK, without.Total.WithTruth())))
}

func pct(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(d))
}

// canonicalAll restituisce i nomi di campo con il nome attuale (certificate al posto del vecchio endorsement).
func canonicalAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = Canonical(n)
	}
	return out
}
