package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"analytics-service/internal/config/messagequeue/kafkaimpl"
)

// Kafka bundles the Kafka helpers shared by all services.
type Kafka struct {
	Manager  *kafkaimpl.KafkaManager
	Consumer *kafkaimpl.KafkaConsumer
	Client   *kafkaimpl.KafkaClient
}

// Config is the environment configuration of analytics-service.
type Config struct {
	ClickHouseURL      string // http://clickhouse:8123
	ClickHouseDB       string
	ClickHouseUser     string
	ClickHousePassword string
	RedisAddr          string
	CacheTTL           time.Duration
	FlushInterval      time.Duration
	FlushRows          int
	BufferMax          int
	Brokers            []string
	// Retention of raw events in months (the TTL of events_raw)
	EventsRetentionMonths int
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// FromEnv reads and validates the configuration.
func FromEnv() (*Config, error) {
	c := &Config{
		ClickHouseURL:         envStr("CLICKHOUSE_URL", "http://localhost:8123"),
		ClickHouseDB:          envStr("CLICKHOUSE_DB", "marketplace"),
		ClickHouseUser:        envStr("CLICKHOUSE_USER", "default"),
		ClickHousePassword:    os.Getenv("CLICKHOUSE_PASSWORD"),
		RedisAddr:             os.Getenv("REDIS_ADDR"),
		CacheTTL:              time.Duration(envInt("ANALYTICS_CACHE_TTL_SEC", 30)) * time.Second,
		FlushInterval:         time.Duration(envInt("ANALYTICS_FLUSH_INTERVAL_MS", 2000)) * time.Millisecond,
		FlushRows:             envInt("ANALYTICS_FLUSH_ROWS", 5000),
		BufferMax:             envInt("ANALYTICS_BUFFER_MAX_ROWS", 100000),
		EventsRetentionMonths: envInt("EVENTS_RETENTION_MONTHS", 13),
	}
	for _, b := range strings.Split(os.Getenv("KAFKA_BROKERS_ADDR"), ",") {
		if b = strings.TrimSpace(b); b != "" {
			c.Brokers = append(c.Brokers, b)
		}
	}
	if len(c.Brokers) == 0 {
		return nil, errors.New("KAFKA_BROKERS_ADDR env variable not set")
	}
	return c, nil
}

// NewKafka builds the Kafka helpers.
func (c *Config) NewKafka() *Kafka {
	m := kafkaimpl.NewKafkaManager(c.Brokers)
	return &Kafka{
		Manager:  m,
		Consumer: kafkaimpl.NewKafkaConsumer(m, time.Duration(envInt("KAFKA_CONSUMER_BACKOFF", 100))*time.Millisecond),
		Client:   kafkaimpl.NewKafkaClient(c.Brokers),
	}
}
