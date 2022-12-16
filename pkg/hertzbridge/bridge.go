package hertzbridge

import (
	"context"
	"net/http"
	"sync/atomic"
	"unsafe"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/jxskiss/treemux"
)

// New creates a new HertzBridge.
func New() *HertzBridge {
	bridge := &HertzBridge{}
	return bridge
}

type HertzBridge struct {
	mux unsafe.Pointer // *treemux.TreeMux[*HertzHandler]

	treemux.UnimplementedBridge[*HertzHandler]
}

func (*HertzBridge) IsHandlerValid(handler *HertzHandler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (*HertzBridge) WrapHandler(handlers ...app.HandlerFunc) *HertzHandler {
	return &HertzHandler{
		HandlersChain: handlers,
	}
}

func (*HertzBridge) WrapMiddleware(handlers ...app.HandlerFunc) treemux.MiddlewareFunc[*HertzHandler] {
	return func(next *HertzHandler) *HertzHandler {
		next.addMiddlewares(handlers)
		return next
	}
}

func (b *HertzBridge) Serve(ctx context.Context, rc *app.RequestContext) {
	method := string(rc.Method())
	requestURI := string(rc.Request.RequestURI())
	urlPath := string(rc.URI().Path())

	mux := b.GetRouter()
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
		lr.Handler.run(lr.RoutePath, ctx, rc)
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

// GetRouter returns the current router attached to this bridge.
func (b *HertzBridge) GetRouter() *treemux.TreeMux[*HertzHandler] {
	return (*treemux.TreeMux[*HertzHandler])(atomic.LoadPointer(&b.mux))
}

// SetRouter changes the router of the bridge, it's safe to change
// the bridge's router concurrently.
// It also assigns the bridge to router.Bridge.
func (b *HertzBridge) SetRouter(mux *treemux.TreeMux[*HertzHandler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}
