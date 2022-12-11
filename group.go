package treemux

import (
	"fmt"
	"net/url"
	"strings"
)

type MiddlewareFunc[T HandlerConstraint] func(next T) T

func withMiddlewares[T HandlerConstraint](handler T, stack []MiddlewareFunc[T]) T {
	for i := len(stack) - 1; i >= 0; i-- {
		handler = stack[i](handler)
	}
	return handler
}

type Group[T HandlerConstraint] struct {
	path  string
	mux   *TreeMux[T]
	stack []MiddlewareFunc[T]
}

// NewGroup adds a new sub-group to this group.
func (g *Group[T]) NewGroup(path string) *Group[T] {
	if len(path) < 1 {
		panic("treemux: group path must not be empty")
	}

	checkPath(path)
	path = g.path + path
	//Don't want trailing slash as all sub-paths start with slash
	if path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return &Group[T]{
		path:  path,
		mux:   g.mux,
		stack: g.stack[:len(g.stack):len(g.stack)],
	}
}

// Use appends a middleware handler to the Group middleware stack.
func (g *Group[T]) Use(fn MiddlewareFunc[T]) {
	g.stack = append(g.stack, fn)
}

// Handle adds routing rules to Group.
//
// Path elements starting with : indicate a wildcard in the path. A wildcard will only match on a
// single path segment. That is, the pattern `/post/:postid` will match on `/post/1` or `/post/1/`,
// but not `/post/1/2`.
//
// A path element starting with * is a catch-all, whose value will be a string containing all text
// in the URL matched by the wildcards. For example, with a pattern of `/images/*path` and a
// requested URL `images/abc/def`, path would contain `abc/def`.
//
// # Routing Rule Priority
//
// The priority rules in the router are simple.
//
// 1. Static path segments take the highest priority. If a segment and its subtree are able to match the URL, that match is returned.
//
// 2. Wildcards take second priority. For a particular wildcard to match, that wildcard and its subtree must match the URL.
//
// 3. Regexp routes are checked after static and wildcards routes. Multiple regexp routes under a same prefix are checked in the registering order, if a regexp route matches the URL, the match is returned. Regular expression must be at the end of a pattern.
//
// 4. Finally, a catch-all rule will match when the earlier path segments have matched, and none of above rules have matched. Catch-all rules must be at the end of a pattern.
//
// So with the following patterns, we'll see certain matches:
//
//	router = treemux.New[MyHandler]()
//	router.GET("/:page", pageHandler)
//	router.GET("/:year/:month/:post", postHandler)
//	router.GET("/:year/:month", archiveHandler)
//	router.GET(`/images/~^(?P<category>\w+)-(?P<name>.+)$`)
//	router.GET("/images/*path", staticHandler)
//	router.GET("/favicon.ico", staticHandler)
//
//	/abc will match /:page
//	/2014/05 will match /:year/:month
//	/2014/05/really-great-blog-post will match /:year/:month/:post
//	/images/cate1-Img1.jpg will match /images/~^(?P<category>\w+)-(?P<name>.+)$, the params will be `category=cate1` and `name=Img1.jpg`
//	/images/CoolImage.gif will match /images/*path
//	/images/2014/05/MayImage.jpg will also match /images/*path, with all the text after /images stored in the variable path.
//	/favicon.ico will match /favicon.ico
//
// # Trailing Slashes
//
// The router has special handling for paths with trailing slashes. If a pattern is added to the
// router with a trailing slash, any matches on that pattern without a trailing slash will be
// redirected to the version with the slash. If a pattern does not have a trailing slash, matches on
// that pattern with a trailing slash will be redirected to the version without.
//
// The trailing slash flag is only stored once for a pattern. That is, if a pattern is added for a
// method with a trailing slash, all other methods for that pattern will also be considered to have a
// trailing slash, regardless of whether it is specified for those methods too.
//
// This behavior can be turned off by setting TreeMux.RedirectTrailingSlash to false.
// By default it is set to true. The specifics of the redirect depend on RedirectBehavior.
//
// One exception to this rule is catch-all patterns. By default, trailing slash redirection is
// disabled on catch-all patterns, since the structure of the entire URL and the desired patterns
// can not be predicted. If trailing slash removal is desired on catch-all patterns, set
// TreeMux.RemoveCatchAllTrailingSlash to true.
//
//	router = treemux.New[MyHandler]()
//	router.GET("/about", pageHandler)
//	router.GET("/posts/", postIndexHandler)
//	router.POST("/posts", postFormHandler)
//
//	GET /about will match normally.
//	GET /about/ will redirect to /about.
//	GET /posts will redirect to /posts/.
//	GET /posts/ will match normally.
//	POST /posts will redirect to /posts/, because the GET method used a trailing slash.
func (g *Group[T]) Handle(method string, path string, handler T) {
	g.mux.mutex.Lock()
	defer g.mux.mutex.Unlock()

	if len(g.stack) > 0 {
		handler = withMiddlewares(handler, g.stack)
	}

	g.addFullStackHandler(method, path, handler)
}

func (g *Group[T]) addFullStackHandler(method string, path string, handler T) {
	addSlash := false
	addOne := func(thePath string) {
		if g.mux.CaseInsensitive {
			thePath = strings.ToLower(thePath)
		}

		node := g.mux.root.addPath(thePath[1:], nil, false)
		if addSlash {
			node.addSlash = true
		}
		node.setHandler(method, handler, false)

		if g.mux.HeadCanUseGet && method == "GET" && !node.leafHandlers["HEAD"].IsValid() {
			node.setHandler("HEAD", handler, true)
		}
	}

	checkPath(path)
	path = g.path + path
	if len(path) == 0 {
		panic("treemux: cannot map an empty path")
	}

	if len(path) > 1 && path[len(path)-1] == '/' && g.mux.RedirectTrailingSlash {
		addSlash = true
		path = path[:len(path)-1]
	}

	if g.mux.EscapeAddedRoutes {
		u, err := url.ParseRequestURI(path)
		if err != nil {
			panic(fmt.Sprintf("treemux: cannot parse URL %s: %v", path, err))
		}
		escapedPath := unescapeSpecial(u.String())

		if escapedPath != path {
			addOne(escapedPath)
		}
	}

	addOne(path)
}

// GET is a shortcut for Handle("GET", path, handler).
func (g *Group[T]) GET(path string, handler T) {
	g.Handle("GET", path, handler)
}

// POST is a shortcut for Handle("POST", path, handler).
func (g *Group[T]) POST(path string, handler T) {
	g.Handle("POST", path, handler)
}

// PUT is a shortcut for Handle("PUT", path, handler).
func (g *Group[T]) PUT(path string, handler T) {
	g.Handle("PUT", path, handler)
}

// DELETE is a shortcut for Handle("DELETE", path, handler).
func (g *Group[T]) DELETE(path string, handler T) {
	g.Handle("DELETE", path, handler)
}

// PATCH is a shortcut for Handle("PATCH", path, handler).
func (g *Group[T]) PATCH(path string, handler T) {
	g.Handle("PATCH", path, handler)
}

// HEAD is a shortcut for Handle("HEAD", path, handler).
func (g *Group[T]) HEAD(path string, handler T) {
	g.Handle("HEAD", path, handler)
}

// OPTIONS is a shortcut for Handle("OPTIONS", path, handler).
func (g *Group[T]) OPTIONS(path string, handler T) {
	g.Handle("OPTIONS", path, handler)
}

func checkPath(path string) {
	// All non-empty paths must start with a slash
	if len(path) > 0 && path[0] != '/' {
		panic(fmt.Sprintf("treemux: path must start with slash"))
	}
}

func unescapeSpecial(s string) string {
	// Look for sequences of \*, *, and \: that were escaped, and undo some of that escaping.

	// Unescape /* since it references a wildcard token.
	s = strings.Replace(s, "/%2A", "/*", -1)

	// Unescape /\: since it references a literal colon
	s = strings.Replace(s, "/%5C:", "/\\:", -1)

	// Replace escaped /\\: with /\:
	s = strings.Replace(s, "/%5C%5C:", "/%5C:", -1)

	// Replace escaped /\* with /*
	s = strings.Replace(s, "/%5C%2A", "/%2A", -1)

	// Replace escaped /\\* with /\*
	s = strings.Replace(s, "/%5C%5C%2A", "/%5C%2A", -1)

	return s
}
