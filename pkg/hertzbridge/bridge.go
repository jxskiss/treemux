package hertzbridge

import (
	"context"
	"net/http"
	"reflect"
	"sync/atomic"
	"unsafe"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/jxskiss/treemux"
)

type HertzHandler struct {
	HandlersChain app.HandlersChain
}

func (h *HertzHandler) addMiddleware(mw app.HandlerFunc) {
	if inHandlersChain(h.HandlersChain, mw) {
		panic("treemux/pkg/hertzbridge: middleware already registered for this handler")
	}

	chain := h.HandlersChain
	if len(chain) == cap(h.HandlersChain) {
		chain = append(app.HandlersChain{mw}, chain...)
	} else {
		chain = chain[:len(chain)+1]
		copy(chain[:len(chain)-1], chain[1:])
		chain[0] = mw
	}
	h.HandlersChain = chain
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

type HertzBridge struct {
	mux unsafe.Pointer // *treemux.TreeMux[*HertzHandler]

	treemux.UnimplementedBridge[*HertzHandler]
}

func (*HertzBridge) IsHandlerValid(handler *HertzHandler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (*HertzBridge) WrapHandler(handler app.HandlerFunc) *HertzHandler {
	return &HertzHandler{
		HandlersChain: app.HandlersChain{handler},
	}
}

func (*HertzBridge) WrapMiddleware(mw app.HandlerFunc) treemux.MiddlewareFunc[*HertzHandler] {
	return func(next *HertzHandler) *HertzHandler {
		next.addMiddleware(mw)
		return next
	}
}

func (b *HertzBridge) Serve(ctx context.Context, rc *app.RequestContext) {
	method := string(rc.Method())
	requestURI := string(rc.Request.RequestURI())
	urlPath := string(rc.URI().Path())

	mux := b.getMux()
	lr, _ := mux.LookupByPath(method, requestURI, urlPath)

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

	if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.AllowedMethods) > 0 {
		for _, m := range lr.AllowedMethods {
			rc.Response.Header.Add("Allow", m)
		}
		rc.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}

	rc.NotFound()
}

func (b *HertzBridge) getMux() *treemux.TreeMux[*HertzHandler] {
	return (*treemux.TreeMux[*HertzHandler])(atomic.LoadPointer(&b.mux))
}

// SetMux changes the mux of the bridge, it's safe to use concurrently.
func (b *HertzBridge) SetMux(mux *treemux.TreeMux[*HertzHandler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}

// New creates a new HertzBridge.
func New(mux *treemux.TreeMux[*HertzHandler]) *HertzBridge {
	bridge := &HertzBridge{}
	bridge.SetMux(mux)
	return bridge
}
