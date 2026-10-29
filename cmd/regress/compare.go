package main

// Confronto tra una prova e la baseline. Ogni dimensione (MRZ, check digit, campi letti, campi giusti) e' valutata
// per nome: perdere anche un solo elemento e' una REGRESSIONE, anche se un altro intanto migliora.

import (
	"fmt"
	"slices"
	"strings"

	"scanlibretto/internal/outsum"
)

// Tipi di segnalazione.
const (
	kindRegression  = "REGRESSIONE"
	kindImprovement = "MIGLIORAMENTO"
	kindNote        = "NOTA" // informativa: non fa fallire il confronto
)

// Soglie delle note sul tempo (il tempo dipende dal carico della macchina: non e' mai una regressione).
const (
	slowRatio   = 1.5
	slowMinDiff = 4.0 // secondi
)

// Finding e' una segnalazione su una immagine.
type Finding struct {
	Kind, File, Text string
}

// Outcome riassume il confronto, contando le immagini.
type Outcome struct {
	Findings  []Finding
	Regressed int // immagini con almeno una REGRESSIONE
	Improved  int // immagini con almeno un MIGLIORAMENTO
	Same      int // immagini senza regressioni ne' miglioramenti
	New       int // nella prova ma non nella baseline
	Absent    int // nella baseline ma non nella prova
}

// compare confronta la prova (cur) con la baseline (base). L'ordine delle segnalazioni segue cur.
func compare(base, cur []Row) Outcome {
	byFile := make(map[string]Row, len(base))
	for _, b := range base {
		byFile[b.File] = b
	}
	var out Outcome
	seen := make(map[string]bool, len(cur))
	for _, c := range cur {
		seen[c.File] = true
		b, ok := byFile[c.File]
		if !ok {
			out.New++
			continue
		}
		fs := compareRow(b, c)
		reg, imp := false, false
		for _, f := range fs {
			reg = reg || f.Kind == kindRegression
			imp = imp || f.Kind == kindImprovement
		}
		switch {
		case reg && imp:
			out.Regressed++
			out.Improved++
		case reg:
			out.Regressed++
		case imp:
			out.Improved++
		default:
			out.Same++
		}
		out.Findings = append(out.Findings, fs...)
	}
	for _, b := range base {
		if !seen[b.File] {
			out.Absent++
		}
	}
	return out
}

// compareRow confronta una immagine. Restituisce al piu' una segnalazione per tipo.
func compareRow(b, c Row) []Finding {
	switch {
	case b.Err && c.Err:
		return nil
	case !b.Err && c.Err:
		return []Finding{{kindRegression, c.File, "la prova ora fallisce (prima dava un risultato)"}}
	case b.Err && !c.Err:
		return []Finding{{kindImprovement, c.File, "la prova ora riesce (prima falliva)"}}
	}
	var reg, imp, note []string

	switch {
	case b.MRZ && !c.MRZ:
		reg = append(reg, "MRZ si->no")
	case !b.MRZ && c.MRZ:
		imp = append(imp, "MRZ no->si")
	}
	// nomi della riga 1 dell'MRZ (senza check digit): letti da giusti a sbagliati e' una regressione, il contrario un
	// miglioramento; un MRZ trovato per la prima volta con i nomi sbagliati si nota soltanto
	switch bs, cs := b.nameSym(), c.nameSym(); {
	case bs == symRight && cs == symWrong:
		reg = append(reg, "nomi MRZ (riga 1) da giusti a sbagliati")
	case bs == symWrong && cs == symRight:
		imp = append(imp, "nomi MRZ (riga 1) da sbagliati a giusti")
	case bs == symMissing && cs == symWrong:
		note = append(note, "MRZ trovato con i nomi sbagliati")
	}
	addDelta := func(label string, bm, cm string, good byte, names []string, bn, cn, total int, filter func(lost, gained []string) ([]string, []string)) {
		lost, gained := delta(bm, cm, good, names)
		if filter != nil {
			lost, gained = filter(lost, gained)
		}
		if len(lost) > 0 {
			reg = append(reg, fmt.Sprintf("%s %d/%d->%d/%d (persi: %s)", label, bn, total, cn, total, strings.Join(lost, ",")))
		}
		if len(gained) > 0 {
			imp = append(imp, fmt.Sprintf("%s %d/%d->%d/%d (guadagnati: %s)", label, bn, total, cn, total, strings.Join(gained, ",")))
		}
	}
	addDelta("check", b.CheckMap, c.CheckMap, symOK, outsum.CheckKeys[:], b.checksOK(), c.checksOK(), outsum.NChecks, nil)
	addDelta("campi", b.FieldMap, c.FieldMap, symOK, outsum.FieldKeys[:], b.found(), c.found(), outsum.NFields, senzaSpuri(b, c))
	if b.Truth && c.Truth {
		bn, bm := b.right()
		cn, cm := c.right()
		lost, gained := delta(b.TruthMap, c.TruthMap, symRight, outsum.FieldKeys[:])
		if len(lost) > 0 {
			reg = append(reg, fmt.Sprintf("giusti %d/%d->%d/%d (da giusti a sbagliati o mancanti: %s)", bn, bm, cn, cm, strings.Join(lost, ",")))
		}
		if len(gained) > 0 {
			imp = append(imp, fmt.Sprintf("giusti %d/%d->%d/%d (ora giusti: %s)", bn, bm, cn, cm, strings.Join(gained, ",")))
		}
		if _, added := delta(b.TruthMap, c.TruthMap, symSpurious, outsum.FieldKeys[:]); len(added) > 0 {
			note = append(note, "campi letti ma vuoti in verita': "+strings.Join(added, ","))
		}
		// un valore sbagliato e' peggio di un campo mancante (il vuoto si nota, l'errore no): campi che ora sono
		// sbagliati e prima non erano giusti (quelli che erano giusti sono gia' nella regressione sopra)
		if _, wrong := delta(b.TruthMap, c.TruthMap, symWrong, outsum.FieldKeys[:]); len(wrong) > 0 {
			var fresh []string
			for _, w := range wrong {
				if !slices.Contains(lost, w) {
					fresh = append(fresh, w)
				}
			}
			if len(fresh) > 0 {
				note = append(note, "campi ora sbagliati (prima mancanti o non confrontabili): "+strings.Join(fresh, ","))
			}
		}
	}

	if b.MRZ && c.MRZ && b.Rotation != c.Rotation {
		note = append(note, fmt.Sprintf("rotazione %d->%d", b.Rotation, c.Rotation))
	}
	if b.Warnings != c.Warnings {
		note = append(note, fmt.Sprintf("avvisi %d->%d", b.Warnings, c.Warnings))
	}
	if c.Seconds > slowRatio*b.Seconds && c.Seconds-b.Seconds >= slowMinDiff {
		note = append(note, fmt.Sprintf("tempo %.1f->%.1f s", b.Seconds, c.Seconds))
	}

	var out []Finding
	for _, g := range []struct {
		kind  string
		items []string
	}{{kindRegression, reg}, {kindImprovement, imp}, {kindNote, note}} {
		if len(g.items) > 0 {
			out = append(out, Finding{g.kind, c.File, strings.Join(g.items, "; ")})
		}
	}
	return out
}

// senzaSpuri restituisce, se entrambe le righe hanno la verita', il filtro che toglie dai campi letti persi e guadagnati
// quelli che la verita' dice vuoti (letti per errore, per esempio su una copertina o su una striscia MRZ senza pagina
// dati): perdere un valore inventato non e' una regressione e guadagnarne uno non e' un miglioramento. Quei campi
// restano visibili nelle note ("campi letti ma vuoti in verita'").
func senzaSpuri(b, c Row) func(lost, gained []string) ([]string, []string) {
	if !b.Truth || !c.Truth {
		return nil
	}
	return func(lost, gained []string) ([]string, []string) {
		return dropSpurious(lost, b.TruthMap), dropSpurious(gained, c.TruthMap)
	}
}

// dropSpurious toglie i nomi dei campi che nella mappa della verita' hanno il simbolo X (letto ma vuoto in verita').
func dropSpurious(names []string, truthMap string) []string {
	var out []string
	for _, n := range names {
		if i := slices.Index(outsum.FieldKeys[:], n); i >= 0 && i < len(truthMap) && truthMap[i] == symSpurious {
			continue
		}
		out = append(out, n)
	}
	return out
}

// delta confronta due mappe di simboli: lost sono i nomi (names[i]) con simbolo good in a ma non in b,
// gained quelli con simbolo good in b ma non in a.
func delta(a, b string, good byte, names []string) (lost, gained []string) {
	for i := 0; i < len(a) && i < len(b) && i < len(names); i++ {
		switch {
		case a[i] == good && b[i] != good:
			lost = append(lost, names[i])
		case a[i] != good && b[i] == good:
			gained = append(gained, names[i])
		}
	}
	return lost, gained
}
