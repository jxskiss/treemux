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

// New creates a new Bridge.
func New() *Bridge {
	bridge := &Bridge{}
	return bridge
}

// Bridge implements treemux.Bridge[*Handler] and can be used
// as a dynamic-configurable router.
type Bridge struct {
	mux unsafe.Pointer // *treemux.Router[*Handler]

	treemux.UnimplementedBridge[*Handler]
}

func (*Bridge) IsHandlerValid(handler *Handler) bool {
	return handler != nil && len(handler.HandlersChain) > 0
}

func (b *Bridge) Serve(ctx context.Context, rc *app.RequestContext) {
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
func (b *Bridge) GetRouter() *treemux.TreeMux[*Handler] {
	return (*treemux.TreeMux[*Handler])(atomic.LoadPointer(&b.mux))
}

// SetRouter changes the router of the bridge, it's safe to change
// the bridge's router concurrently.
// It also assigns the bridge to router.Bridge.
func (b *Bridge) SetRouter(mux *treemux.TreeMux[*Handler]) {
	mux.Bridge = b
	atomic.StorePointer(&b.mux, unsafe.Pointer(mux))
}
