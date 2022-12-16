package treemux

import (
	"context"
	"net/http"
)

type contextData struct {
	route  string
	params Params
}

func (cd *contextData) Route() string {
	return cd.route
}

func (cd *contextData) Param(name string) string {
	return cd.params.Get(name)
}

func (cd *contextData) Params() Params {
	return cd.params
}

// ContextData is the information associated with the matched path.
type ContextData interface {

	// Route returns the matched route, without expanded params.
	Route() string

	// Param returns the param value by name.
	Param(name string) string

	// Params returns the matched params.
	Params() Params
}

// NewContextData creates a new ContextData.
func NewContextData(route string, params Params) ContextData {
	return &contextData{
		route:  route,
		params: params,
	}
}

// GetContextData returns the ContextData associated with the request.
// In case that no data is available, it returns an empty ContextData.
func GetContextData(r *http.Request) ContextData {
	return getDataFromContext(r.Context())
}

func getDataFromContext(ctx context.Context) ContextData {
	if p, ok := ctx.Value(contextDataKey).(ContextData); ok {
		return p
	}
	return &contextData{}
}

// AddContextData helps to do testing.
// It inserts given ContestData into the request's `Context` using the
// package's internal context key.
func AddContextData(r *http.Request, data ContextData) *http.Request {
	return r.WithContext(addDataToContext(r.Context(), data))
}

func addDataToContext(ctx context.Context, data ContextData) context.Context {
	return context.WithValue(ctx, contextDataKey, data)
}

// AddParamsToContext helps to do testing.
// It inserts given params into a context using the package's internal context key.
func AddParamsToContext(ctx context.Context, params Params) context.Context {
	return addDataToContext(ctx, &contextData{
		params: params,
	})
}

// AddRouteToContext helps to do testing.
// It inserts given route into a context using the package's internal context key.
func AddRouteToContext(ctx context.Context, route string) context.Context {
	return addDataToContext(ctx, &contextData{
		route: route,
	})
}

type contextKey int

// contextDataKey is used to retrieve the path's params and matched route
// from a request's context.
const contextDataKey contextKey = 0
