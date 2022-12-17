package ginbridge

import (
	"net/http"
	"sync/atomic"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/jxskiss/treemux"
)

// New creates a new Bridge.
func New() *Bridge {
	bridge := &Bridge{}
	return bridge
}

// Bridge implements treemux.Bridge[*Handler] and can be used
// as a dynamic-configurable router.
type Bridge struct {
	mux unsafe.Pointer // *treemux.TreeMux[*Handler]

	treemux.UnimplementedBridge[*Handler]
}

func (*Bridge) IsHandlerValid(handler *Handler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

type ginContextWrapper struct {
	c      *gin.Context
	called bool
}

func (c *ginContextWrapper) ServeHTTP(_ http.ResponseWriter, _ *http.Request) {
	c.called = true
	c.c.Next()
}

func (*Bridge) ConvertMiddleware(middleware treemux.HTTPHandlerMiddleware) treemux.MiddlewareFunc[*Handler] {
	wrapper := func(c *gin.Context) {
		innerHandler := &ginContextWrapper{c: c}
		w, r := c.Writer, c.Request
		middleware(innerHandler).ServeHTTP(w, r)
		if !innerHandler.called {
			c.Abort()
		}
	}
	return func(next *Handler) *Handler {
		next.addMiddlewares([]gin.HandlerFunc{wrapper})
		return next
	}
}

func (b *Bridge) Serve(c *gin.Context) {
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
func (b *Bridge) GetRouter() *treemux.TreeMux[*Handler] {
	return (*treemux.TreeMux[*Handler])(atomic.LoadPointer(&b.mux))
}

// SetRouter changes the router of the bridge, it's safe to change the bridge's
// router concurrently.
// It also assigns the bridge to router.Bridge.
func (b *Bridge) SetRouter(mux *treemux.TreeMux[*Handler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}
