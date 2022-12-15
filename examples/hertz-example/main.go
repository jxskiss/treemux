package main

import (
	"context"
	"log"
	"net/http"
	"reflect"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route/param"

	"github.com/jxskiss/treemux"
	"github.com/jxskiss/treemux/examples"
)

func main() {
	router := treemux.New[*HertzHandler]()

	bridge := hertzBridge{mux: router}
	router.Bridge = bridge

	router.Use(bridge.WrapMiddleware(func(ctx context.Context, c *app.RequestContext) {
		ctx = context.WithValue(ctx, "middlewareVar", "middlewareValue")
		c.Next(ctx)
	}))

	examples.SetupRoutes(router, func() *HertzHandler {
		return bridge.WrapHandler(dummyHandler)
	})

	addr := ":8888"

	// server.Default() creates a Hertz with recovery middleware.
	// If you need a pure hertz, you can use server.New()
	h := server.Default(server.WithHostPorts(addr))

	h.Any("/*any", bridge.Dispatch)

	log.Printf("listening: %v", addr)
	h.Spin()
}

func dummyHandler(ctx context.Context, c *app.RequestContext) {

	log.Printf("middlewareVar: %v", ctx.Value("middlewareVar"))

	var data [][2]interface{}
	addKV := func(k string, v interface{}) {
		data = append(data, [2]interface{}{k, v})
	}

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

type HertzHandler struct {
	HandlersChain app.HandlersChain
}

type hertzBridge struct {
	mux *treemux.TreeMux[*HertzHandler]
	treemux.UnimplementedBridge[*HertzHandler]
}

func (hertzBridge) IsHandlerValid(handler *HertzHandler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (hertzBridge) WrapHandler(handler app.HandlerFunc) *HertzHandler {
	return &HertzHandler{
		HandlersChain: app.HandlersChain{handler},
	}
}

func (hertzBridge) WrapMiddleware(mw app.HandlerFunc) treemux.MiddlewareFunc[*HertzHandler] {
	return func(next *HertzHandler) *HertzHandler {

		log.Printf("next.HandlersChain: %+v", next.HandlersChain)

		if inHandlersChain(next.HandlersChain, mw) {
			panic("middleware already registered for this handler")
		}

		next.HandlersChain = append(app.HandlersChain{mw}, next.HandlersChain...)
		return next
	}
}

func (b hertzBridge) Dispatch(ctx context.Context, rc *app.RequestContext) {
	method := string(rc.Method())
	path := string(rc.Path())
	requestURI := string(rc.Request.RequestURI())

	log.Printf("%v, path= %v, requestURI= %v", method, path, requestURI)

	lr, _ := b.mux.LookupByPath(method, path, requestURI)

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
		realHandlers := append(rc.Handlers(), lr.Handler.HandlersChain...)
		rc.SetHandlers(realHandlers)
		rc.SetFullPath(lr.RoutePath)
		rc.Next(ctx)
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
}

func inHandlersChain(chain app.HandlersChain, h app.HandlerFunc) bool {
	for _, x := range chain {
		if getFuncAddr(x) == getFuncAddr(h) {
			return true
		}
	}
	return false
}

func getFuncAddr(v interface{}) uintptr {
	return reflect.ValueOf(reflect.ValueOf(v)).Field(1).Pointer()
}
