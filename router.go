// Package treemux is forked from [github.com/dimfeld/httptreemux].
//
// `httptreemux` is inspired by Julien Schmidt's httprouter, in that it uses a patricia tree, but the
// implementation is rather different. Specifically, the routing rules are relaxed so that a
// single path segment may be a wildcard in one route and a static token in another. This gives a
// nice combination of high performance with a lot of convenience in designing the routing patterns.
package treemux

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type PanicHandler func(http.ResponseWriter, *http.Request, interface{})

// RedirectBehavior sets the behavior when the router redirects the request to the
// canonical version of the requested URL using RedirectTrailingSlash or RedirectClean.
// The default behavior is to return a 301 status, redirecting the browser to the version
// of the URL that matches the given pattern.
//
// On a POST request, most browsers that receive a 301 will submit a GET request to
// the redirected URL, meaning that any data will likely be lost. If you want to handle
// and avoid this behavior, you may use Redirect307, which causes most browsers to
// resubmit the request using the original method and request body.
//
// Since 307 is supposed to be a temporary redirect, the new 308 status code has been
// proposed, which is treated the same, except it indicates correctly that the redirection
// is permanent. The big caveat here is that the RFC is relatively recent, and older
// browsers will not know what to do with it. Therefore its use is not recommended
// unless you really know what you're doing.
//
// Finally, the UseHandler value will simply call the handler function for the pattern.
type RedirectBehavior int

type PathSource int

const (
	Redirect301 RedirectBehavior = iota // Return 301 Moved Permanently
	Redirect307                         // Return 307 HTTP/1.1 Temporary Redirect
	Redirect308                         // Return a 308 RFC7538 Permanent Redirect
	UseHandler                          // Just call the handler function
)

const (
	RequestURI PathSource = iota // Use r.RequestURI
	URLPath                      // Use r.URL.Path
)

// HandlerConstraint is the type constraint for types that can be used as
// routing target. Typically, users use a function (e.g. [http.HandlerFunc])
// as routing target, but it's not required to be a function, any type
// which satisfies HandlerConstraint is ok.
type HandlerConstraint interface {

	// IsValid tells whether the Handler is valid, a valid Handler which
	// matches the request stops searching the routing rules.
	IsValid() bool
}

// LookupResult contains information about a route lookup, which is returned from Lookup and
// can be passed to ServeLookupResult if the request should be served.
type LookupResult[T HandlerConstraint] struct {
	// StatusCode informs the caller about the result of the lookup.
	// This will generally be `http.StatusNotFound` or `http.StatusMethodNotAllowed`
	// for an error case.
	// On a normal success, the StatusCode will be `http.StatusOK`.
	// A redirect code will also be used in case that redirectPath is not empty.
	StatusCode int

	// Params represents the key value pairs of the path parameters.
	Params map[string]string

	// Non-empty redirectPath indicates that the request should be redirected.
	redirectPath string

	handler      T
	leafHandlers map[string]T // Only has a value when StatusCode is MethodNotAllowed.
}

// TreeMux is a generic HTTP request router.
// It matches the URL of each incoming request against a list of registered
// patterns.
type TreeMux[T HandlerConstraint] struct {
	root  *node[T]
	mutex sync.RWMutex

	Group[T]

	// Optional bridge function to convert Handler to [http.HandlerFunc].
	// This is unnecessary when you don't use http.Handler.
	BridgeFunc HandlerBridge[T]

	// Optional bridge function to convert [http.Handler] middleware to MiddlewareFunc.
	// This is unnecessary when you don't use http.Handler middlewares.
	MiddlewareWrapper MiddlewareBridge[T]

	// The default PanicHandler just returns a 500 code.
	PanicHandler PanicHandler

	// The default NotFoundHandler is http.NotFound.
	NotFoundHandler func(w http.ResponseWriter, r *http.Request)

	// Any OPTIONS request that matches a path without its own OPTIONS handler will use this handler,
	// if set, instead of calling MethodNotAllowedHandler.
	OptionsHandler T

	// MethodNotAllowedHandler is called when a pattern matches, but that
	// pattern does not have a handler for the requested method. The default
	// handler just writes the status code http.StatusMethodNotAllowed and adds
	// the required "Allowed" header.
	// The methods parameter contains the map of each method to the corresponding
	// handler function.
	MethodNotAllowedHandler func(w http.ResponseWriter, r *http.Request,
		methods map[string]T)

	// HeadCanUseGet allows the router to use the GET handler to respond to
	// HEAD requests if no explicit HEAD handler has been added for the
	// matching pattern. This is true by default.
	HeadCanUseGet bool

	// RedirectCleanPath allows the router to try clean the current request path,
	// if no handler is registered for it. This is true by default.
	RedirectCleanPath bool

	// RedirectTrailingSlash enables automatic redirection in case router doesn't find a matching route
	// for the current request path but a handler for the path with or without the trailing
	// slash exists. This is true by default.
	RedirectTrailingSlash bool

	// RemoveCatchAllTrailingSlash removes the trailing slash when a catch-all pattern
	// is matched, if set to true. By default, catch-all paths are never redirected.
	RemoveCatchAllTrailingSlash bool

	// RedirectBehavior sets the default redirect behavior when RedirectTrailingSlash or
	// RedirectCleanPath are true. The default value is Redirect301.
	RedirectBehavior RedirectBehavior

	// RedirectMethodBehavior overrides the default behavior for a particular HTTP method.
	// The key is the method name, and the value is the behavior to use for that method.
	RedirectMethodBehavior map[string]RedirectBehavior

	// PathSource determines from where the router gets its path to search.
	// By default, it pulls the data from the RequestURI member, but this can
	// be overridden to use URL.Path instead.
	//
	// There is a small tradeoff here. Using RequestURI allows the router to handle
	// encoded slashes (i.e. %2f) in the URL properly, while URL.Path provides
	// better compatibility with some utility functions in the http
	// library that modify the Request before passing it to the router.
	PathSource PathSource

	// EscapeAddedRoutes controls URI escaping behavior when adding a route to the tree.
	// If set to true, the router will add both the route as originally passed, and
	// a version passed through URL.EscapedPath. This behavior is disabled by default.
	EscapeAddedRoutes bool

	// If present, override the default context with this one.
	DefaultContext context.Context

	// SafeAddRoutesWhileRunning tells the router to protect all accesses to the tree with an RWMutex. This is only needed
	// if you are going to add routes after the router has already begun serving requests. There is a potential
	// performance penalty at high load.
	SafeAddRoutesWhileRunning bool

	// CaseInsensitive determines if routes should be treated as case-insensitive.
	CaseInsensitive bool
}

// Dump returns a text representation of the routing tree.
func (t *TreeMux[_]) Dump() string {
	return t.root.dumpTree("", "")
}

func (t *TreeMux[_]) serveHTTPPanic(w http.ResponseWriter, r *http.Request) {
	if err := recover(); err != nil {
		t.PanicHandler(w, r, err)
	}
}

func (t *TreeMux[_]) redirectStatusCode(method string) (int, bool) {
	var behavior RedirectBehavior
	var ok bool
	if behavior, ok = t.RedirectMethodBehavior[method]; !ok {
		behavior = t.RedirectBehavior
	}
	switch behavior {
	case Redirect301:
		return http.StatusMovedPermanently, true
	case Redirect307:
		return http.StatusTemporaryRedirect, true
	case Redirect308:
		// Go doesn't have a constant for this yet. Yet another sign
		// that you probably shouldn't use it.
		return 308, true
	case UseHandler:
		return 0, false
	default:
		return http.StatusMovedPermanently, true
	}
}

func (t *TreeMux[T]) lookup(w http.ResponseWriter, r *http.Request) (result LookupResult[T], found bool) {

	result.StatusCode = http.StatusNotFound
	path := r.RequestURI
	unescapedPath := r.URL.Path
	pathLen := len(path)
	if pathLen > 0 && t.PathSource == RequestURI {
		rawQueryLen := len(r.URL.RawQuery)

		if rawQueryLen != 0 || path[pathLen-1] == '?' {
			// Remove any query string and the ?.
			path = path[:pathLen-rawQueryLen-1]
			pathLen = len(path)
		}
	} else {
		// In testing with http.NewRequest,
		// RequestURI is not set so just grab URL.Path instead.
		path = r.URL.Path
		pathLen = len(path)
	}
	if t.CaseInsensitive {
		path = strings.ToLower(path)
		unescapedPath = strings.ToLower(unescapedPath)
	}

	trailingSlash := path[pathLen-1] == '/' && pathLen > 1
	if trailingSlash && t.RedirectTrailingSlash {
		path = path[:pathLen-1]
		unescapedPath = unescapedPath[:len(unescapedPath)-1]
	}

	n, handler, params := t.root.search(r.Method, path[1:])
	if n == nil {
		if t.RedirectCleanPath {
			// Path was not found. Try cleaning it up and search again.
			// TODO Test this
			cleanPath := Clean(unescapedPath)
			n, handler, params = t.root.search(r.Method, cleanPath[1:])
			if n == nil {
				// Still nothing found.
				return
			}
			if statusCode, ok := t.redirectStatusCode(r.Method); ok {
				// Redirect to the actual path
				result.StatusCode = statusCode
				result.redirectPath = cleanPath
				found = true
				return
			}
		} else {
			// Not found.
			return
		}
	}

	if !handler.IsValid() {
		if r.Method == "OPTIONS" && t.OptionsHandler.IsValid() {
			handler = t.OptionsHandler
		}

		if !handler.IsValid() {
			result.leafHandlers = n.leafHandlers
			result.StatusCode = http.StatusMethodNotAllowed
			return
		}
	}

	if !n.isCatchAll || t.RemoveCatchAllTrailingSlash {
		if trailingSlash != n.addSlash && t.RedirectTrailingSlash {
			if statusCode, ok := t.redirectStatusCode(r.Method); ok {
				if n.addSlash {
					result.StatusCode = statusCode
					result.redirectPath = unescapedPath + "/"
				} else if path != "/" {
					result.StatusCode = statusCode
					result.redirectPath = unescapedPath
				}
				if result.redirectPath != "" {
					found = true
					return
				}
			}
		}
	}

	var paramMap map[string]string
	if len(params) != 0 {
		numParams := len(params)
		paramMap = make(map[string]string, numParams)
		if n.isRegex {
			for i, name := range n.regExpr.SubexpNames() {
				if i > 0 && name != "" {
					paramMap[name] = params[i]
				}
			}
		} else {
			if numParams != len(n.leafParamNames) {
				// Need better behavior here. Should this be a panic?
				panic(fmt.Sprintf("treemux: parameter list length mismatch: %v, %v",
					params, n.leafParamNames))
			}

			for index := 0; index < numParams; index++ {
				paramMap[n.leafParamNames[numParams-index-1]] = params[index]
			}
		}
	}

	result = LookupResult[T]{
		StatusCode: http.StatusOK,
		Params:     paramMap,
		handler:    handler,
	}
	found = true
	return
}

// Lookup performs a lookup without actually serving the request or mutating the request or response.
// The return values are a LookupResult and a boolean. The boolean will be true when a handler
// was found or the lookup resulted in a redirect which will point to a real handler. It is false
// for requests which would result in a `StatusNotFound` or `StatusMethodNotAllowed`.
//
// Regardless of the returned boolean's value, the LookupResult may be passed to ServeLookupResult
// to be served appropriately.
func (t *TreeMux[T]) Lookup(w http.ResponseWriter, r *http.Request) (LookupResult[T], bool) {
	if t.SafeAddRoutesWhileRunning {
		// In concurrency safe mode, we acquire a read lock on the mutex for any access.
		// This is optional to avoid potential performance loss in high-usage scenarios.
		t.mutex.RLock()
	}

	result, found := t.lookup(w, r)

	if t.SafeAddRoutesWhileRunning {
		t.mutex.RUnlock()
	}

	return result, found
}

// MethodNotAllowedHandler is the default handler for TreeMux.MethodNotAllowedHandler,
// which is called for patterns that match, but do not have a handler installed for the
// requested method. It simply writes the status code http.StatusMethodNotAllowed and fills
// in the `Allow` header value appropriately.
func MethodNotAllowedHandler[T HandlerConstraint](w http.ResponseWriter, r *http.Request,
	methods map[string]T) {

	for m := range methods {
		w.Header().Add("Allow", m)
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func New[T HandlerConstraint]() *TreeMux[T] {
	tm := &TreeMux[T]{
		root:                    &node[T]{path: "/"},
		NotFoundHandler:         http.NotFound,
		MethodNotAllowedHandler: MethodNotAllowedHandler[T],
		HeadCanUseGet:           true,
		RedirectTrailingSlash:   true,
		RedirectCleanPath:       true,
		RedirectBehavior:        Redirect301,
		RedirectMethodBehavior:  make(map[string]RedirectBehavior),
		PathSource:              RequestURI,
		EscapeAddedRoutes:       false,
	}
	tm.Group.mux = tm
	setDefaultBridgeFunctions(tm)
	return tm
}
