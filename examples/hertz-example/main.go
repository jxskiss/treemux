package main

import (
	"context"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
	"github.com/jxskiss/treemux/pkg/hertzbridge"
)

func main() {
	router := treemux.New[*hertzbridge.HertzHandler]()
	bridge := hertzbridge.New()
	bridge.SetRouter(router)

	router.Use(bridge.WrapMiddleware(
		func(ctx context.Context, c *app.RequestContext) {
			ctx = context.WithValue(ctx, "middlewareVar1", "middlewareValue1")
			c.Next(ctx)
		},
		func(ctx context.Context, c *app.RequestContext) {
			c.Set("middlewareVar2", "middlewareValue2")
			// Don't need to call c.Next here.
		},
	))

	examples.SetupRoutes(router, func() *hertzbridge.HertzHandler {
		return bridge.WrapHandler(
			func(ctx context.Context, c *app.RequestContext) {
				log.Printf("middleware added in bridge.WrapHandler: middlewareVar2= %v", c.GetString("middlewareVar2"))
			},
			dummyHandler,
		)
	})

	addr := ":8888"

	// server.Default() creates a Hertz with recovery middleware.
	// If you need a pure hertz, you can use server.New()
	h := server.Default(server.WithHostPorts(addr))

	h.Any("/*any", bridge.Serve)

	log.Printf("listening: %v", addr)
	h.Spin()
}

func dummyHandler(ctx context.Context, c *app.RequestContext) {

	var data [][2]interface{}
	addKV := func(k string, v interface{}) {
		data = append(data, [2]interface{}{k, v})
	}

	addKV("middlewareVar1", ctx.Value("middlewareVar1"))
	addKV("middlewareVar2", c.GetString("middlewareVar2"))
	addKV("Params", c.Params)
	addKV("Host", string(c.Host()))
	addKV("Path", string(c.Path()))
	addKV("RequestURI", string(c.Request.RequestURI()))
	addKV("FullPath", c.FullPath())
	addKV("Method", string(c.Method()))
	addKV("HandlerName", c.HandlerName())
	addKV("Content-Type", string(c.ContentType()))

	c.IndentedJSON(200, data)
}
