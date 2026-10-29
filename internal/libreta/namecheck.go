package libreta

// Confronto tra il nome stampato (riga "Nombres/Given Names Apellidos/Surname", letto da viz_name.go) e cognome e nomi
// dell'MRZ. La riga 1 dell'MRZ non ha check digit: una lettura storpiata passa inosservata con i cinque check della riga 2
// validi. Il confronto e' per parole e ignora l'ordine, perche' nei libretti di Panama i nomi precedono il cognome. Si
// avvisa solo con una lettura stampata di confidenza alta: un OCR debole del testo libero darebbe falsi allarmi.

import (
	"sort"
	"strings"
)

const (
	// warnNameDiffers e' l'avviso interno; cmd/libretto lo converte in MRZ_NAME_DIFFERS_FROM_PRINTED. Senza valori.
	warnNameDiffers = "nome stampato diverso dall'MRZ"

	// printedNameMinConf e' la confidenza minima della lettura stampata (viz_name.go: 1 = tutte le varianti concordano).
	// Misurata sul corpus (64 immagini, 40 con nome letto): la confidenza dei nomi e' bassa quasi ovunque (mediana 0,5) e
	// non separa bene le letture giuste dalle sbagliate (un caso con 0,88 e' un errore di lettura del nome stampato, con
	// MRZ giusta). Sotto 0,9 gli avvisi sarebbero in maggioranza falsi allarmi; a 0,9 il corpus ne produce zero.
	printedNameMinConf = 0.9

	// mrzNamesFieldLen e' la lunghezza del campo nomi della riga 1 (posizioni 6-44): se e' tutto occupato il nome puo'
	// essere stato troncato dall'MRZ e il confronto non e' possibile.
	mrzNamesFieldLen = 39
)

// foldLetter riporta una lettera con accento alla base ASCII, come fa l'MRZ.
var foldLetter = strings.NewReplacer(
	"Á", "A", "À", "A", "Â", "A", "Ä", "A", "Ã", "A", "Å", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Ö", "O", "Õ", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ñ", "N", "Ç", "C",
)

// nameTokens rende le parole di un nome: maiuscolo, senza accenti, solo lettere A-Z, ordinate.
func nameTokens(parts ...string) []string {
	s := foldLetter.Replace(strings.ToUpper(strings.Join(parts, " ")))
	words := strings.FieldsFunc(s, func(r rune) bool { return r < 'A' || r > 'Z' })
	sort.Strings(words)
	return words
}

// namesDiffer dice se il nome stampato e cognome+nomi dell'MRZ hanno parole diverse (ordine ignorato).
func namesDiffer(printed, surname, given string) bool {
	a, b := nameTokens(printed), nameTokens(surname, given)
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// mrzNamesTruncated e' vero se il campo nomi della riga 1 e' tutto occupato (possibile troncamento).
func mrzNamesTruncated(line1 string) bool {
	if len(line1) < 44 {
		return false
	}
	return len(strings.TrimRight(line1[5:44], "<")) >= mrzNamesFieldLen-1
}

// checkPrintedName aggiunge l'avviso warnNameDiffers se il nome stampato, letto con confidenza sufficiente, non coincide
// con l'MRZ. Va chiamata dopo finalizeVIZ, che aggiorna il nome con il voto finale.
func checkPrintedName(r *Result) {
	if r.MRZ == nil {
		return
	}
	name, conf, ok := vizPrintedName(r)
	if !ok || conf < printedNameMinConf || mrzNamesTruncated(r.MRZ.Line1) {
		return
	}
	if namesDiffer(name, r.MRZ.Surname, r.MRZ.GivenNames) {
		r.Warnings = append(r.Warnings, warnNameDiffers)
	}
}
