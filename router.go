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

type MethodNotAllowedHandler func(w http.ResponseWriter, r *http.Request, allowedMethods []string)

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

type RouteType int

const (
	Static RouteType = iota
	Wildcard
	Regexp
	CatchAll
)

// HandlerConstraint is the type constraint for a handler,
// any type can be used as a handler target.
type HandlerConstraint = any

// LookupResult contains information about a route lookup, which is returned from Lookup and
// can be passed to [TreeMux.ServeLookupResult] if the request should be served.
type LookupResult[T HandlerConstraint] struct {
	// StatusCode informs the caller about the result of the lookup.
	// This will generally be `http.StatusNotFound` or `http.StatusMethodNotAllowed`
	// for an error case.
	// On a normal success, the StatusCode will be `http.StatusOK`.
	// A redirect code will also be used in case that RedirectPath is not empty.
	StatusCode int

	// Non-empty RedirectPath indicates that the request should be redirected.
	RedirectPath string

	// When StatusCode is `http.StatusMethodNotAllowed`, AllowedMethods contains the
	// methods registered for the request path, else it is nil.
	AllowedMethods []string

	// Params represents the key value pairs of the path parameters.
	Params Params

	// Handler is the result handler if it's found.
	Handler T

	// RoutePath is the route path registered with the result handler.
	RoutePath string

	// When StatusCode is not `http.StatusNotFound`, RouteType is the type
	// of the matched route.
	RouteType RouteType
}

// TreeMux is a generic HTTP request router.
// It matches the URL of each incoming request against a list of registered
// patterns.
type TreeMux[T HandlerConstraint] struct {
	root  *node[T]
	mutex sync.RWMutex

	Group[T]

	// Bridge connects TreeMux to user defined handler type T.
	Bridge Bridge[T]

	// The default PanicHandler just returns a 500 code.
	PanicHandler PanicHandler

	// The default NotFoundHandler is http.NotFound.
	NotFoundHandler http.HandlerFunc

	// Any OPTIONS request that matches a path without its own OPTIONS handler will use this handler,
	// if set, instead of calling MethodNotAllowedHandler.
	OptionsHandler T

	// MethodNotAllowedHandler is called when a pattern matches, but that
	// pattern does not have a handler for the requested method.
	// The default handler just writes the status code
	// http.StatusMethodNotAllowed and adds the required "Allow" header.
	MethodNotAllowedHandler MethodNotAllowedHandler

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

	// UseContextData tells the router to populate router-related data to the context
	// associated with a request.
	UseContextData bool

	// SafeAddRoutesWhileRunning tells the router to protect all accesses to the tree with an RWMutex.
	// This is only needed if you are going to add routes after the router has already begun serving requests.
	// There is a potential performance penalty at high load.
	SafeAddRoutesWhileRunning bool

	// CaseInsensitive determines if routes should be treated as case-insensitive.
	CaseInsensitive bool
}

// Dump returns a text representation of the routing tree.
func (t *TreeMux[_]) Dump() string {
	return t.root.dumpTree("", "")
}

func (t *TreeMux[T]) setDefaultRequestContext(r *http.Request) *http.Request {
	if t.DefaultContext != nil {
		r = r.WithContext(t.DefaultContext)
	}
	return r
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

func (t *TreeMux[T]) lookup(method, requestURI, urlPath string) (result LookupResult[T], found bool) {

	result.StatusCode = http.StatusNotFound

	path := requestURI
	pathLen := len(path)
	if pathLen > 0 && t.PathSource == RequestURI {
		// Remove any query string.
		queryIdx := strings.IndexByte(path, '?')
		if queryIdx >= 0 {
			path = path[:queryIdx]
		}
	} else {
		// In testing with http.NewRequest,
		// RequestURI is not set so just grab URL.Path instead.
		path = urlPath
	}
	pathLen = len(path)
	unescapedPath := urlPath
	if t.CaseInsensitive {
		path = strings.ToLower(path)
		unescapedPath = strings.ToLower(unescapedPath)
	}

	trailingSlash := path[pathLen-1] == '/' && pathLen > 1
	if trailingSlash && t.RedirectTrailingSlash {
		path = path[:pathLen-1]
		unescapedPath = unescapedPath[:len(unescapedPath)-1]
	}

	isValid := t.Bridge.IsHandlerValid

	n, handler, params := t.root.search(method, path[1:], isValid)
	if n == nil {
		if t.RedirectCleanPath {
			// Path was not found. Try cleaning it up and search again.
			cleanPath := Clean(unescapedPath)
			n, handler, params = t.root.search(method, cleanPath[1:], isValid)
			if n == nil {
				// Still nothing found.
				return
			}
			if statusCode, ok := t.redirectStatusCode(method); ok {
				// Redirect to the actual path
				result.StatusCode = statusCode
				result.RedirectPath = cleanPath
				result.RoutePath = n.fullPath
				result.RouteType = n.routeType
				found = true
				return
			}
		} else {
			// Not found.
			return
		}
	}

	if !isValid(handler) {
		if method == "OPTIONS" && isValid(t.OptionsHandler) {
			handler = t.OptionsHandler
		}

		if !isValid(handler) {
			result.StatusCode = http.StatusMethodNotAllowed
			result.AllowedMethods = getSortedKeys(n.leafHandlers)
			result.RoutePath = n.fullPath
			result.RouteType = n.routeType
			return
		}
	}

	if !n.isCatchAll() || t.RemoveCatchAllTrailingSlash {
		if trailingSlash != n.addSlash && t.RedirectTrailingSlash {
			if statusCode, ok := t.redirectStatusCode(method); ok {
				if n.addSlash {
					result.StatusCode = statusCode
					result.RedirectPath = unescapedPath + "/"
					result.RoutePath = n.fullPath
					result.RouteType = n.routeType
				} else if path != "/" {
					result.StatusCode = statusCode
					result.RedirectPath = unescapedPath
					result.RoutePath = n.fullPath
					result.RouteType = n.routeType
				}
				if result.RedirectPath != "" {
					found = true
					return
				}
			}
		}
	}

	var retParams Params
	if num := len(params); num > 0 {
		if num != len(n.leafParamNames) {
			// Need better behavior here. Should this be a panic?
			panic(fmt.Sprintf("treemux: parameter list length mismatch: %v, %v",
				params, n.leafParamNames))
		}

		reverseSlice(params)
		retParams.Keys = n.leafParamNames
		retParams.Values = params
	}

	result = LookupResult[T]{
		StatusCode: http.StatusOK,
		Params:     retParams,
		Handler:    handler,
		RoutePath:  n.fullPath,
		RouteType:  n.routeType,
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
		t.mutex.RLock()
		defer t.mutex.RUnlock()
	}

	method := r.Method
	requestURI := r.RequestURI
	urlPath := r.URL.Path
	return t.lookup(method, requestURI, urlPath)
}

// LookupByPath is similar to Lookup, except that it accepts the routing parameters directly.
func (t *TreeMux[T]) LookupByPath(method, requestURI, urlPath string) (LookupResult[T], bool) {
	if t.SafeAddRoutesWhileRunning {
		t.mutex.RLock()
		defer t.mutex.RUnlock()
	}
	return t.lookup(method, requestURI, urlPath)
}

// defaultMethodNotAllowedHandler is the default handler for TreeMux.MethodNotAllowedHandler,
// which is called for patterns that match, but do not have a handler installed for the
// requested method. It simply writes the status code http.StatusMethodNotAllowed and fills
// in the `Allow` header value appropriately.
func defaultMethodNotAllowedHandler(
	w http.ResponseWriter, _ *http.Request, allowedMethods []string) {

	w.Header()["Allow"] = allowedMethods
	w.WriteHeader(http.StatusMethodNotAllowed)
}

// New creates a new TreeMux[T].
func New[T HandlerConstraint]() *TreeMux[T] {
	tm := &TreeMux[T]{
		root:                    &node[T]{path: "/"},
		NotFoundHandler:         http.NotFound,
		MethodNotAllowedHandler: defaultMethodNotAllowedHandler,
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
