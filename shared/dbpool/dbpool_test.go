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
		{"", "", 30, 30}, // idle follows open by default
		{"abc", "-1", 30, 30},
		{"0", "0", 30, 30},
		{"12", "", 12, 12},
		{"12", "4", 12, 4}, // explicit smaller idle
		{"6", "20", 6, 6},  // idle never above open
		{"", "10", 30, 10},
		{"9999", "", 500, 500}, // capped
	}
	for _, c := range cases {
		t.Setenv("DB_MAX_OPEN_CONNS", c.open)
		t.Setenv("DB_MAX_IDLE_CONNS", c.idle)
		o, i := Limits(30)
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
	Apply(db, 7)
	if got := db.Stats().MaxOpenConnections; got != 7 {
		t.Fatalf("MaxOpenConnections = %d, want 7", got)
	}
	if o, i := Limits(7); o != 7 || i != 7 {
		t.Fatalf("Limits(7) = %d/%d, want 7/7", o, i)
	}
}
