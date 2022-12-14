package treemux

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func dummyHandler(w http.ResponseWriter, r *http.Request, urlParams Params) {

}

func addPath(t *testing.T, tree *node[HandlerFunc], path string) {
	t.Logf("Adding path %s", path)
	n := tree.addPath(path[1:], nil, false)
	var handler = func(w http.ResponseWriter, r *http.Request, urlParams Params) {
		w.Write([]byte(path))
	}
	n.setHandler("GET", handler, false)
}

var test *testing.T

func testPath(t *testing.T, tree *node[HandlerFunc], path string,
	expectPath string, expectRouteType RouteType, expectParams map[string]string) {
	if t.Failed() {
		t.Log(tree.dumpTree("", " "))
		t.FailNow()
	}

	t.Log("Testing", path)
	n, foundHandler, paramList := tree.search("GET", path[1:], defaultBridge{}.IsHandlerValid)
	if expectPath != "" && n == nil {
		t.Errorf("No match for %s, expected %s", path, expectPath)
		return
	} else if expectPath == "" && n != nil {
		t.Errorf("Expected no match for %s but got %v with params %v", path, n, expectParams)
		t.Error("Node and subtree was\n" + n.dumpTree("", " "))
		return
	}

	if n == nil {
		return
	}

	if n.routeType != expectRouteType {
		t.Errorf("Expected route type %d but got %d for %s", expectRouteType, n.routeType, path)
		return
	}

	handler, ok := n.leafHandlers["GET"]
	if !ok {
		t.Errorf("Path %s returned node without handler", path)
		t.Error("Node and subtree was\n" + n.dumpTree("", " "))
		return
	}

	if foundHandler == nil {
		t.Errorf("Path %s returned valid node but foundHandler was false", path)
		t.Error("Node and subtree was\n" + n.dumpTree("", " "))
		return
	}

	recorder := httptest.NewRecorder()
	handler(recorder, nil, Params{})
	matchedPath := recorder.Body.String()

	if matchedPath != expectPath {
		t.Errorf("Path %s matched %s, expected %s", path, matchedPath, expectPath)
		t.Error("Node and subtree was\n" + n.dumpTree("", " "))
	}

	if expectParams == nil {
		if len(paramList) != 0 {
			t.Errorf("Path %s expected no parameters, saw %v", path, paramList)
		}
	}
	if len(paramList) != len(n.leafParamNames) {
		t.Errorf("Got %d params back but node specifies %d",
			len(paramList), len(n.leafParamNames))
	}

	params := map[string]string{}
	for i := 0; i < len(paramList); i++ {
		params[n.leafParamNames[len(paramList)-i-1]] = paramList[i]
	}
	t.Log("\tGot params", params)

	for key, val := range expectParams {
		sawVal, ok := params[key]
		if !ok {
			t.Errorf("Path %s matched without key %s", path, key)
		} else if sawVal != val {
			t.Errorf("Path %s expected param %s to be %s, saw %s", path, key, val, sawVal)
		}

		delete(params, key)
	}

	for key, val := range params {
		t.Errorf("Path %s returned unexpected param %s=%s", path, key, val)
	}

}

func checkHandlerNodes(t *testing.T, n *node[HandlerFunc]) {
	hasHandlers := len(n.leafHandlers) != 0
	hasWildcards := len(n.leafParamNames) != 0

	if hasWildcards && !hasHandlers {
		t.Errorf("Node %s has wildcards without handlers", n.path)
	}
}

func TestTree(t *testing.T) {
	test = t
	tree := &node[HandlerFunc]{path: "/"}

	addPath(t, tree, "/")
	addPath(t, tree, "/i")
	addPath(t, tree, "/i/:aaa")
	addPath(t, tree, "/images")
	addPath(t, tree, "/images/abc.jpg")
	addPath(t, tree, "/images/:imgname")
	addPath(t, tree, "/images/\\*path")
	addPath(t, tree, "/images/\\*patch")
	addPath(t, tree, "/images/*path")
	addPath(t, tree, "/ima")
	addPath(t, tree, "/ima/:par")
	addPath(t, tree, "/images1")
	addPath(t, tree, "/images2")
	addPath(t, tree, "/apples")
	addPath(t, tree, "/app/les")
	addPath(t, tree, "/apples1")
	addPath(t, tree, "/appeasement")
	addPath(t, tree, "/appealing")
	addPath(t, tree, "/date/\\:year/\\:month")
	addPath(t, tree, "/date/:year/:month")
	addPath(t, tree, "/date/:year/month")
	addPath(t, tree, "/date/:year/:month/abc")
	addPath(t, tree, "/date/:year/:month/:post")
	addPath(t, tree, "/date/:year/:month/*post")
	addPath(t, tree, "/:page")
	addPath(t, tree, "/:page/:index")
	addPath(t, tree, "/post/:post/page/:page")
	addPath(t, tree, "/plaster")
	addPath(t, tree, "/users/:pk/:related")
	addPath(t, tree, "/users/:id/updatePassword")
	addPath(t, tree, `/users/~^.+$`) // not matched by others go to this route
	addPath(t, tree, "/:something/abc")
	addPath(t, tree, "/:something/def")
	addPath(t, tree, "/apples/ab:cde/:fg/*hi")
	addPath(t, tree, "/apples/ab*cde/:fg/*hi")
	addPath(t, tree, "/apples/ab\\*cde/:fg/*hi")
	addPath(t, tree, "/apples/ab*dde")
	addPath(t, tree, `/smith/~^.+$`)
	addPath(t, tree, `/smith/abc/~^some-(?P<var1>\w+)-(?P<var2>\d+)-(.*)$`)
	addPath(t, tree, `/smith/abc/~^some-.*second.*$`) // the previous one will be matched first
	addPath(t, tree, "/images3/*path")
	addPath(t, tree, `/images3/~^(?P<category>\w+)-(?P<name>.+)$`)

	testPath(t, tree, "/smith/abc/some-holiday-202110-hawaii-beach", `/smith/abc/~^some-(?P<var1>\w+)-(?P<var2>\d+)-(.*)$`,
		Regexp, map[string]string{"var1": "holiday", "var2": "202110"})
	testPath(t, tree, "/smith/abc/some-matchthesecondregex", `/smith/abc/~^some-.*second.*$`, Regexp, nil)
	testPath(t, tree, "/smith/abc/third-no-specific-match", `/smith/~^.+$`, Regexp, nil)
	testPath(t, tree, "/users/123/something/notmatch", `/users/~^.+$`, Regexp, nil)
	testPath(t, tree, "/images3/categorya-img1.jpg", `/images3/~^(?P<category>\w+)-(?P<name>.+)$`,
		Regexp, map[string]string{"category": "categorya", "name": "img1.jpg"})
	testPath(t, tree, "/images3/nocategoryimg.jpg", "/images3/*path",
		CatchAll, map[string]string{"path": "nocategoryimg.jpg"})

	testPath(t, tree, "/users/abc/updatePassword", "/users/:id/updatePassword",
		Wildcard, map[string]string{"id": "abc"})
	testPath(t, tree, "/users/all/something", "/users/:pk/:related",
		Wildcard, map[string]string{"pk": "all", "related": "something"})

	testPath(t, tree, "/aaa/abc", "/:something/abc",
		Wildcard, map[string]string{"something": "aaa"})
	testPath(t, tree, "/aaa/def", "/:something/def",
		Wildcard, map[string]string{"something": "aaa"})

	testPath(t, tree, "/paper", "/:page",
		Wildcard, map[string]string{"page": "paper"})

	testPath(t, tree, "/", "/", Static, nil)
	testPath(t, tree, "/i", "/i", Static, nil)
	testPath(t, tree, "/images", "/images", Static, nil)
	testPath(t, tree, "/images/abc.jpg", "/images/abc.jpg", Static, nil)
	testPath(t, tree, "/images/something", "/images/:imgname",
		Wildcard, map[string]string{"imgname": "something"})
	testPath(t, tree, "/images/long/path", "/images/*path",
		CatchAll, map[string]string{"path": "long/path"})
	testPath(t, tree, "/images/even/longer/path", "/images/*path",
		CatchAll, map[string]string{"path": "even/longer/path"})
	testPath(t, tree, "/ima", "/ima", Static, nil)
	testPath(t, tree, "/apples", "/apples", Static, nil)
	testPath(t, tree, "/app/les", "/app/les", Static, nil)
	testPath(t, tree, "/abc", "/:page",
		Wildcard, map[string]string{"page": "abc"})
	testPath(t, tree, "/abc/100", "/:page/:index",
		Wildcard, map[string]string{"page": "abc", "index": "100"})
	testPath(t, tree, "/post/a/page/2", "/post/:post/page/:page",
		Wildcard, map[string]string{"post": "a", "page": "2"})
	testPath(t, tree, "/date/2014/5", "/date/:year/:month",
		Wildcard, map[string]string{"year": "2014", "month": "5"})
	testPath(t, tree, "/date/2014/month", "/date/:year/month",
		Wildcard, map[string]string{"year": "2014"})
	testPath(t, tree, "/date/2014/5/abc", "/date/:year/:month/abc",
		Wildcard, map[string]string{"year": "2014", "month": "5"})
	testPath(t, tree, "/date/2014/5/def", "/date/:year/:month/:post",
		Wildcard, map[string]string{"year": "2014", "month": "5", "post": "def"})
	testPath(t, tree, "/date/2014/5/def/hij", "/date/:year/:month/*post",
		CatchAll, map[string]string{"year": "2014", "month": "5", "post": "def/hij"})
	testPath(t, tree, "/date/2014/5/def/hij/", "/date/:year/:month/*post",
		CatchAll, map[string]string{"year": "2014", "month": "5", "post": "def/hij/"})

	testPath(t, tree, "/date/2014/ab%2f", "/date/:year/:month",
		Wildcard, map[string]string{"year": "2014", "month": "ab/"})
	testPath(t, tree, "/post/ab%2fdef/page/2%2f", "/post/:post/page/:page",
		Wildcard, map[string]string{"post": "ab/def", "page": "2/"})

	// Test paths with escaped wildcard characters.
	testPath(t, tree, "/images/*path", "/images/\\*path", Static, nil)
	testPath(t, tree, "/images/*patch", "/images/\\*patch", Static, nil)
	testPath(t, tree, "/date/:year/:month", "/date/\\:year/\\:month", Static, nil)
	testPath(t, tree, "/apples/ab*cde/lala/baba/dada", "/apples/ab*cde/:fg/*hi",
		CatchAll, map[string]string{"fg": "lala", "hi": "baba/dada"})
	testPath(t, tree, "/apples/ab\\*cde/lala/baba/dada", "/apples/ab\\*cde/:fg/*hi",
		CatchAll, map[string]string{"fg": "lala", "hi": "baba/dada"})
	testPath(t, tree, "/apples/ab:cde/:fg/*hi", "/apples/ab:cde/:fg/*hi",
		CatchAll, map[string]string{"fg": ":fg", "hi": "*hi"})
	testPath(t, tree, "/apples/ab*cde/:fg/*hi", "/apples/ab*cde/:fg/*hi",
		CatchAll, map[string]string{"fg": ":fg", "hi": "*hi"})
	testPath(t, tree, "/apples/ab*cde/one/two/three", "/apples/ab*cde/:fg/*hi",
		CatchAll, map[string]string{"fg": "one", "hi": "two/three"})
	testPath(t, tree, "/apples/ab*dde", "/apples/ab*dde", Static, nil)

	testPath(t, tree, "/ima/bcd/fgh", "", 0, nil)
	testPath(t, tree, "/date/2014//month", "", 0, nil)
	testPath(t, tree, "/date/2014/05/", "", 0, nil) // Empty catchall should not match
	testPath(t, tree, "/post//abc/page/2", "", 0, nil)
	testPath(t, tree, "/post/abc//page/2", "", 0, nil)
	testPath(t, tree, "/post/abc/page//2", "", 0, nil)
	testPath(t, tree, "//post/abc/page/2", "", 0, nil)
	testPath(t, tree, "//post//abc//page//2", "", 0, nil)

	t.Log("Test retrieval of duplicate paths")
	p := "date/:year/:month/abc"
	n := tree.addPath(p, nil, false)
	if n == nil {
		t.Errorf("Duplicate add of %s didn't return a node", p)
	} else {
		handler, ok := n.leafHandlers["GET"]
		matchPath := ""
		if ok {
			recorder := httptest.NewRecorder()
			handler(recorder, nil, Params{})
			matchPath = recorder.Body.String()
		}

		if len(matchPath) < 2 || matchPath[1:] != p {
			t.Errorf("Duplicate add of %s returned node for %s\n%s", p, matchPath,
				n.dumpTree("", " "))

		}
	}

	checkHandlerNodes(t, tree)

	t.Log(tree.dumpTree("", " "))
	test = nil
}

func TestDumpTree(t *testing.T) {
	router := New[HandlerFunc]()
	router.GET("/pumpkin", simpleHandler)
	router.GET("/passing", simpleHandler)
	router.GET("/:slug", simpleHandler)
	router.GET("/:slug/:abc/def/:ghi", simpleHandler)
	router.POST("/re/~^some-(?P<var1>\\w+)-(?P<var2>\\d+)-(.*)$", simpleHandler)

	t.Log("\n" + router.root.dumpTree("", " "))
}

func TestPanics(t *testing.T) {
	sawPanic := false

	panicHandler := func() {
		if err := recover(); err != nil {
			sawPanic = true
		}
	}

	addPathPanic := func(p ...string) {
		sawPanic = false
		defer panicHandler()
		tree := &node[HandlerFunc]{path: "/"}
		for _, path := range p {
			tree.addPath(path, nil, false)
		}
	}

	addPathPanic("abc/*path/")
	if !sawPanic {
		t.Error("Expected panic with slash after catch-all")
	}

	addPathPanic("abc/*path/def")
	if !sawPanic {
		t.Error("Expected panic with path segment after catch-all")
	}

	addPathPanic("abc/*path", "abc/*paths")
	if !sawPanic {
		t.Error("Expected panic when adding conflicting catch-alls")
	}

	func() {
		sawPanic = false
		defer panicHandler()
		tree := &node[HandlerFunc]{path: "/"}
		tree.setHandler("GET", dummyHandler, false)
		tree.setHandler("GET", dummyHandler, false)
	}()
	if !sawPanic {
		t.Error("Expected panic when adding a duplicate handler for a pattern")
	}

	twoPathPanic := func(first, second string) {
		addPathPanic(first, second)
		if !sawPanic {
			t.Errorf("Expected panic with ambiguous wildcards on paths %s and %s", first, second)
		}
	}

	twoPathPanic("abc/:ab/def/:cd", "abc/:ad/def/:cd")
	twoPathPanic("abc/:ab/def/:cd", "abc/:ab/def/:ef")
	twoPathPanic(":abc", ":def")
	twoPathPanic(":abc/ggg", ":def/ggg")
}

func BenchmarkTreeNullRequest(b *testing.B) {
	b.ReportAllocs()
	tree := &node[HandlerFunc]{
		path: "/",
		leafHandlers: map[string]HandlerFunc{
			"GET": dummyHandler,
		},
	}

	validFunc := defaultBridge{}.IsHandlerValid

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("GET", "", validFunc)
	}
}

func BenchmarkTreeOneStatic(b *testing.B) {
	b.ReportAllocs()
	tree := &node[HandlerFunc]{
		path: "/",
		leafHandlers: map[string]HandlerFunc{
			"GET": dummyHandler,
		},
	}
	tree.addPath("abc", nil, false)

	validFunc := defaultBridge{}.IsHandlerValid

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("GET", "abc", validFunc)
	}
}

func BenchmarkTreeOneParam(b *testing.B) {
	tree := &node[HandlerFunc]{
		path: "/",
		leafHandlers: map[string]HandlerFunc{
			"GET": dummyHandler,
		},
	}
	b.ReportAllocs()
	tree.addPath(":abc", nil, false)

	validFunc := defaultBridge{}.IsHandlerValid

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("GET", "abc", validFunc)
	}
}

func BenchmarkTreeLongParams(b *testing.B) {
	tree := &node[HandlerFunc]{
		path: "/",
		leafHandlers: map[string]HandlerFunc{
			"GET": dummyHandler,
		},
	}
	b.ReportAllocs()
	tree.addPath(":abc/:def/:ghi", nil, false)

	validFunc := defaultBridge{}.IsHandlerValid

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree.search("GET", "abcdefghijklmnop/aaaabbbbccccddddeeeeffffgggg/hijkl", validFunc)
	}
}
