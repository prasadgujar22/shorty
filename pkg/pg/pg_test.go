package pg

import (
	"testing"
	"time"

	"github.com/prasadgujar22/shorty/pkg/config"
)

func TestPoolConfigFromEnv(t *testing.T) {
	env := config.NewFromMap(map[string]string{
		"DATABASE_URL":         "postgres://u:p@db:5432/shorty",
		"DB_MAX_CONNS":         "25",
		"DB_STATEMENT_TIMEOUT": "750ms",
	})
	c := ConfigFromEnv(env, "links")
	if err := env.Err(); err != nil {
		t.Fatal(err)
	}
	pc, err := c.PoolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != 25 || pc.MinConns != 2 {
		t.Errorf("conns = %d/%d", pc.MaxConns, pc.MinConns)
	}
	rp := pc.ConnConfig.RuntimeParams
	if rp["search_path"] != "links" || rp["statement_timeout"] != "750" {
		t.Errorf("runtime params = %v", rp)
	}
	if pc.MaxConnLifetime != 30*time.Minute {
		t.Errorf("lifetime = %v", pc.MaxConnLifetime)
	}
}

func TestLockKeyStablePerSchema(t *testing.T) {
	if lockKey("links") == lockKey("analytics") {
		t.Fatal("lock keys collide")
	}
	a, b := lockKey("links"), lockKey("links")
	if a != b || a < 0 {
		t.Fatal("lock key must be stable and positive")
	}
}

func TestZeroValuesKeepDefaults(t *testing.T) {
	pc, err := Config{DSN: "postgres://u@db/x"}.PoolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if pc.HealthCheckPeriod <= 0 || pc.MaxConns <= 0 || pc.MaxConnLifetime <= 0 {
		t.Fatalf("defaults clobbered: %+v", pc)
	}
	if _, ok := pc.ConnConfig.RuntimeParams["search_path"]; ok {
		t.Fatal("search_path set without a schema")
	}
}
