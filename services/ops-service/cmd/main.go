package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"mini-marketplace/shared/ops"
	"mini-marketplace/shared/tracing"
	"ops-service/internal/api"
	"ops-service/internal/executor"
	"ops-service/internal/migrate"
	"ops-service/internal/notify"
	"ops-service/internal/poller"
	"ops-service/internal/queries"
	"ops-service/internal/store"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
		return n
	}
	return def
}

func envMinutes(k string, def int) time.Duration { return time.Duration(envInt(k, def)) * time.Minute }

func main() {
	// cùng định dạng log với các service khác: JSON, khoá `timestamp` ISO8601
	zcfg := zap.NewProductionConfig()
	zcfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zcfg.EncoderConfig.TimeKey = "timestamp"
	logger, err := zcfg.Build()
	if err != nil {
		log.Fatal(err)
	}
	zap.RedirectStdLog(logger)

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN env variable not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	if err := migrate.Run(context.Background(), db); err != nil {
		log.Fatalf("Can not migrate database: %v", err)
	}
	stopTracing, terr := tracing.Init(context.Background(), "ops-service")
	if terr != nil {
		logger.Warn("tracing disabled", zap.Error(terr))
	}
	defer stopTracing(context.Background())

	st := &store.Store{DB: db}
	interval := 30 * time.Second
	if n, err := strconv.Atoi(env("OPS_POLL_INTERVAL_SEC", "30")); err == nil && n >= 5 {
		interval = time.Duration(n) * time.Second
	}
	promURL := env("PROMETHEUS_URL", "http://prometheus:9090")
	p := &poller.Poller{URL: promURL, Interval: interval, Store: st, Logf: func(f string, a ...any) { log.Printf(f, a...) }}
	go p.Run(context.Background())
	go (&notify.Notifier{Source: st, URL: os.Getenv("OPS_NOTIFY_WEBHOOK_URL"), Interval: 15 * time.Second, Logf: func(f string, a ...any) { log.Printf(f, a...) }}).Run(context.Background())

	token := os.Getenv("OPS_API_TOKEN")
	if token == "" {
		logger.Warn("OPS_API_TOKEN chưa đặt: API /incidents, /audit từ chối mọi yêu cầu (poller vẫn chạy)")
	}
	execHTTP := &executor.HTTP{Token: strings.TrimSpace(os.Getenv("METRICS_TOKEN"))}
	agentToken := os.Getenv("OPS_AGENT_TOKEN")
	if agentToken != "" && agentToken == token {
		logger.Fatal("OPS_AGENT_TOKEN phải khác OPS_API_TOKEN (token agent chỉ có phạm vi hẹp)")
	}
	handler := (&api.API{Store: st, Token: token, AgentToken: agentToken, ProposalTTL: envMinutes("OPS_PROPOSAL_TTL_MIN", 1440), ApprovalTTL: envMinutes("OPS_APPROVAL_TTL_MIN", 5), MaxPending: envInt("OPS_MAX_PENDING_PER_INCIDENT", 5),
		ActionsEnabled: os.Getenv("OPS_ACTIONS_ENABLED") == "true",
		ExecLimits:     store.ExecLimits{MaxPerHour: envInt("OPS_MAX_EXEC_PER_HOUR", 5), Cooldown: envMinutes("OPS_EXEC_COOLDOWN_MIN", 5), Window: envMinutes("OPS_EXEC_WINDOW_MIN", 60)},
		Runner:         execHTTP,
		Outbox:         execHTTP,
		Queries:        &queries.Runner{PrometheusURL: promURL, LokiURL: env("LOKI_URL", "http://loki:3100")}}).Handler()
	// đề xuất PENDING quá hạn chuyển EXPIRED (mỗi phút)
	// lần thực thi EXECUTING bị bỏ dở (tiến trình chết) -> FAILED, không tự chạy lại
	go func() {
		for range time.Tick(time.Minute) {
			if n, err := st.FailStaleExecutions(context.Background(), 10*time.Minute, time.Now()); err != nil {
				log.Printf("ops stale exec: %v", err)
			} else if n > 0 {
				log.Printf("ops stale exec: %d lần thực thi bị bỏ dở", n)
			}
		}
	}()
	go func() {
		for range time.Tick(time.Minute) {
			if n, err := st.ExpireDue(context.Background(), time.Now()); err != nil {
				log.Printf("ops expire: %v", err)
			} else if n > 0 {
				log.Printf("ops expire: %d đề xuất hết hạn", n)
			}
		}
	}()

	adm := ops.New(ops.Options{
		Service: "ops-service",
		Checks: []ops.Check{
			ops.SQLCheck("postgres", func() (*sql.DB, error) { return db, nil }, true),
			ops.FuncCheck("prometheus", false, func(ctx context.Context) error {
				if last := p.LastSuccess.Load(); last == 0 || time.Since(time.Unix(last, 0)) > 4*interval {
					return errors.New("no recent successful poll")
				}
				return nil
			}),
		},
	})
	adm.AttachDB(db)
	adm.KeepReadyFresh(context.Background())
	for _, s := range []string{store.StateOpen, store.StateInvestigating} {
		state := s
		adm.GaugeFunc("mm_ops_incidents", "Unresolved incidents by state.", map[string]string{"state": state}, func() float64 {
			m, err := st.CountByState(context.Background())
			if err != nil {
				return 0
			}
			return float64(m[state])
		})
	}
	for _, s := range []string{store.PPending, store.PAccepted, store.PRejected, store.PNeedsReview, store.PAlternative, store.PExpired} {
		state := s
		adm.GaugeFunc("mm_ops_proposals", "Proposals by state.", map[string]string{"state": state}, func() float64 {
			m, err := st.CountProposalsByState(context.Background())
			if err != nil {
				return 0
			}
			return float64(m[state])
		})
	}
	adm.GaugeFunc("mm_ops_poll_last_success_timestamp_seconds", "Unix time of the last successful Prometheus alerts poll.", nil, func() float64 { return float64(p.LastSuccess.Load()) })
	adm.GaugeFunc("mm_ops_poll_failures", "Failed polls since start.", nil, func() float64 { return float64(p.Failures.Load()) })

	srv := &http.Server{Addr: ":8080", Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("ops-service listening on %s (poll %s every %s)", srv.Addr, promURL, interval)
	if err := adm.ServeHTTPMain(srv); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
