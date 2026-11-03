package middleware

import (
	"errors"
	"os"
	"strconv"
	"time"
)

// RequestPolicy bounds business work per process; operational endpoints bypass capacity.
type RequestPolicy struct {
	Timeout     time.Duration
	MaxInFlight int
}

// DefaultRequestPolicy stays within the existing HTTP write and upstream timeout budgets.
func DefaultRequestPolicy() RequestPolicy {
	return RequestPolicy{Timeout: 5 * time.Second, MaxInFlight: 64}
}

// RequestPolicyFromEnv reads REQUEST_TIMEOUT and REQUEST_MAX_IN_FLIGHT without logging values.
func RequestPolicyFromEnv() (RequestPolicy, error) {
	policy := DefaultRequestPolicy()
	var err error
	if raw := os.Getenv("REQUEST_TIMEOUT"); raw != "" {
		policy.Timeout, err = time.ParseDuration(raw)
		if err != nil {
			return RequestPolicy{}, errors.New("request policy: invalid timeout")
		}
	}
	if raw := os.Getenv("REQUEST_MAX_IN_FLIGHT"); raw != "" {
		policy.MaxInFlight, err = strconv.Atoi(raw)
		if err != nil {
			return RequestPolicy{}, errors.New("request policy: invalid capacity")
		}
	}
	return policy, policy.Validate()
}

// Validate prevents unbounded execution or capacity; caller deadlines may still be shorter.
func (p RequestPolicy) Validate() error {
	if p.Timeout < 100*time.Millisecond || p.Timeout > 5*time.Second || p.MaxInFlight < 1 || p.MaxInFlight > 1024 {
		return errors.New("request policy: timeout must be 100ms–5s and capacity 1–1024")
	}
	return nil
}
