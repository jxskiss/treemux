package treemux

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestContextData(t *testing.T) {
	p := &contextData{
		route:  "route/path",
		params: map[string]string{"id": "123"},
	}

	ctx := addDataToContext(context.Background(), p)

	ctxData := getDataFromContext(ctx)
	pathValue := ctxData.Route()
	if pathValue != p.route {
		t.Errorf("expected '%s', but got '%s'", p, pathValue)
	}

	params := ctxData.Params()
	if v := params["id"]; v != "123" {
		t.Errorf("expected '%s', but got '%#v'", p.params["id"], params["id"])
	}
}

func TestContextDataParams(t *testing.T) {
	m := &contextData{
		params: map[string]string{"id": "123"},
		route:  "",
	}

	ctx := addDataToContext(context.Background(), m)

	params := getDataFromContext(ctx).Params()
	if params == nil {
		t.Errorf("expected '%#v', but got '%#v'", m, params)
	}

	if v := params["id"]; v != "123" {
		t.Errorf("expected '%s', but got '%#v'", m.params["id"], params["id"])
	}
}

func TestContextDataRoute(t *testing.T) {
	tests := []struct {
		name,
		expectedRoute string
	}{
		{
			name:          "basic",
			expectedRoute: "/base/path",
		},
		{
			name:          "params",
			expectedRoute: "/base/path/:id/items/:itemid",
		},
		{
			name:          "catch-all",
			expectedRoute: "/base/*path",
		},
		{
			name:          "empty",
			expectedRoute: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cd := &contextData{}
			if len(test.expectedRoute) > 0 {
				cd.route = test.expectedRoute
			}

			ctx := context.WithValue(context.Background(), contextDataKey, cd)

			gotRoute := getDataFromContext(ctx).Route()

			if test.expectedRoute != gotRoute {
				t.Errorf("ContextRoute didn't return the desired route\nexpected %s\ngot: %s", test.expectedRoute, gotRoute)
			}
		})
	}
}

func TestDefaultContext(t *testing.T) {
	router := New[HandlerFunc]()
	ctx := context.WithValue(context.Background(), "abc", "def")
	expectContext := false

	router.GET("/abc", func(w http.ResponseWriter, r *http.Request, params map[string]string) {
		contextValue := r.Context().Value("abc")
		if expectContext {
			x, ok := contextValue.(string)
			if !ok || x != "def" {
				t.Errorf("Unexpected context key value: %+v", contextValue)
			}
		} else {
			if contextValue != nil {
				t.Errorf("Expected blank context but key had value %+v", contextValue)
			}
		}
	})

	r, err := http.NewRequest("GET", "/abc", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	t.Log("Testing without DefaultContext")
	router.ServeHTTP(w, r)

	router.DefaultContext = ctx
	expectContext = true
	w = httptest.NewRecorder()
	t.Log("Testing with DefaultContext")
	router.ServeHTTP(w, r)
}

func TestAddContextData(t *testing.T) {
	expectedRoute := "/expected/route"
	expectedParams := map[string]string{
		"test": "expected",
	}

	ctx := addDataToContext(context.Background(), &contextData{
		route:  expectedRoute,
		params: expectedParams,
	})

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if gotData.route != expectedRoute {
			t.Errorf("Did not retrieve the desired route. Expected: %s; Got: %s", expectedRoute, gotData.route)
		}
		if !reflect.DeepEqual(expectedParams, gotData.params) {
			t.Errorf("Did not retrieve the desired parameters. Expected: %#v; Got: %#v", expectedParams, gotData.params)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}

func TestAddParamsToContext(t *testing.T) {
	expectedParams := map[string]string{
		"test": "expected",
	}

	ctx := AddParamsToContext(context.Background(), expectedParams)

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if !reflect.DeepEqual(expectedParams, gotData.params) {
			t.Errorf("Did not retrieve the desired parameters. Expected: %#v; Got: %#v", expectedParams, gotData.params)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}

func TestAddRouteToContext(t *testing.T) {
	expectedRoute := "/expected/route"

	ctx := AddRouteToContext(context.Background(), expectedRoute)

	if gotData, ok := ctx.Value(contextDataKey).(*contextData); ok && gotData != nil {
		if gotData.route != expectedRoute {
			t.Errorf("Did not retrieve the desired route. Expected: %s; Got: %s", expectedRoute, gotData.route)
		}
	} else {
		t.Error("failed to retrieve context data")
	}
}

func TestContextDataWithMiddleware(t *testing.T) {
	wantRoute := "/foo/:id/bar"
	wantParams := map[string]string{
		"id": "15",
	}

	validateRequestAndParams := func(request *http.Request, params map[string]string, location string) {
		data := GetContextData(request)
		if data == nil {
			t.Fatalf("GetContextData returned nil in %s", location)
		}
		if data.Route() != wantRoute {
			t.Errorf("Unexpected route in %s.  Got %s", location, data.Route())
		}
		if !reflect.DeepEqual(data.Params(), wantParams) {
			t.Errorf("Unexpected context params in %s. Got %+v", location, data.Params())
		}
		if !reflect.DeepEqual(params, wantParams) {
			t.Errorf("Unexpected handler params in %s. Got %+v", location, params)
		}
	}

	router := New[HandlerFunc]()
	router.UseContextData = true
	router.Use(func(next HandlerFunc) HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request, m map[string]string) {
			t.Log("Testing Middleware")
			validateRequestAndParams(request, m, "middleware")
			next(writer, request, m)
		}
	})

	router.GET(wantRoute, tempToHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Log("Testing handler")
		validateRequestAndParams(request, GetContextData(request).Params(), "handler")
		writer.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/foo/15/bar", nil)
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code.  got %d", w.Code)
	}
}

func tempToHandlerFunc(f http.HandlerFunc) func(http.ResponseWriter, *http.Request, map[string]string) {
	return func(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
		_ = urlParams
		f(w, r)
	}
}
