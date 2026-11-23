package dbpool

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

type stubDriver struct{}

func (stubDriver) Open(string) (driver.Conn, error) { return nil, errors.New("stub: never connects") }

func init() { sql.Register("dbpool-stub", stubDriver{}) }

func TestLimitsDefaultsAndOverrides(t *testing.T) {
	cases := []struct {
		open, idle   string
		wantO, wantI int
	}{
		{"", "", 30, 10},
		{"abc", "-1", 30, 10},
		{"0", "0", 30, 10},
		{"12", "", 12, 10},
		{"12", "4", 12, 4},
		{"6", "", 6, 6},       // idle never above open
		{"9999", "", 500, 10}, // capped
	}
	for _, c := range cases {
		t.Setenv("DB_MAX_OPEN_CONNS", c.open)
		t.Setenv("DB_MAX_IDLE_CONNS", c.idle)
		o, i := Limits(30, 10)
		if o != c.wantO || i != c.wantI {
			t.Errorf("open=%q idle=%q: got %d/%d, want %d/%d", c.open, c.idle, o, i, c.wantO, c.wantI)
		}
	}
}

func TestApplySetsMaxOpen(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	db, err := sql.Open("dbpool-stub", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	Apply(db, 7, 3)
	if got := db.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("MaxOpenConnections = %d, want 7", got)
	}
}
