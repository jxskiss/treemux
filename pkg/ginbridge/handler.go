package ginbridge

import (
	"net/http"
	"reflect"
	"unsafe"

	"github.com/gin-gonic/gin"
	"github.com/jxskiss/treemux"
)

type Handler struct {
	HandlersChain gin.HandlersChain
}

func (h *Handler) addMiddlewares(handlers []gin.HandlerFunc) {
	for _, mw := range handlers {
		if h.inHandlersChain(mw) {
			panic("treemux/pkg/ginbridge: middleware already added for this handler")
		}
	}

	chain := h.HandlersChain
	oldLen := len(chain)
	newLen := oldLen + len(handlers)
	if cap(chain) < newLen {
		chain = append(handlers[:len(handlers):len(handlers)], chain...)
	} else {
		chain = chain[:newLen]
		copy(chain[len(handlers):], chain[:oldLen])
		copy(chain[:len(handlers)], handlers)
	}
	h.HandlersChain = chain
}

func (h *Handler) inHandlersChain(mw gin.HandlerFunc) bool {
	mwAddr := getFuncAddr(mw)
	for _, x := range h.HandlersChain {
		if getFuncAddr(x) == mwAddr {
			return true
		}
	}
	return false
}

func getFuncAddr(v interface{}) uintptr {
	return reflect.ValueOf(reflect.ValueOf(v)).Field(1).Pointer()
}

func (h *Handler) run(fullPath string, c *gin.Context) {
	realHandlers := append(getGinContextHandlers(c), h.HandlersChain...)
	setGinContextHandlers(c, realHandlers)
	setGinContextFullPath(c, fullPath)
	c.Next()
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

// WrapHandler wraps gin handler functions into a *Handler.
func WrapHandler(handlers ...gin.HandlerFunc) *Handler {
	return &Handler{
		HandlersChain: handlers,
	}
}

// WrapHTTPHandler wraps a [http.Handler] into a *Handler.
// This wrapper function allows http.Handler to be used as gin handler endpoint.
// Note that the returned handler can only be used as a leaf handler, but not a
// middleware handler.
func WrapHTTPHandler(handler http.Handler) *Handler {
	wrapper := func(c *gin.Context) {
		w, r := c.Writer, c.Request
		handler.ServeHTTP(w, r)
	}
	return &Handler{
		HandlersChain: gin.HandlersChain{wrapper},
	}
}

// WrapMiddleware wraps gin handler functions into a treemux MiddlewareFunc.
func WrapMiddleware(handlers ...gin.HandlerFunc) treemux.MiddlewareFunc[*Handler] {
	return func(next *Handler) *Handler {
		next.addMiddlewares(handlers)
		return next
	}
}
