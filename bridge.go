package treemux

import (
	"net/http"
	"reflect"
)

// HTTPHandlerMiddleware is a short name for [http.Handler] middleware
// `func(http.Handler) http.Handler`.
type HTTPHandlerMiddleware func(http.Handler) http.Handler

// Bridge is a bridge which helps TreeMux with user defined handlers
// to work together with [http.Handler] and [http.HandlerFunc] in stdlib.
type Bridge[T HandlerConstraint] interface {

	// ToHTTPHandlerFunc convert a handler T and params to [http.HandlerFunc].
	ToHTTPHandlerFunc(handler T, urlParams map[string]string) http.HandlerFunc

	// ConvertMiddleware converts a HTTPHandlerMiddleware to MiddlewareFunc[T].
	ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[T]
}

// ServeHTTP implements the interface [http.Handler].
func (t *TreeMux[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.PanicHandler != nil {
		defer t.serveHTTPPanic(w, r)
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
// TreeMux.Bridge must be configured, else it panics.
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
		if t.Bridge == nil {
			panic("treemux: Bridge is not configured")
		}
		t.Bridge.ToHTTPHandlerFunc(lr.handler, lr.Params)(w, r)
	} else if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.registeredMethods) > 0 {
		t.MethodNotAllowedHandler(w, r, lr.registeredMethods)
	} else {
		t.NotFoundHandler(w, r)
	}
}

// UseHandler is like Use but accepts [http.Handler] middleware.
// It calls the middleware wrapper to convert the given middleware
// to a MiddlewareFunc.
func (g *Group[T]) UseHandler(middleware func(http.Handler) http.Handler) {
	if g.mux.Bridge == nil {
		panic("treemux: Bridge is not configured")
	}
	g.stack = append(g.stack, g.mux.Bridge.ConvertMiddleware(middleware))
}

// HandlerFunc is a default handler type which satisfies the HandlerConstraint.
// The parameter urlParams contains the params parsed from the request's URL.
type HandlerFunc func(w http.ResponseWriter, r *http.Request, urlParams map[string]string)

func (p HandlerFunc) IsValid() bool { return p != nil }

type httpTreeMuxBridge struct{}

func (httpTreeMuxBridge) ToHTTPHandlerFunc(handler HandlerFunc, params map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, params)
	}
}

type httpTreeMuxHandlerWithParams struct {
	handler HandlerFunc
	params  map[string]string
}

func (h httpTreeMuxHandlerWithParams) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler(w, r, h.params)
}

func (b httpTreeMuxBridge) ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[HandlerFunc] {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
			innerHandler := httpTreeMuxHandlerWithParams{next, urlParams}
			middleware(innerHandler).ServeHTTP(w, r)
		}
	}
}

// HTTPHandlerFunc equals http.HandlerFunc.
// It satisfies HandlerConstraint, thus it can be used with TreeMux.
type HTTPHandlerFunc func(w http.ResponseWriter, r *http.Request)

func (p HTTPHandlerFunc) IsValid() bool { return p != nil }

type stdlibBridge struct{}

func (stdlibBridge) ToHTTPHandlerFunc(handler HTTPHandlerFunc, urlParams map[string]string) http.HandlerFunc {
	_ = urlParams
	return http.HandlerFunc(handler)
}

func (stdlibBridge) ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[HTTPHandlerFunc] {
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
		r.Bridge = (interface{}(httpTreeMuxBridge{})).(Bridge[T])
	} else if typ == reflect.TypeOf(HTTPHandlerFunc(nil)) {
		r.Bridge = (interface{}(stdlibBridge{})).(Bridge[T])
	}
}
