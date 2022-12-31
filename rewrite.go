package treemux

import (
	"fmt"
	"regexp"
	"strings"
	"unsafe"
)

// RewriteFunc is a function which rewrites an url to another one.
type RewriteFunc func(string) string

// NewRewriteFunc creates a new RewriteFunc to rewrite path.
func NewRewriteFunc(path, rewrite string) (RewriteFunc, error) {
	impl := &rewriteImpl{
		path:    path,
		rewrite: rewrite,
	}
	err := impl.parsePath()
	if err != nil {
		return nil, fmt.Errorf("cannot parse path to re: %w", err)
	}
	impl.hasRegexVar = strings.Contains(rewrite, "$")
	impl.hasNamedVar = strings.Contains(rewrite, ":")
	return impl.Rewrite, nil
}

type rewriteImpl struct {
	path    string
	rewrite string

	re *regexp.Regexp

	hasRegexVar bool
	hasNamedVar bool
}

func (p *rewriteImpl) parsePath() (err error) {
	path := Clean(p.path)
	if path == "" || path[0] != '/' {
		return nil
	}

	getName := func(path string, nextSlash int) string {
		if nextSlash < 0 {
			return path
		}
		return path[:nextSlash]
	}

	rePattern := "^"
	for path != "" {
		c := path[1]
		nextSlash := strings.Index(path[1:], "/")
		rePattern += "/"
		if c == ':' {
			name := getName(path[2:], nextSlash-1)
			rePattern += fmt.Sprintf(`(?P<%s>[^/#?]+)`, name)
		} else if c == '*' {
			name := path[2:]
			rePattern += fmt.Sprintf("(?P<%s>.+)", name)
			break
		} else if c == '~' {
			re := path[2:]
			rePattern += removeRegexBeginEnd(re)
			break
		} else {
			name := getName(path[1:], nextSlash)
			if strings.HasPrefix(name, `\\`) {
				name = name[1:]
			} else if len(name) >= 2 {
				if name[0] == '\\' && (name[1] == ':' || name[1] == '*' || name[1] == '^') {
					name = name[1:]
				}
			}
			name = regexp.QuoteMeta(name)
			rePattern += name
		}
		if nextSlash < 0 {
			break
		}
		path = path[1+nextSlash:]
	}
	rePattern += "$"
	p.re, err = regexp.Compile(rePattern)
	return
}

func removeRegexBeginEnd(re string) string {
	if strings.HasPrefix(re, "^") {
		re = re[1:]
	}
	if strings.HasPrefix(re, "(^") {
		re = "(" + re[2:]
	}
	if strings.HasSuffix(re, "$") {
		re = re[:len(re)-1]
	}
	if strings.HasPrefix(re, "$)") {
		re = re[:len(re)-2] + ")"
	}
	return re
}

func (p *rewriteImpl) Rewrite(path string) string {
	if p.re == nil {
		return p.rewrite
	}
	if !p.re.MatchString(path) {
		return path
	}

	to := p.rewrite
	if p.hasRegexVar {
		to = p.replaceREParams(path, to)
	}
	if p.hasNamedVar {
		to = p.replaceNamedParams(path, to)
	}
	return to
}

func (p *rewriteImpl) replaceREParams(path, to string) string {
	match := p.re.FindStringSubmatchIndex(path)
	result := make([]byte, 0, len(path)+16)
	result = p.re.ExpandString(result, to, path, match)
	return *(*string)(unsafe.Pointer(&result))
}

func (p *rewriteImpl) replaceNamedParams(from, to string) string {
	fromMatches := p.re.FindStringSubmatch(from)
	if len(fromMatches) > 0 {
		for i, name := range p.re.SubexpNames() {
			if len(name) > 0 {
				to = strings.Replace(to, ":"+name, fromMatches[i], -1)
			}
		}
	}
	return to
}
