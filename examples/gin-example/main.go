package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
	"github.com/jxskiss/treemux/pkg/ginbridge"
)

func main() {
	router := treemux.New[*ginbridge.GinHandler]()
	bridge := ginbridge.New(router)
	bridge.SetMux(router)

	router.Use(bridge.WrapMiddleware(func(c *gin.Context) {
		c.Set("middlewareVar", "middlewareValue")
	}))

	examples.SetupRoutes(router, func() *ginbridge.GinHandler {
		return bridge.WrapHandler(dummyHandler)
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

	addKV("middlewareVar", c.GetString("middlewareVar"))
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
