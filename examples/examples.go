package examples

import "github.com/jxskiss/treemux"

type Route struct {
	Method string
	Path   string
}

func SetupRoutes[T any](r *treemux.TreeMux[T], handler T) {
	for _, x := range GithubAPIList {
		r.Handle(x.Method, x.Path, handler)
	}
}
