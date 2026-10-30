package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vitess.io/vitess/go/vt/vitessdriver"
)

// VitessConfig configures the VTGate gRPC endpoint; Envoy owns transport mTLS.
type VitessConfig struct {
	Address, Target                     string
	MaxOpenConns, MaxIdleConns          int
	Timeout, ConnLifetime, ConnIdleTime time.Duration
}

var primaryTarget = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*@primary$`)

func (c VitessConfig) validated() (VitessConfig, error) {
	if c.Address == "" {
		c.Address = "127.0.0.1:15991"
	}
	if c.Target == "" {
		c.Target = "commerce@primary"
	}
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 10
	}
	if c.Timeout == 0 {
		c.Timeout = 5 * time.Second
	}
	if c.ConnLifetime == 0 {
		c.ConnLifetime = 5 * time.Minute
	}
	if c.ConnIdleTime == 0 {
		c.ConnIdleTime = time.Minute
	}
	host, port, err := net.SplitHostPort(c.Address)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || host == "" || strings.ContainsAny(host, "/@ \t\r\n") || number < 1 || number > 65535 || !primaryTarget.MatchString(c.Target) || c.MaxOpenConns < 1 || c.MaxOpenConns > 64 || c.MaxIdleConns < 0 || c.MaxIdleConns > c.MaxOpenConns || c.Timeout < 100*time.Millisecond || c.Timeout > 30*time.Second || c.ConnLifetime < time.Second || c.ConnLifetime > time.Hour || c.ConnIdleTime < time.Second || c.ConnIdleTime > c.ConnLifetime {
		return c, errors.New("database configuration: invalid VTGate address, target or pool bounds")
	}
	return c, nil
}

// VitessConfigFromEnv reads only VTGate-specific configuration, never PostgreSQL DSNs.
func VitessConfigFromEnv() (VitessConfig, error) {
	c := VitessConfig{Address: os.Getenv("VTGATE_ADDRESS"), Target: os.Getenv("VTGATE_TARGET"), MaxOpenConns: 10, MaxIdleConns: 5}
	for name, target := range map[string]*int{"VTGATE_MAX_OPEN_CONNS": &c.MaxOpenConns, "VTGATE_MAX_IDLE_CONNS": &c.MaxIdleConns} {
		if value := os.Getenv(name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil || (name == "VTGATE_MAX_OPEN_CONNS" && number == 0) {
				return c, errors.New("database configuration: invalid numeric pool setting")
			}
			*target = number
		}
	}
	for name, target := range map[string]*time.Duration{"VTGATE_TIMEOUT": &c.Timeout, "VTGATE_CONN_LIFETIME": &c.ConnLifetime, "VTGATE_CONN_IDLE_TIME": &c.ConnIdleTime} {
		if value := os.Getenv(name); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return c, errors.New("database configuration: invalid duration setting")
			}
			*target = duration
		}
	}
	return c.validated()
}

// OpenVitess creates a bounded pool and verifies connectivity with the caller's deadline.
// The address points to a local Envoy listener in the SPIFFE deployment.
func OpenVitess(ctx context.Context, cfg VitessConfig) (*DB, error) {
	cfg, err := cfg.validated()
	if err != nil {
		return nil, err
	}
	pool, err := vitessdriver.Open(cfg.Address, cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("database: open VTGate driver: %w", err)
	}
	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnLifetime)
	pool.SetConnMaxIdleTime(cfg.ConnIdleTime)
	startup, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	if err := pool.PingContext(startup); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: VTGate startup check failed: %w", err)
	}
	return &DB{Pool: pool}, nil
}
