//go:build go1.8
// +build go1.8

package treemux

import "net/url"

func unescape(path string) (string, error) {
	return url.PathUnescape(path)
}
