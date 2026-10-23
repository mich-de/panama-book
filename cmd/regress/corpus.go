package main

// Corpus di immagini su cui gira l'harness. Le immagini si leggono soltanto: l'harness non scrive mai in queste cartelle.

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// corpusRule dice quali file di una cartella (non ricorsiva) entrano nel corpus. I pattern sono in minuscolo e si
// confrontano col nome del file in minuscolo (path.Match).
type corpusRule struct {
	dir     string   // relativa alla radice, con '/'
	include []string // nomi da prendere
	exclude []string // nomi da scartare
}

// corpusRules: le scansioni reali, le prove WIA vecchie e i facsimili. Le pagine dati ritagliate (-pagina*.png) non
// sono scansioni ma ritagli di una scansione gia' presente. Le sottocartelle (debug incluse) non si visitano.
var corpusRules = []corpusRule{
	{"scans", []string{"*.png"}, []string{"*-pagina*.png"}},
	{"scans/vecchi test WIA", []string{"*.png"}, []string{"*-pagina*.png"}},
	{"facsimili/panama", []string{"*.jpg", "*.png"}, nil},
}

// discover elenca le immagini del corpus come percorsi relativi a root (con '/'), in ordine alfabetico.
// Una cartella assente non e' un errore: il corpus e' solo piu' piccolo.
func discover(root string) ([]string, error) {
	var files []string
	for _, rule := range corpusRules {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rule.dir)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if e.IsDir() || !matchAny(rule.include, name) || matchAny(rule.exclude, name) {
				continue
			}
			files = append(files, rule.dir+"/"+e.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// filterFiles tiene i file il cui percorso contiene una delle parole di only (se non vuoto) e nessuna di quelle di
// skip. Le parole sono separate da virgola e si confrontano senza distinguere maiuscole e minuscole.
func filterFiles(files []string, only, skip string) []string {
	onlyW, skipW := words(only), words(skip)
	var out []string
	for _, f := range files {
		l := strings.ToLower(f)
		if len(onlyW) > 0 && !containsAny(l, onlyW) {
			continue
		}
		if containsAny(l, skipW) {
			continue
		}
		out = append(out, f)
	}
	return out
}

func words(list string) []string {
	var out []string
	for _, w := range strings.Split(strings.ToLower(list), ",") {
		if w = strings.TrimSpace(w); w != "" {
			out = append(out, w)
		}
	}
	return out
}

func containsAny(s string, subs []string) bool {
	for _, w := range subs {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// relToRoot rende un percorso indicato a mano relativo a root (con '/') se sta dentro root; altrimenti lo lascia
// com'e' (con '/'). I percorsi relativi sono gia' intesi rispetto a root.
func relToRoot(root, p string) string {
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(root, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(filepath.Clean(p))
}

// absPath e' il percorso completo di un elemento del corpus.
func absPath(root, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}
