package middleware

import (
	"strings"
	"testing"
	"time"
)

func TestRequestPolicyRejectsUnboundedSettings(t *testing.T) {
	for _, policy := range []RequestPolicy{
		{Timeout: 0, MaxInFlight: 1}, {Timeout: 6 * time.Second, MaxInFlight: 1},
		{Timeout: time.Second, MaxInFlight: 0}, {Timeout: time.Second, MaxInFlight: 1025},
	} {
		if policy.Validate() == nil {
			t.Errorf("unbounded policy accepted: %v", policy)
		}
	}
	t.Setenv("REQUEST_TIMEOUT", "sensitive invalid value")
	_, err := RequestPolicyFromEnv()
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("invalid configuration was accepted or leaked")
	}
	t.Setenv("REQUEST_TIMEOUT", "1s")
	t.Setenv("REQUEST_MAX_IN_FLIGHT", "2")
	policy, err := RequestPolicyFromEnv()
	if err != nil || policy.Timeout != time.Second || policy.MaxInFlight != 2 {
		t.Fatalf("typed policy = %v, %v", policy, err)
	}
}
