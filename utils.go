package treemux

import (
	"net/url"
	"regexp"
	"sort"
)

func unescape(path string) (string, error) {
	return url.PathUnescape(path)
}

func getSortedKeys[M ~map[string]V, V any](m M) (ret []string) {
	for k := range m {
		ret = append(ret, k)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i] < ret[j]
	})
	return
}

func getRegexParamsNames(re *regexp.Regexp) (ret []string) {
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			ret = append(ret, name)
		}
	}
	return
}
