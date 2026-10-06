package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"time"

	_ "time/tzdata" // Asia/Ho_Chi_Minh must resolve inside the minimal alpine image

	"github.com/lpernett/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"analytics-service/internal/ch"
	"analytics-service/internal/config"
	"analytics-service/internal/ingest"
	"analytics-service/internal/server"
	"analytics-service/internal/service"
	"analytics-service/internal/store"
	"analytics-service/pkg/ops"
	pb "analytics-service/pkg/pb"
)

func main() {
	godotenv.Load(".env")
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	// ClickHouse: wait for the server (it starts in parallel in the stack), then migrate
	root := ch.New(cfg.ClickHouseURL, "", cfg.ClickHouseUser, cfg.ClickHousePassword)
	for i := 0; ; i++ {
		if err = root.Ping(ctx); err == nil {
			err = store.Migrate(ctx, root, cfg.ClickHouseDB, cfg.EventsRetentionMonths)
		}
		if err == nil {
			break
		}
		if i >= 60 {
			log.Fatalf("ClickHouse not ready: %v", err)
		}
		log.Printf("waiting for ClickHouse: %v", err)
		time.Sleep(2 * time.Second)
	}
	db := root.WithDatabase(cfg.ClickHouseDB)

	sink := store.NewSink(db, cfg.FlushInterval, cfg.FlushRows, cfg.BufferMax)
	go sink.Run(ctx)

	var rdb *redis.Client
	var cache service.Cache
	if cfg.RedisAddr != "" {
		rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
		cache = service.RedisCache{C: rdb}
	}
	svc := &service.Service{Q: &store.Queries{DB: db}, Sink: sink, Cache: cache, CacheTTL: cfg.CacheTTL}

	// Kafka → ClickHouse (idempotent consumers)
	kc := cfg.NewKafka()
	defer kc.Manager.CloseReaderAll()
	cons := &ingest.Consumers{Out: sink}
	for topic, handler := range cons.Topics() {
		if err := kc.Client.EnsureTopicExist(ctx, topic); err != nil {
			log.Fatalf("Can not ensure Kafka topic %s: %v", topic, err)
		}
		go func(topic string, h func(context.Context, *kafka.Message) error) {
			if err := kc.Consumer.Consume(ctx, topic, "analytics-service-"+topic, h); err != nil {
				log.Printf("Consumer of %s stopped with error: %v", topic, err)
			}
		}(topic, handler)
	}

	lis, err := net.Listen("tcp", ":50055")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	checks := []ops.Check{
		ops.FuncCheck("clickhouse", true, func(ctx context.Context) error { return db.Ping(ctx) }),
		ops.TCPCheck("kafka", os.Getenv("KAFKA_BROKERS_ADDR"), true),
		ops.FuncCheck("insert_pipeline", false, func(ctx context.Context) error {
			if st := sink.Stats(); st.Buffered > 0 && time.Since(st.LastSuccess) > time.Minute {
				return errors.New("inserts are failing")
			}
			return nil
		}),
	}
	if rdb != nil {
		checks = append(checks, ops.FuncCheck("redis", false, func(ctx context.Context) error { return rdb.Ping(ctx).Err() }))
	}
	adm := ops.New(ops.Options{Service: "analytics-service", Checks: checks})
	g := func(name, help string, fn func() float64) { adm.GaugeFunc(name, help, nil, fn) }
	g("mm_ch_buffer_rows", "Rows waiting to be inserted into ClickHouse.", func() float64 { return float64(sink.Stats().Buffered) })
	g("mm_ch_insert_lag_seconds", "Age of the oldest row still waiting for insertion.", func() float64 { return sink.Stats().OldestAgeSeconds })
	g("mm_ch_rows_inserted_total", "Rows inserted into ClickHouse.", func() float64 { return float64(sink.Stats().Inserted) })
	g("mm_ch_insert_failures_total", "Failed insert batches.", func() float64 { return float64(sink.Stats().FailedBatches) })
	g("mm_ch_rows_dropped_total", "Rows dropped because the buffer was full.", func() float64 { return float64(sink.Stats().Dropped) })
	g("mm_events_accepted_total", "Tracking events accepted.", func() float64 { a, _ := svc.Counters(); return float64(a) })
	g("mm_events_rejected_total", "Tracking events rejected (validation).", func() float64 { _, r := svc.Counters(); return float64(r) })

	s := grpc.NewServer(grpc.ChainUnaryInterceptor(adm.UnaryInterceptor()), grpc.ChainStreamInterceptor(adm.StreamInterceptor()))
	pb.RegisterAnalyticsServiceServer(s, &server.Server{Service: svc})
	reflection.Register(s)

	log.Printf("Analytics Server listening at %v", lis.Addr())
	if err := adm.ServeGRPC(s, lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
