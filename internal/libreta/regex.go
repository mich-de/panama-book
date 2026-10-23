package libreta

import "regexp"

var reCache = map[string]*regexp.Regexp{}

func regexpCache(p string) *regexp.Regexp {
	if re, ok := reCache[p]; ok {
		return re
	}
	re := regexp.MustCompile(p)
	reCache[p] = re
	return re
}
