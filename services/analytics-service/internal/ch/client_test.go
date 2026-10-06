package ch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClientBuildsRequests(t *testing.T) {
	var gotQuery, gotBody, gotAuth string
	var gotParams map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := new(strings.Builder)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		gotBody = b.String()
		gotQuery = r.URL.Query().Get("query")
		gotAuth = r.Header.Get("Authorization")
		gotParams = map[string]string{}
		for k, v := range r.URL.Query() {
			if strings.HasPrefix(k, "param_") {
				gotParams[k] = v[0]
			}
		}
		if strings.Contains(gotBody, "boom") {
			http.Error(w, "Code: 62. Syntax error", 400)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"a":"7","b":1.5,"c":"x"}],"rows":1}`)
	}))
	defer srv.Close()
	c := New(srv.URL+"/", "db1", "u", "p")

	rows, err := c.Query(context.Background(), "SELECT 1 WHERE x = {x:String}", map[string]string{"x": "'; DROP TABLE t; --"})
	if err != nil {
		t.Fatal(err)
	}
	if gotParams["param_x"] != "'; DROP TABLE t; --" || strings.Contains(gotBody, "DROP") && !strings.HasPrefix(gotBody, "SELECT 1 WHERE x = {x:String}") {
		t.Fatalf("values must travel as parameters, not inside the SQL: %q %v", gotBody, gotParams)
	}
	if !strings.HasSuffix(gotBody, "FORMAT JSON") || gotAuth == "" {
		t.Fatalf("body=%q auth=%q", gotBody, gotAuth)
	}
	if rows[0].Int("a") != 7 || rows[0].Float("b") != 1.5 || rows[0].Str("c") != "x" || rows[0].Int("missing") != 0 {
		t.Fatalf("%+v", rows)
	}

	if err := c.Insert(context.Background(), "t", []any{map[string]any{"a": 1}, map[string]any{"a": 2}}); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "INSERT INTO t FORMAT JSONEachRow" || gotBody != "{\"a\":1}\n{\"a\":2}\n" {
		t.Fatalf("insert: %q %q", gotQuery, gotBody)
	}
	if err := c.Insert(context.Background(), "t", nil); err != nil {
		t.Fatal("empty insert is a no-op")
	}
	err = c.Exec(context.Background(), "boom", nil)
	if e, ok := err.(*Error); !ok || e.Status != 400 {
		t.Fatalf("server errors are typed: %v", err)
	}
}

func TestClientAgainstClickHouse(t *testing.T) {
	url := os.Getenv("TEST_CLICKHOUSE_URL")
	if url == "" {
		t.Skip("TEST_CLICKHOUSE_URL not set")
	}
	c := New(url, "", "", "")
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Query(context.Background(), "SELECT {n:UInt32} + 1 AS x, {s:String} AS s", map[string]string{"n": "41", "s": "it's"})
	if err != nil || rows[0].Int("x") != 42 || rows[0].Str("s") != "it's" {
		t.Fatalf("%v %v", rows, err)
	}
	if _, err := c.Query(context.Background(), "SELECT nope FROM nowhere", nil); err == nil {
		t.Fatal("errors must surface")
	}
}
