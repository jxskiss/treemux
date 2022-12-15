package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
)

func main() {
	router := treemux.New[gin.HandlerFunc]()
	router.Bridge = ginBridge{}
	examples.SetupRoutes(router, func(c *gin.Context) {
		c.JSON(200, c.Params)
	})

	server := gin.Default()

	server.Any("/*any", func(c *gin.Context) {
		method := c.Request.Method
		path := c.Request.URL.Path
		requestURI := c.Request.RequestURI

		log.Printf("%v, path= %v, requestURI= %v", method, path, requestURI)

		lr, _ := router.LookupByPath(method, path, requestURI)

		if lr.RedirectPath != "" {
			c.Redirect(lr.StatusCode, lr.RedirectPath)
			return
		}

		c.Params = c.Params[:0]
		for i, key := range lr.Params.Keys {
			val := lr.Params.Values[i]
			c.Params = append(c.Params, gin.Param{key, val})
		}

		if lr.Handler != nil {
			lr.Handler(c)
			return
		}

		if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.RegisteredMethods) > 0 {
			router.MethodNotAllowedHandler(c.Writer, c.Request, lr.RegisteredMethods)
			return
		}

		router.NotFoundHandler(c.Writer, c.Request)
	})

	addr := ":8888"
	log.Printf("listening: %v", addr)
	server.Run(addr)
}

type ginBridge struct {
	treemux.UnimplementedBridge[gin.HandlerFunc]
}

func (ginBridge) IsHandlerValid(handler gin.HandlerFunc) bool {
	return handler != nil
}
