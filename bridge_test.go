package treemux

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGroupUseHandler(t *testing.T) {
	var execLog []string

	record := func(s string) {
		execLog = append(execLog, s)
	}

	assertExecLog := func(wanted []string) {
		if !reflect.DeepEqual(execLog, wanted) {
			t.Fatalf("got %v, wanted %v", execLog, wanted)
		}
	}

	newHandler := func(name string) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, params Params) {
			record(name)
		}
	}

	newParamsMiddleware := func(name string, paramKey string) MiddlewareFunc[HandlerFunc] {
		return func(next HandlerFunc) HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request, params Params) {
				t.Log(params)
				record(name)
				record(params.Get(paramKey))
				next(w, r, params)
			}
		}
	}

	t.Log("Http Handler Middleware")
	newHttpHandlerMiddleware := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record(name)
				next.ServeHTTP(w, r)
			})
		}
	}
	{
		router := New[HandlerFunc]()
		w := httptest.NewRecorder()
		execLog = nil
		router.UseHandler(newHttpHandlerMiddleware("m5"))
		router.Use(newParamsMiddleware("m6", "p"))
		router.GET("/h7/:p", newHandler("h7"))

		req, _ := newRequest("GET", "/h7/paramvalue", nil)
		router.ServeHTTP(w, req)

		req, _ = newRequest("GET", "/h7/anothervalue", nil)
		router.ServeHTTP(w, req)

		assertExecLog([]string{"m5", "m6", "paramvalue", "h7", "m5", "m6", "anothervalue", "h7"})
	}
}
