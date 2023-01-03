package treemux

import (
	"fmt"
	"regexp"
	"strings"
	"unsafe"
)

// RewriteFunc is a function which rewrites an url to another one.
type RewriteFunc func(string) string

func noopRewrite(path string) string {
	return path
}

// NewRewriteFunc creates a new RewriteFunc to rewrite path.
// The returned function checks its input to match path, if the input
// does not match path, the input is returned unmodified, else it
// rewrites the input using the pattern specified by rewrite.
func NewRewriteFunc(path, rewrite string) (RewriteFunc, error) {
	if rewrite == "" {
		return noopRewrite, nil
	}
	hasRegexVar := strings.Contains(rewrite, "$")
	hasNamedVar := strings.Contains(rewrite, ":")
	impl := &rewriteImpl{
		path:        path,
		rewrite:     rewrite,
		hasRegexVar: hasRegexVar,
		hasNamedVar: hasNamedVar,
	}
	err := impl.parsePath()
	if err != nil {
		return nil, fmt.Errorf("cannot parse path to re: %w", err)
	}
	return impl.Rewrite, nil
}

type rewriteImpl struct {
	path    string
	rewrite string

	hasRegexVar bool
	hasNamedVar bool

	re *regexp.Regexp
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

	to := p.rewrite
	match := p.re.FindStringSubmatchIndex(path)
	if len(match) == 0 {
		return path
	}
	if p.hasRegexVar {
		tmp := make([]byte, 0, len(path)+16)
		tmp = p.re.ExpandString(tmp, to, path, match)
		to = *(*string)(unsafe.Pointer(&tmp))
	}
	if p.hasNamedVar {
		for i, iname := range p.re.SubexpNames() {
			if iname == "" {
				continue
			}
			if 2*i+1 < len(match) && match[2*i] >= 0 {
				placeholder := ":" + iname
				value := path[match[2*i]:match[2*i+1]]
				to = strings.Replace(to, placeholder, value, -1)
			}
		}
	}
	return to
}
