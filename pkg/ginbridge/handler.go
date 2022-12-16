package ginbridge

import (
	"reflect"
	"unsafe"

	"github.com/gin-gonic/gin"
)

type GinHandler struct {
	HandlersChain gin.HandlersChain
}

func (h *GinHandler) addMiddlewares(handlers []gin.HandlerFunc) {
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

func (h *GinHandler) inHandlersChain(mw gin.HandlerFunc) bool {
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

func (h *GinHandler) run(fullPath string, c *gin.Context) {
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
