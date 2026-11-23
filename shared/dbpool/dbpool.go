// Package dbpool caps a service's Postgres connection pool.
//
// database/sql opens a new connection whenever every existing one is busy and has NO upper bound by default, so a load
// peak turns into hundreds of connections and Postgres refuses the excess ("sorry, too many clients already", R15).
// With a cap the surplus callers wait inside the service (bounded by their own context deadline) instead.
//
// Budget: the sum over all replicas of MaxOpen must stay clearly below Postgres max_connections (leave room for the
// exporter, ops, analytics, a rolling update that starts the new replica before stopping the old one, and admin sessions).
package dbpool

import (
	"database/sql"
	"os"
	"strconv"
	"time"
)

const (
	maxEnvOpen   = 500
	connLifetime = 30 * time.Minute
	connIdleTime = 5 * time.Minute
)

// Limits returns the pool size: DB_MAX_OPEN_CONNS / DB_MAX_IDLE_CONNS when set to a positive number, else the given defaults.
// Idle is never above open.
func Limits(defOpen, defIdle int) (open, idle int) {
	open, idle = envInt("DB_MAX_OPEN_CONNS", defOpen), envInt("DB_MAX_IDLE_CONNS", defIdle)
	if open > maxEnvOpen {
		open = maxEnvOpen
	}
	if idle > open {
		idle = open
	}
	return open, idle
}

// Apply sets the pool limits on db (see Limits) and returns them.
func Apply(db *sql.DB, defOpen, defIdle int) (open, idle int) {
	open, idle = Limits(defOpen, defIdle)
	db.SetMaxOpenConns(open)
	db.SetMaxIdleConns(idle)
	db.SetConnMaxLifetime(connLifetime)
	db.SetConnMaxIdleTime(connIdleTime)
	return open, idle
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}
