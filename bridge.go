package treemux

import (
	"net/http"
	"net/url"
	"reflect"
	"unsafe"
)

// HandlerBridge is a bridge function which connects TreeMux and user defined Handler
// by converting Handler and matched params to [http.HandlerFunc].
type HandlerBridge[T HandlerConstraint] func(handler T, params map[string]string) http.HandlerFunc

// HTTPHandlerMiddleware is a short name for [http.Handler] middleware
// `func(http.Handler) http.Handler`.
type HTTPHandlerMiddleware func(http.Handler) http.Handler

// MiddlewareBridge is a bridge function which converts a stdlib
// http middleware to MiddlewareFunc[T].
type MiddlewareBridge[T HandlerConstraint] func(HTTPHandlerMiddleware) MiddlewareFunc[T]

// ServeHTTP implements the interface [http.Handler].
func (t *TreeMux[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.PanicHandler != nil {
		defer t.serveHTTPPanic(w, r)
	}
	if t.BridgeFunc == nil {
		panic("treemux: BridgeFunc is not configured")
	}

	if t.SafeAddRoutesWhileRunning {
		// In concurrency safe mode, we acquire a read lock on the mutex for any access.
		// This is optional to avoid potential performance loss in high-usage scenarios.
		t.mutex.RLock()
	}

	result, _ := t.lookup(w, r)

	if t.SafeAddRoutesWhileRunning {
		t.mutex.RUnlock()
	}

	t.ServeLookupResult(w, r, result)
}

// ServeLookupResult serves a request, given a lookup result from the Lookup function.
func (t *TreeMux[T]) ServeLookupResult(
	w http.ResponseWriter,
	r *http.Request,
	lr LookupResult[T],
) {
	if lr.redirectPath != "" {
		redirect(w, r, lr.redirectPath, lr.StatusCode)
		return
	}

	r = t.setDefaultRequestContext(r)
	if t.UseContextData {
		r = AddContextData(r, &contextData{
			route:  lr.routePath,
			params: lr.Params,
		})
	}

	if lr.handler.IsValid() {
		t.BridgeFunc(lr.handler, lr.Params)(w, r)
	} else if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.registeredMethods) > 0 {
		t.MethodNotAllowedHandler(w, r, lr.registeredMethods)
	} else {
		t.NotFoundHandler(w, r)
	}
}

func redirect(w http.ResponseWriter, r *http.Request, newPath string, statusCode int) {
	newURL := url.URL{
		Path:     newPath,
		RawQuery: r.URL.RawQuery,
		Fragment: r.URL.Fragment,
	}
	http.Redirect(w, r, newURL.String(), statusCode)
}

// UseHandler is like Use but accepts [http.Handler] middleware.
// It calls the middleware wrapper to convert the given middleware
// to a MiddlewareFunc.
func (g *Group[T]) UseHandler(middleware func(http.Handler) http.Handler) {
	if g.mux.MiddlewareWrapper == nil {
		panic("treemux: MiddlewareWrapper is not configured")
	}
	g.stack = append(g.stack, g.mux.MiddlewareWrapper(middleware))
}

// The params argument contains the parameters parsed from wildcards and catch-alls in the URL.
type HandlerFunc func(http.ResponseWriter, *http.Request, map[string]string)

func (p HandlerFunc) IsValid() bool { return p != nil }

func httpTreeMuxBridge(h HandlerFunc, params map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r, params)
	}
}

type httpTreeMuxHandlerWithParams struct {
	handler HandlerFunc
	params  map[string]string
}

func (h httpTreeMuxHandlerWithParams) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler(w, r, h.params)
}

func httpTreeMuxMiddlewareBridge(middleware HTTPHandlerMiddleware) MiddlewareFunc[HandlerFunc] {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
			innerHandler := httpTreeMuxHandlerWithParams{next, urlParams}
			middleware(innerHandler).ServeHTTP(w, r)
		}
	}
}

type HTTPHandlerFunc func(w http.ResponseWriter, r *http.Request)

func (p HTTPHandlerFunc) IsValid() bool { return p != nil }

func httpHandlerBridge(h HTTPHandlerFunc, params map[string]string) http.HandlerFunc {
	return http.HandlerFunc(h)
}

func httpHandlerMiddlewareBridge(middleware HTTPHandlerMiddleware) MiddlewareFunc[HTTPHandlerFunc] {
	return func(next HTTPHandlerFunc) HTTPHandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware(http.HandlerFunc(next)).ServeHTTP(w, r)
		}
	}
}

func setDefaultBridgeFunctions[T HandlerConstraint](r *TreeMux[T]) {
	var x T
	var typ = reflect.TypeOf(x)
	if typ == reflect.TypeOf(HandlerFunc(nil)) {
		handlerBridge := httpTreeMuxBridge
		middlewareBridge := httpTreeMuxMiddlewareBridge
		r.BridgeFunc = *(*HandlerBridge[T])(unsafe.Pointer(&handlerBridge))
		r.MiddlewareWrapper = *(*MiddlewareBridge[T])(unsafe.Pointer(&middlewareBridge))
	} else if typ == reflect.TypeOf(HTTPHandlerFunc(nil)) {
		handlerBridge := httpHandlerBridge
		middlewareBridge := httpHandlerMiddlewareBridge
		r.BridgeFunc = *(*HandlerBridge[T])(unsafe.Pointer(&handlerBridge))
		r.MiddlewareWrapper = *(*MiddlewareBridge[T])(unsafe.Pointer(&middlewareBridge))
	}
}
