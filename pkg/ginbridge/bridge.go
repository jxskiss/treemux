package ginbridge

import (
	"net/http"
	"reflect"
	"sync/atomic"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/jxskiss/treemux"
)

type GinHandler struct {
	HandlersChain gin.HandlersChain
}

func (h *GinHandler) addMiddleware(mw gin.HandlerFunc) {
	if inHandlersChain(h.HandlersChain, mw) {
		panic("treemux/pkg/ginbridge: middleware already registered for this handler")
	}

	chain := h.HandlersChain
	if len(chain) == cap(h.HandlersChain) {
		chain = append(gin.HandlersChain{mw}, chain...)
	} else {
		chain = chain[:len(chain)+1]
		copy(chain[:len(chain)-1], chain[1:])
		chain[0] = mw
	}
	h.HandlersChain = chain
}

func inHandlersChain(chain gin.HandlersChain, h gin.HandlerFunc) bool {
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

type GinBridge struct {
	mux unsafe.Pointer // *treemux.TreeMux[*GinHandler]

	treemux.UnimplementedBridge[*GinHandler]
}

func (*GinBridge) IsHandlerValid(handler *GinHandler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (*GinBridge) WrapHandler(handler gin.HandlerFunc) *GinHandler {
	return &GinHandler{
		HandlersChain: gin.HandlersChain{handler},
	}
}

func (*GinBridge) WrapMiddleware(mw gin.HandlerFunc) treemux.MiddlewareFunc[*GinHandler] {
	return func(next *GinHandler) *GinHandler {
		next.addMiddleware(mw)
		return next
	}
}

func (b *GinBridge) Serve(c *gin.Context) {
	mux := b.getMux()
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
		realHandlers := append(getGinContextHandlers(c), lr.Handler.HandlersChain...)
		setGinContextHandlers(c, realHandlers)
		setGinContextFullPath(c, lr.RoutePath)
		c.Next()
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

func (b *GinBridge) getMux() *treemux.TreeMux[*GinHandler] {
	return (*treemux.TreeMux[*GinHandler])(atomic.LoadPointer(&b.mux))
}

// SetMux changes the mux of the bridge, it's safe to use concurrently.
func (b *GinBridge) SetMux(mux *treemux.TreeMux[*GinHandler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}

// New creates a new GinBridge.
func New(mux *treemux.TreeMux[*GinHandler]) *GinBridge {
	bridge := &GinBridge{}
	bridge.SetMux(mux)
	return bridge
}

var (
	handlersOffset uintptr
	fullPathOffset uintptr
)

func init() {
	typ := reflect.TypeOf(gin.Context{})
	handlersField, ok := typ.FieldByName("handlers")
	if !ok {
		panic("treemux/pkg/ginbridge: cannot find field gin.Context.handlers")
	}
	fullPathField, ok := typ.FieldByName("fullPath")
	if !ok {
		panic("treemux/pkg/ginbridge: cannot find field gin.Context.fullPath")
	}
	handlersOffset = handlersField.Offset
	fullPathOffset = fullPathField.Offset
}

func getGinContextHandlers(c *gin.Context) gin.HandlersChain {
	return *(*gin.HandlersChain)(unsafe.Pointer(uintptr(unsafe.Pointer(c)) + handlersOffset))
}

func setGinContextHandlers(c *gin.Context, chain gin.HandlersChain) {
	*(*gin.HandlersChain)(unsafe.Pointer(uintptr(unsafe.Pointer(c)) + handlersOffset)) = chain
}

func setGinContextFullPath(c *gin.Context, path string) {
	*(*string)(unsafe.Pointer(uintptr(unsafe.Pointer(c)) + fullPathOffset)) = path
}
