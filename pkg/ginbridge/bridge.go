package ginbridge

import (
	"net/http"
	"sync/atomic"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/jxskiss/treemux"
)

// New creates a new GinBridge.
func New() *GinBridge {
	bridge := &GinBridge{}
	return bridge
}

type GinBridge struct {
	mux unsafe.Pointer // *treemux.TreeMux[*GinHandler]

	treemux.UnimplementedBridge[*GinHandler]
}

func (*GinBridge) IsHandlerValid(handler *GinHandler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (*GinBridge) WrapHandler(handlers ...gin.HandlerFunc) *GinHandler {
	return &GinHandler{
		HandlersChain: handlers,
	}
}

func (*GinBridge) WrapMiddleware(handlers ...gin.HandlerFunc) treemux.MiddlewareFunc[*GinHandler] {
	return func(next *GinHandler) *GinHandler {
		next.addMiddlewares(handlers)
		return next
	}
}

func (b *GinBridge) Serve(c *gin.Context) {
	mux := b.GetRouter()
	lr, _ := mux.Lookup(c.Writer, c.Request)

	if lr.RedirectPath != "" {
		c.Redirect(lr.StatusCode, lr.RedirectPath)
		return
	}

	c.Params = c.Params[:0]
	for i, key := range lr.Params.Keys {
		val := lr.Params.Values[i]
		c.AddParam(key, val)
	}

	if lr.Handler != nil {
		lr.Handler.run(lr.RoutePath, c)
		return
	}

	if lr.StatusCode == http.StatusMethodNotAllowed && len(lr.AllowedMethods) > 0 {
		for _, m := range lr.AllowedMethods {
			c.Request.Header.Add("Allow", m)
		}
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}

	http.NotFound(c.Writer, c.Request)
}

// GetRouter returns the current router attached to this bridge.
func (b *GinBridge) GetRouter() *treemux.TreeMux[*GinHandler] {
	return (*treemux.TreeMux[*GinHandler])(atomic.LoadPointer(&b.mux))
}

// SetRouter changes the router of the bridge, it's safe to change the bridge's
// router concurrently.
// It also assigns the bridge to router.Bridge.
func (b *GinBridge) SetRouter(mux *treemux.TreeMux[*GinHandler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}
