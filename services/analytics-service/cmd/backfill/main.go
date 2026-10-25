// Command backfill rebuilds ClickHouse fact / dimension tables from Postgres and reconciles the two stores
// (docs/design/07_DATA_PIPELINE.md §5, D4).
//
//	backfill backfill  --from 2026-01-01 --to 2026-01-31 [--tables order_status,order_items,payments,refunds,products,inventory]
//	backfill reconcile --from 2026-01-01 --to 2026-01-31 [--json] [--tolerance 0.0]
//
// Dates are YYYY-MM-DD in Asia/Ho_Chi_Minh, both inclusive. Connection flags default to the environment:
// POSTGRES_DSN, CLICKHOUSE_URL, CLICKHOUSE_DB, CLICKHOUSE_USER, CLICKHOUSE_PASSWORD.
// Exit codes: 0 ok / match, 1 reconcile found a mismatch over the tolerance, 2 error.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"analytics-service/internal/backfill"
	"analytics-service/internal/ch"
	"analytics-service/internal/store"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type conn struct{ pgDSN, chURL, chDB, chUser, chPass string }

func addConnFlags(fs *flag.FlagSet) *conn {
	c := &conn{}
	fs.StringVar(&c.pgDSN, "postgres-dsn", env("POSTGRES_DSN", ""), "Postgres DSN (env POSTGRES_DSN)")
	fs.StringVar(&c.chURL, "clickhouse-url", env("CLICKHOUSE_URL", "http://localhost:8123"), "ClickHouse HTTP URL (env CLICKHOUSE_URL)")
	fs.StringVar(&c.chDB, "clickhouse-db", env("CLICKHOUSE_DB", "marketplace"), "ClickHouse database (env CLICKHOUSE_DB)")
	fs.StringVar(&c.chUser, "clickhouse-user", env("CLICKHOUSE_USER", "default"), "ClickHouse user (env CLICKHOUSE_USER)")
	fs.StringVar(&c.chPass, "clickhouse-password", os.Getenv("CLICKHOUSE_PASSWORD"), "ClickHouse password (env CLICKHOUSE_PASSWORD)")
	return c
}

func (c *conn) openPG() (*sql.DB, error) {
	if c.pgDSN == "" {
		return nil, fmt.Errorf("--postgres-dsn / POSTGRES_DSN is required")
	}
	db, err := sql.Open("pgx", c.pgDSN)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: backfill <backfill|reconcile> --from YYYY-MM-DD --to YYYY-MM-DD [flags]")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch args[0] {
	case "backfill":
		return cmdBackfill(ctx, args[1:], stdout, stderr)
	case "reconcile":
		return cmdReconcile(ctx, args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q (want backfill or reconcile)\n", args[0])
	return 2
}

func cmdBackfill(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "first day, YYYY-MM-DD (Asia/Ho_Chi_Minh, inclusive)")
	to := fs.String("to", "", "last day, YYYY-MM-DD (inclusive)")
	tables := fs.String("tables", "", "comma separated: "+strings.Join(backfill.AllTables, ",")+" (default all)")
	batch := fs.Int("batch", 5000, "rows per ClickHouse INSERT")
	c := addConnFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := backfill.ParseDays(*from, *to)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	ts, err := backfill.ParseTables(*tables)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	pg, err := c.openPG()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer pg.Close()
	root := ch.New(c.chURL, "", c.chUser, c.chPass)
	if err := root.Ping(ctx); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if err := store.Migrate(ctx, root, c.chDB, 13); err != nil {
		fmt.Fprintln(stderr, "error: clickhouse migrate:", err)
		return 2
	}
	sink := store.NewSink(root.WithDatabase(c.chDB), time.Hour, *batch, *batch*20)
	res, err := backfill.Run(ctx, pg, sink, backfill.Options{Range: r, Tables: ts, BatchRows: *batch,
		Logf: func(f string, a ...any) { fmt.Fprintf(stderr, f+"\n", a...) }})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(res)
	return 0
}

func cmdReconcile(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(stderr)
	from := fs.String("from", "", "first day, YYYY-MM-DD (Asia/Ho_Chi_Minh, inclusive)")
	to := fs.String("to", "", "last day, YYYY-MM-DD (inclusive)")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	tol := fs.Float64("tolerance", 0, "allowed relative difference |ch-pg|/max(|pg|,1) per day and metric (0 = exact)")
	verbose := fs.Bool("verbose", false, "table: list every day, not only the mismatching ones")
	c := addConnFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r, err := backfill.ParseDays(*from, *to)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if *tol < 0 {
		fmt.Fprintln(stderr, "error: --tolerance must be >= 0")
		return 2
	}
	pg, err := c.openPG()
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer pg.Close()
	db := ch.New(c.chURL, c.chDB, c.chUser, c.chPass)
	rep, err := backfill.Reconcile(ctx, pg, db, r, *tol)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rep)
	} else {
		printReport(stdout, rep, *verbose)
	}
	if !rep.OK {
		return 1
	}
	return 0
}

func printReport(w io.Writer, rep *backfill.Report, verbose bool) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintf(tw, "reconcile %s .. %s (tolerance %g)\n\nMETRIC\tPOSTGRES\tCLICKHOUSE\tDIFF\tSTATUS\n", rep.From, rep.To, rep.Tolerance)
	for _, l := range rep.Totals {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%+d\t%s\n", l.Metric, l.Postgres, l.ClickHouse, l.Diff, status(l))
	}
	fmt.Fprintf(tw, "\nDAY\tMETRIC\tPOSTGRES\tCLICKHOUSE\tDIFF\tSTATUS\n")
	for _, l := range rep.Lines {
		if verbose || l.Mismatch {
			fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%+d\t%s\n", l.Day, l.Metric, l.Postgres, l.ClickHouse, l.Diff, status(l))
		}
	}
	verdict := "MATCH"
	if !rep.OK {
		verdict = "MISMATCH"
	}
	fmt.Fprintf(tw, "\n%s: %d checks, %d over tolerance, max ratio %.6f\n", verdict, rep.Checked, rep.Mismatches, rep.MaxRatio)
	tw.Flush()
}

func status(l backfill.Line) string {
	switch {
	case l.Mismatch:
		return "MISMATCH"
	case l.Diff != 0:
		return "within tolerance"
	}
	return "ok"
}
