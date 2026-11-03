package auth

import "github.com/prometheus/client_golang/prometheus"

// VerifierMetrics preserves the auth cache gauges without exposing claims or tokens.
func VerifierMetrics(verifier *Verifier) []prometheus.Collector {
	return []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "auth_jwks_cache_age_seconds", Help: "Age of cached JWKS."}, func() float64 { age, _, _ := verifier.CacheStatus(); return age.Seconds() }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "auth_jwks_keys", Help: "Cached public JWKS keys."}, func() float64 { _, count, _ := verifier.CacheStatus(); return float64(count) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "auth_jwks_ready", Help: "Whether JWT verification is ready."}, func() float64 {
			_, _, ready := verifier.CacheStatus()
			if ready {
				return 1
			}
			return 0
		}),
	}
}
