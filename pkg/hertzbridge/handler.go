package hertzbridge

import (
	"context"
	"reflect"

	"github.com/cloudwego/hertz/pkg/app"
)

type HertzHandler struct {
	HandlersChain app.HandlersChain
}

func (h *HertzHandler) addMiddlewares(handlers []app.HandlerFunc) {
	for _, mw := range handlers {
		if h.inHandlersChain(mw) {
			panic("treemux/pkg/hertzbridge: middleware already added for this handler")
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

func (h *HertzHandler) inHandlersChain(mw app.HandlerFunc) bool {
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

func (h *HertzHandler) run(fullPath string, ctx context.Context, c *app.RequestContext) {
	realHandlers := append(c.Handlers(), h.HandlersChain...)
	c.SetHandlers(realHandlers)
	c.SetFullPath(fullPath)
	c.Next(ctx)
}
