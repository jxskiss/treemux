package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
)

func main() {
	router := treemux.New[treemux.HandlerFunc]()
	examples.SetupRoutes(router, func(w http.ResponseWriter, r *http.Request, urlParams treemux.Params) {
		buf, _ := json.Marshal(urlParams)
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
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
