// Package ch is a minimal ClickHouse client over the HTTP interface (port 8123): DDL/DML, JSONEachRow inserts and
// parameterised SELECTs ({name:Type} placeholders, values travel as param_<name> so nothing is concatenated into SQL).
package ch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to one ClickHouse server.
type Client struct {
	BaseURL  string
	Database string
	User     string
	Password string
	HTTP     *http.Client
}

// New builds a client; database may be empty for the bootstrap (CREATE DATABASE) connection.
func New(baseURL, database, user, password string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), Database: database, User: user, Password: password,
		HTTP: &http.Client{Timeout: 60 * time.Second},
	}
}

// WithDatabase returns a copy bound to another database.
func (c *Client) WithDatabase(db string) *Client {
	cp := *c
	cp.Database = db
	return &cp
}

// Error is a ClickHouse server error (non-200 reply).
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("clickhouse: HTTP %d: %s", e.Status, strings.TrimSpace(e.Message))
}

func (c *Client) do(ctx context.Context, query string, params map[string]string, body []byte) ([]byte, error) {
	q := url.Values{}
	if c.Database != "" {
		q.Set("database", c.Database)
	}
	q.Set("output_format_json_quote_64bit_integers", "0")
	q.Set("date_time_input_format", "best_effort")
	for k, v := range params {
		q.Set("param_"+k, v)
	}
	var (
		req *http.Request
		err error
	)
	if body != nil { // INSERT: statement in the URL, rows in the body
		q.Set("query", query)
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/?"+q.Encode(), bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/?"+q.Encode(), strings.NewReader(query))
	}
	if err != nil {
		return nil, err
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &Error{Status: resp.StatusCode, Message: string(out)}
	}
	return out, nil
}

// Ping checks that the server answers.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("clickhouse ping: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Exec runs a statement that returns nothing (DDL, ALTER, ...).
func (c *Client) Exec(ctx context.Context, sql string, params map[string]string) error {
	_, err := c.do(ctx, sql, params, nil)
	return err
}

// Insert appends rows (maps or structs with json tags) to table with INSERT ... FORMAT JSONEachRow.
func (c *Client) Insert(ctx context.Context, table string, rows []any) error {
	if len(rows) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	_, err := c.do(ctx, "INSERT INTO "+table+" FORMAT JSONEachRow", nil, buf.Bytes())
	return err
}

// Row is one result row; numbers are kept as json.Number so 64-bit values keep their precision.
type Row map[string]any

// Query runs a SELECT (FORMAT JSON is appended) and returns the rows.
func (c *Client) Query(ctx context.Context, sql string, params map[string]string) ([]Row, error) {
	out, err := c.do(ctx, sql+"\nFORMAT JSON", params, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []Row `json:"data"`
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	dec.UseNumber()
	if err := dec.Decode(&res); err != nil {
		return nil, fmt.Errorf("clickhouse: decode reply: %w", err)
	}
	return res.Data, nil
}

// Int returns the integer in column col (0 if absent); accepts numbers and quoted numbers.
func (r Row) Int(col string) int64 {
	switch v := r[col].(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		f, _ := v.Float64()
		return int64(f)
	case string:
		var n json.Number = json.Number(v)
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, _ := n.Float64()
		return int64(f)
	}
	return 0
}

// Float returns the float in column col.
func (r Row) Float(col string) float64 {
	switch v := r[col].(type) {
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := json.Number(v).Float64()
		return f
	}
	return 0
}

// Str returns the string in column col.
func (r Row) Str(col string) string {
	switch v := r[col].(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	}
	return ""
}
