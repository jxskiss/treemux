package treemux

import (
	"testing"
)

type testRewriteCase struct {
	path string
	want string
}

func TestRewriteFunc(t *testing.T) {

	type testCase struct {
		path    string
		rewrite string
		cases   []testRewriteCase
	}

	tests := []testCase{
		{
			path:    "/a",
			rewrite: "/b",
			cases: []testRewriteCase{
				{path: "/a", want: "/b"},
			},
		},
		{
			path:    "/a/~(.*)",
			rewrite: "/bb",
			cases: []testRewriteCase{
				{path: "/a", want: "/a"},
				{path: "/a/", want: "/bb"},
				{path: "/a/a", want: "/bb"},
				{path: "/a/b/c", want: "/bb"},
			},
		},
		{
			path:    "/r/~(.*)",
			rewrite: `/r/v1/$1`,
			cases: []testRewriteCase{
				{path: "/a", want: "/a"},
				{path: "/r", want: "/r"},
				{path: "/r/a", want: "/r/v1/a"},
				{path: "/r/a/b", want: "/r/v1/a/b"},
			},
		},
		{
			path:    "/r/~(.*)/a/(.*)",
			rewrite: `/r/v1/$1/a/$2`,
			cases: []testRewriteCase{
				{path: "/r/1/2", want: "/r/1/2"},
				{path: "/r/1/a/2", want: "/r/v1/1/a/2"},
				{path: "/r/1/a/2/3", want: "/r/v1/1/a/2/3"},
			},
		},
		{
			path:    "/r/~(.*)/a/(.*)",
			rewrite: `/r/v1/$2/a/$1`,
			cases: []testRewriteCase{
				{path: "/r/1/a/2", want: "/r/v1/2/a/1"},
				{path: "/r/1/a/2/3", want: "/r/v1/2/3/a/1"},
			},
		},
		{
			path:    "/from/:one/to/:two",
			rewrite: "/from/:two/to/:one",
			cases: []testRewriteCase{
				{path: "/from/123/to/456", want: "/from/456/to/123"},
				{path: "/from/abc/to/def", want: "/from/def/to/abc"},
			},
		},
		{
			path:    "/from/:one/to/:two",
			rewrite: "/:one/:two/:three/:two/:one",
			cases: []testRewriteCase{
				{path: "/from/123/to/456", want: "/123/456/:three/456/123"},
				{path: "/from/abc/to/def", want: "/abc/def/:three/def/abc"},
			},
		},
		{
			path:    "/from/~(.*)",
			rewrite: "/to/$1",
			cases: []testRewriteCase{
				{path: "/from/untitled-1%2F/upload", want: "/to/untitled-1%2F/upload"},
			},
		},
		{
			path:    "/date/:year/:month/abc",
			rewrite: "/date/$2/$1/def",
			cases: []testRewriteCase{
				{path: "/date/1/2/abc", want: "/date/2/1/def"},
			},
		},
		{
			path:    "/date/:year/:month/*post",
			rewrite: "/date/:month/:year/$3",
			cases: []testRewriteCase{
				{path: "/date/1/2/post-1", want: "/date/2/1/post-1"},
				{path: "/date/1/2/post/4/5/6", want: "/date/2/1/post/4/5/6"},
			},
		},
		{
			path:    `/smith/abc/~^some-(?P<var1>\w+)-(?P<var2>\d+)-(.*)$`,
			rewrite: "/$1/:var2/$3",
			cases: []testRewriteCase{
				{path: "/smith/abc/some-abc-123-last-whatever", want: "/abc/123/last-whatever"},
				{path: "/smith/abc/some-abc-123-last/whatever", want: "/abc/123/last/whatever"},
			},
		},
	}

	for _, test := range tests {
		t.Logf("Test - path: %s, rewrite: %s", test.path, test.rewrite)

		fn, err := NewRewriteFunc(test.path, test.rewrite)
		if err != nil {
			t.Fatalf("Failed create rewrite func: %v", err)
		}

		for _, fixture := range test.cases {
			want := fixture.want
			got := fn(fixture.path)
			if got != fixture.want {
				t.Errorf("Unexpected rewrite result, want %q, but got %q", want, got)
			}
		}
	}
}
