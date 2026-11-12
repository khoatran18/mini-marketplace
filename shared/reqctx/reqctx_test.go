package reqctx

import (
	"context"
	"testing"
)

type k struct{}

func TestUseGetPut(t *testing.T) {
	if Get(context.Background()) != "" {
		t.Fatal("no hooks: Get must be empty")
	}
	if Put(context.Background(), "abcdefgh12") != context.Background() {
		t.Fatal("no hooks: Put must return ctx unchanged")
	}
	Use(func(c context.Context) string { s, _ := c.Value(k{}).(string); return s },
		func(c context.Context, id string) context.Context { return context.WithValue(c, k{}, id) })
	t.Cleanup(func() { current.Store(nil) })

	ctx := Put(context.Background(), "req-12345678")
	if got := Get(ctx); got != "req-12345678" {
		t.Fatalf("Get = %q", got)
	}
	for _, bad := range []string{"", "short", "has space in it", "ünicode-ünicode"} {
		if Get(Put(context.Background(), bad)) != "" {
			t.Fatalf("malformed id %q must be ignored", bad)
		}
	}
}
