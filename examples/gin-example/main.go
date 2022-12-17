package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
	"github.com/jxskiss/treemux/pkg/ginbridge"
)

func main() {
	router := treemux.New[*ginbridge.Handler]()
	bridge := ginbridge.New()
	bridge.SetRouter(router)

	router.Use(ginbridge.WrapMiddleware(
		func(c *gin.Context) {
			c.Set("middlewareVar1", "middlewareValue1")
		},
		func(c *gin.Context) {
			c.Set("middlewareVar2", "middlewareValue2")
		},
	))

	router.UseHandler(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cArg := r.URL.Query().Get("c")
			log.Printf("in http handler middleware, c= %v", cArg)
			if cArg == "next" {

				// Go to next handler.
				next.ServeHTTP(w, r)

			} else {

				// Here we don't call next.ServeHTTP, the resting handlers won't be called.
				w.WriteHeader(200)
				w.Write([]byte("Early response from http handler middleware."))

			}
		})
	})

	examples.SetupRoutes(router, func() *ginbridge.Handler {
		return ginbridge.WrapHandler(
			func(c *gin.Context) {
				log.Printf("middleware added in bridge.WrapHandler: middlewareVar2= %v", c.GetString("middlewareVar2"))
			},
			dummyHandler,
		)
	})

	// gin.Default() returns an Engine instance with the Logger and Recovery
	// middleware already attached.
	eng := gin.Default()
	eng.Any("/*any", bridge.Serve)

	addr := ":8888"
	log.Printf("listening: %v", addr)
	eng.Run(addr)
}

func dummyHandler(c *gin.Context) {

	var data [][2]interface{}
	addKV := func(k string, v interface{}) {
		data = append(data, [2]interface{}{k, v})
	}

	addKV("middlewareVar1", c.GetString("middlewareVar1"))
	addKV("middlewareVar2", c.GetString("middlewareVar2"))
	addKV("Params", c.Params)
	addKV("Host", c.Request.Host)
	addKV("Path", c.Request.URL.Path)
	addKV("RequestURI", c.Request.URL.RequestURI())
	addKV("FullPath", c.FullPath())
	addKV("Method", c.Request.Method)
	addKV("HandlerName", c.HandlerName())
	addKV("Content-Type", c.ContentType())

	c.IndentedJSON(200, data)
}
