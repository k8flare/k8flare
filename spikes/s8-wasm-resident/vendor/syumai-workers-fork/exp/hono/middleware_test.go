package hono

import "testing"

func TestChainMiddlewares(t *testing.T) {
	result := ""
	middlewares := []Middleware{
		func(c *Context, next func()) {
			result += "1"
			next()
			result += "1"
		},
		func(c *Context, next func()) {
			result += "2"
			next()
			result += "2"
		},
		func(c *Context, next func()) {
			result += "3"
			next()
			result += "3"
		},
	}
	m := ChainMiddlewares(middlewares...)
	m(nil, func() {
		result += "0"
	})
	const want = "1230321"
	if result != want {
		t.Errorf("result: got %q, want %q", result, want)
	}
}
