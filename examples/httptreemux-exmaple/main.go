package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dimfeld/httptreemux/v5"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
)

func main() {
	router := treemux.New[httptreemux.HandlerFunc]()
	router.Bridge = httptreemuxBridge{}

	examples.SetupRoutes(router, func() httptreemux.HandlerFunc {
		return dummyHandler
	})

	server := http.Server{
		Addr:    ":8888",
		Handler: router,
	}

	log.Printf("listening: %v", server.Addr)
	err := server.ListenAndServe()
	if err != nil {
		log.Printf("server exit: %v", err)
	}
}

func dummyHandler(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
	buf, _ := json.Marshal(urlParams)
	w.Header().Set("Content-Type", "application/json")
	w.Write(buf)
}

type httptreemuxBridge struct{}

func (httptreemuxBridge) IsHandlerValid(handler httptreemux.HandlerFunc) bool {
	return handler != nil
}

func (httptreemuxBridge) ToHTTPHandlerFunc(handler httptreemux.HandlerFunc, urlParams treemux.Params) http.HandlerFunc {
	paramMap := urlParams.ToMap()
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, paramMap)
	}
}

type handlerWithParams struct {
	handler httptreemux.HandlerFunc
	params  map[string]string
}

func (h handlerWithParams) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler(w, r, h.params)
}

func (httptreemuxBridge) ConvertMiddleware(middleware treemux.HTTPHandlerMiddleware) treemux.MiddlewareFunc[httptreemux.HandlerFunc] {
	return func(next httptreemux.HandlerFunc) httptreemux.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, urlParams map[string]string) {
			innerHandler := handlerWithParams{next, urlParams}
			middleware(innerHandler).ServeHTTP(w, r)
		}
	}
}
