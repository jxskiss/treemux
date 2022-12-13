package treemux

import (
	"net/http"
	"net/url"
	"regexp"
	"sort"
)

func getRegexParamNames(re *regexp.Regexp) (ret []string) {
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			ret = append(ret, name)
		}
	}
	return
}

// Note that the returned slice is in reverse order.
func getRegexMatchParams(re *regexp.Regexp, match []string) (ret []string) {
	ret = match[:0]
	for i, name := range re.SubexpNames() {
		if i > 0 && name != "" {
			ret = append(ret, match[i])
		}
	}
	reverseSlice(ret)
	return
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

func redirect(w http.ResponseWriter, r *http.Request, newPath string, statusCode int) {
	newURL := url.URL{
		Path:     newPath,
		RawQuery: r.URL.RawQuery,
		Fragment: r.URL.Fragment,
	}
	http.Redirect(w, r, newURL.String(), statusCode)
}

func reverseSlice[S []E, E any](s S) {
	for i := 0; i < len(s)/2; i++ {
		j := len(s) - i - 1
		s[i], s[j] = s[j], s[i]
	}
}

func unescape(path string) (string, error) {
	return url.PathUnescape(path)
}
