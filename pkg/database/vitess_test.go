package database

import (
	"context"
	"testing"
	"time"
)

func TestVitessConfigurationValidation(t *testing.T) {
	for _, cfg := range []VitessConfig{
		{Address: "https://user:password@vtgate:15991"},
		{Address: "vtgate:0"}, {Target: "commerce@replica"},
		{MaxOpenConns: -1}, {MaxOpenConns: 1000},
		{MaxOpenConns: 1, MaxIdleConns: 2}, {Timeout: time.Hour},
	} {
		if db, err := OpenVitess(context.Background(), cfg); err == nil {
			db.Close(context.Background())
			t.Fatal("invalid configuration accepted")
		}
	}
}

func TestVitessEnvironmentDefaults(t *testing.T) {
	for _, name := range []string{"VTGATE_ADDRESS", "VTGATE_TARGET", "VTGATE_MAX_OPEN_CONNS", "VTGATE_MAX_IDLE_CONNS", "VTGATE_TIMEOUT", "VTGATE_CONN_LIFETIME", "VTGATE_CONN_IDLE_TIME"} {
		t.Setenv(name, "")
	}
	cfg, err := VitessConfigFromEnv()
	if err != nil || cfg.Address != "127.0.0.1:15991" || cfg.Target != "commerce@primary" || cfg.MaxOpenConns != 10 || cfg.MaxIdleConns != 5 {
		t.Fatal("unexpected VTGate defaults")
	}
	t.Setenv("VTGATE_MAX_OPEN_CONNS", "invalid")
	if _, err := VitessConfigFromEnv(); err == nil {
		t.Fatal("invalid pool setting accepted")
	}
	t.Setenv("VTGATE_MAX_OPEN_CONNS", "0")
	if _, err := VitessConfigFromEnv(); err == nil {
		t.Fatal("explicitly unbounded pool setting accepted")
	}
}
