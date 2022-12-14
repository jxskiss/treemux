package main

import (
	"context"
	"log"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route/param"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
)

func main() {
	router := treemux.New[app.HandlerFunc]()
	router.Bridge = hertzBridge{}
	examples.SetupRoutes(router, func(ctx context.Context, c *app.RequestContext) {
		c.JSON(200, c.Params)
	})

	addr := ":8888"

	// server.Default() creates a Hertz with recovery middleware.
	// If you need a pure hertz, you can use server.New()
	h := server.Default(server.WithHostPorts(addr))

	h.Any("/*any", func(ctx context.Context, rc *app.RequestContext) {
		method := string(rc.Method())
		path := string(rc.Path())
		requestURI := string(rc.Request.RequestURI())

		log.Printf("%v, path= %v, requestURI= %v", method, path, requestURI)

		lr, _ := router.LookupByPath(method, path, requestURI)

		if lr.RedirectPath != "" {
			rc.Redirect(lr.StatusCode, []byte(lr.RedirectPath))
			return
		}

		rc.Params = rc.Params[:0]
		for i, key := range lr.Params.Keys {
			val := lr.Params.Values[i]
			rc.Params = append(rc.Params, param.Param{key, val})
		}

		if lr.Handler != nil {
			lr.Handler(ctx, rc)
			return
		}

		if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.RegisteredMethods) > 0 {
			for _, m := range lr.RegisteredMethods {
				rc.Response.Header.Add("Allow", m)
			}
			rc.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}

		rc.NotFound()
	})

	log.Printf("listening: %v", addr)
	h.Spin()
}

type hertzBridge struct {
	treemux.UnimplementedBridge[app.HandlerFunc]
}

func (hertzBridge) IsHandlerValid(handler app.HandlerFunc) bool {
	return handler != nil
}
