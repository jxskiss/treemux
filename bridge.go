package treemux

import (
	"net/http"
	"reflect"
)

// HTTPHandlerMiddleware is an alias name for [http.Handler] middleware
// `func(http.Handler) http.Handler`.
type HTTPHandlerMiddleware = func(http.Handler) http.Handler

// Bridge is a bridge which helps TreeMux with user defined handlers
// to work together with [http.Handler] and [http.HandlerFunc] in stdlib.
type Bridge[T HandlerConstraint] interface {

	// IsHandlerValid tells whether the Handler is valid, a valid Handler
	// which matches the request stops the router searching the routing rules.
	IsHandlerValid(handler T) bool

	// ToHTTPHandlerFunc convert a handler T and params to [http.HandlerFunc].
	//
	// This method is unnecessary when you don't use http.Handler features.
	ToHTTPHandlerFunc(handler T, urlParams Params) http.HandlerFunc

	// ConvertMiddleware converts a HTTPHandlerMiddleware to MiddlewareFunc[T].
	//
	// This method is unnecessary when you don't use and http.Handler based middlewares.
	ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[T]
}

// ServeHTTP implements the interface [http.Handler].
func (t *TreeMux[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.PanicHandler != nil {
		defer t.serveHTTPPanic(w, r)
	}

	result, _ := t.Lookup(w, r)
	t.ServeLookupResult(w, r, result)
}

// ServeLookupResult serves a request, given a lookup result from the Lookup function.
// TreeMux.Bridge must be configured, else it panics.
func (t *TreeMux[T]) ServeLookupResult(
	w http.ResponseWriter,
	r *http.Request,
	lr LookupResult[T],
) {
	if lr.RedirectPath != "" {
		redirect(w, r, lr.RedirectPath, lr.StatusCode)
		return
	}

	r = t.setDefaultRequestContext(r)
	if t.UseContextData {
		r = AddContextData(r, &contextData{
			route:  lr.RoutePath,
			params: lr.Params,
		})
	}

	if t.Bridge == nil {
		panic("treemux: Bridge is not configured")
	}
	if t.Bridge.IsHandlerValid(lr.Handler) {
		t.Bridge.ToHTTPHandlerFunc(lr.Handler, lr.Params)(w, r)
	} else if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.RegisteredMethods) > 0 {
		t.MethodNotAllowedHandler(w, r, lr.RegisteredMethods)
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

// HandlerFunc is a default handler type.
// The parameter urlParams contains the params parsed from the request's URL.
type HandlerFunc func(w http.ResponseWriter, r *http.Request, urlParams Params)

type defaultBridge struct{}

func (defaultBridge) IsHandlerValid(handler HandlerFunc) bool {
	return handler != nil
}

func (defaultBridge) ToHTTPHandlerFunc(handler HandlerFunc, params Params) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, params)
	}
}

type handlerWithParams struct {
	handler HandlerFunc
	params  Params
}

func (h handlerWithParams) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler(w, r, h.params)
}

func (b defaultBridge) ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[HandlerFunc] {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, urlParams Params) {
			innerHandler := handlerWithParams{next, urlParams}
			middleware(innerHandler).ServeHTTP(w, r)
		}
	}
}

// HTTPHandlerFunc is an alias type of [http.HandlerFunc].
type HTTPHandlerFunc = http.HandlerFunc

type stdlibBridge struct{}

func (stdlibBridge) IsHandlerValid(handler HTTPHandlerFunc) bool {
	return handler != nil
}

func (stdlibBridge) ToHTTPHandlerFunc(handler HTTPHandlerFunc, urlParams Params) http.HandlerFunc {
	_ = urlParams
	return handler
}

func (stdlibBridge) ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[HTTPHandlerFunc] {
	return func(next HTTPHandlerFunc) HTTPHandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware(next).ServeHTTP(w, r)
		}
	}
}

func setDefaultBridgeFunctions[T HandlerConstraint](r *TreeMux[T]) {
	var x T
	var typ = reflect.TypeOf(x)
	if typ == reflect.TypeOf(HandlerFunc(nil)) {
		r.Bridge = (interface{}(defaultBridge{})).(Bridge[T])
	} else if typ == reflect.TypeOf(HTTPHandlerFunc(nil)) {
		r.Bridge = (interface{}(stdlibBridge{})).(Bridge[T])
	}
}

type UnimplementedBridge[T HandlerConstraint] struct{}

func (UnimplementedBridge[T]) IsHandlerValid(handler T) bool {
	panic("implement me")
}

func (UnimplementedBridge[T]) ToHTTPHandlerFunc(handler T, urlParams Params) http.HandlerFunc {
	panic("implement me")
}

func (UnimplementedBridge[T]) ConvertMiddleware(middleware HTTPHandlerMiddleware) MiddlewareFunc[T] {
	panic("implement me")
}
