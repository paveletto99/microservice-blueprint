package auth

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/paveletto99/microservice-blueprint/pkg/logging"
)

var (
	ErrInvalidToken    = errors.New("auth verifier: invalid access token")
	ErrJWKSUnavailable = errors.New("auth verifier: trusted signing keys unavailable")
	ErrRefreshLimited  = errors.New("auth verifier: JWKS refresh rate limited")
)

// VerifierConfig fixes trust anchors; Now is injectable for deterministic clock tests.
type VerifierConfig struct {
	Issuer, Audience, JWKSURL                                          string
	TokenType                                                          string
	Claims                                                             ClaimsMapping
	Leeway, MaxLifetime, RefreshInterval, MinimumRefresh, MaxStaleness time.Duration
	HTTPClient                                                         *http.Client
	Now                                                                func() time.Time
}

type refreshFlight struct {
	done chan struct{}
	err  error
}

// Verifier owns one bounded JWKS refresh worker and a last-known-good public-key cache.
type Verifier struct {
	cfg                    VerifierConfig
	client                 *http.Client
	mu                     sync.RWMutex
	keys                   map[string]*rsa.PublicKey
	fetchedAt, nextAttempt time.Time
	refreshDelay           time.Duration
	flight                 *refreshFlight
	closed                 bool
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
}

// NewVerifier warms the JWKS cache before readiness; ctx owns the worker lifetime.
func NewVerifier(ctx context.Context, cfg VerifierConfig) (*Verifier, error) {
	if ValidateEndpoint(cfg.Issuer, false) != nil || ValidateEndpoint(cfg.JWKSURL, false) != nil || !validValue(cfg.Audience) {
		return nil, errors.New("auth verifier: invalid issuer, audience or JWKS endpoint")
	}
	if cfg.TokenType == "" {
		cfg.TokenType = "at+jwt"
	}
	if !validValue(cfg.TokenType) {
		return nil, errors.New("auth verifier: invalid access-token type")
	}
	var err error
	cfg.Claims, err = cfg.Claims.configured()
	if err != nil {
		return nil, err
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MaxLifetime == 0 {
		cfg.MaxLifetime = 10 * time.Minute
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 5 * time.Second
	}
	if cfg.MinimumRefresh == 0 {
		cfg.MinimumRefresh = time.Second
	}
	if cfg.MaxStaleness == 0 {
		cfg.MaxStaleness = 10 * time.Minute
	}
	if cfg.Leeway < 0 || cfg.Leeway > 30*time.Second || cfg.MaxLifetime < time.Second || cfg.MaxLifetime > 24*time.Hour || cfg.MinimumRefresh < time.Second || cfg.RefreshInterval < cfg.MinimumRefresh || cfg.RefreshInterval > time.Minute || cfg.MaxStaleness < cfg.RefreshInterval || cfg.MaxStaleness > time.Hour {
		return nil, errors.New("auth verifier: invalid clock or cache bounds")
	}
	v := &Verifier{cfg: cfg, client: boundedHTTPClient(cfg.HTTPClient), refreshDelay: cfg.RefreshInterval}
	if err := v.Refresh(ctx); err != nil {
		return nil, err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	v.cancel = cancel
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		for {
			v.mu.RLock()
			delay := v.refreshDelay
			v.mu.RUnlock()
			timer := time.NewTimer(delay)
			select {
			case <-workerCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if err := v.Refresh(workerCtx); err != nil && !errors.Is(err, ErrRefreshLimited) && workerCtx.Err() == nil {
					logging.FromContext(workerCtx).Warn("JWKS refresh failed; retaining last known trusted keys")
				}
			}
		}
	}()
	return v, nil
}

// Close stops and joins the sole refresh worker; repeated Close calls are safe.
func (v *Verifier) Close() {
	v.mu.Lock()
	v.closed = true
	v.mu.Unlock()
	v.cancel()
	v.wg.Wait()
	v.client.CloseIdleConnections()
}

// CacheStatus exposes only public cache health, never tokens or credentials.
func (v *Verifier) CacheStatus() (age time.Duration, count int, ready bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.fetchedAt.IsZero() {
		return 0, 0, false
	}
	age = v.cfg.Now().Sub(v.fetchedAt)
	return age, len(v.keys), !v.closed && age >= 0 && age <= v.cfg.MaxStaleness && len(v.keys) > 0
}

// Ready requires initialized trusted keys within the configured outage staleness limit.
func (v *Verifier) Ready() bool { _, _, ready := v.CacheStatus(); return ready }

// Refresh coalesces concurrent downloads and rate-limits even failed and unknown-kid refreshes.
func (v *Verifier) Refresh(ctx context.Context) error {
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return ErrJWKSUnavailable
	}
	if flight := v.flight; flight != nil {
		v.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-flight.done:
			return flight.err
		}
	}
	if v.cfg.Now().Before(v.nextAttempt) {
		v.mu.Unlock()
		return ErrRefreshLimited
	}
	flight := &refreshFlight{done: make(chan struct{})}
	v.flight = flight
	v.nextAttempt = v.cfg.Now().Add(v.cfg.MinimumRefresh)
	v.mu.Unlock()
	keys, delay, err := v.download(ctx)
	v.mu.Lock()
	if err == nil && !v.closed {
		v.keys = keys
		v.fetchedAt = v.cfg.Now()
		v.refreshDelay = delay
	}
	flight.err = err
	v.flight = nil
	close(flight.done)
	v.mu.Unlock()
	return err
}

func (v *Verifier) download(ctx context.Context) (map[string]*rsa.PublicKey, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return nil, 0, ErrJWKSUnavailable
	}
	response, err := v.client.Do(req)
	if err != nil {
		return nil, 0, ErrJWKSUnavailable
	}
	defer response.Body.Close() // Bounded read and connection reuse; close errors cannot repair an HTTP response.
	if response.StatusCode != http.StatusOK {
		return nil, 0, ErrJWKSUnavailable
	}
	var set publicKeySet
	if decodeProviderJSON(response.Body, 64*1024, &set) != nil {
		return nil, 0, ErrJWKSUnavailable
	}
	keys, err := decodePublicKeys(set)
	if err != nil {
		return nil, 0, ErrJWKSUnavailable
	}
	delay := v.cfg.RefreshInterval
	for _, part := range strings.Split(response.Header.Get("Cache-Control"), ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name == "max-age" {
			seconds, err := strconv.ParseInt(value, 10, 32)
			if err == nil && seconds >= 0 && seconds <= 3600 {
				delay = min(delay, time.Duration(seconds)*time.Second)
			}
		}
		if strings.TrimSpace(part) == "no-cache" || strings.TrimSpace(part) == "no-store" {
			delay = v.cfg.MinimumRefresh
		}
	}
	return keys, max(delay, v.cfg.MinimumRefresh), nil
}

func (v *Verifier) signingKey(ctx context.Context, token *jwt.Token) (any, error) {
	if token.Method != jwt.SigningMethodRS256 || token.Header["typ"] != v.cfg.TokenType {
		return nil, ErrInvalidToken
	}
	for _, field := range []string{"jku", "jwk", "x5u", "crit", "b64"} {
		if _, exists := token.Header[field]; exists {
			return nil, ErrInvalidToken
		}
	}
	kid, ok := token.Header["kid"].(string)
	if !ok || !validValue(kid) || len(kid) > 64 {
		return nil, ErrInvalidToken
	}
	v.mu.RLock()
	key := v.keys[kid]
	v.mu.RUnlock()
	if !v.Ready() {
		return nil, ErrJWKSUnavailable
	}
	if key == nil {
		if err := v.Refresh(ctx); err != nil {
			return nil, ErrInvalidToken
		}
		v.mu.RLock()
		key = v.keys[kid]
		v.mu.RUnlock()
	}
	if key == nil || !v.Ready() {
		return nil, ErrInvalidToken
	}
	return key, nil
}

// Verify checks RS256 signatures and the configured token profile before normalizing claims.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Claims, error) {
	if ctx.Err() != nil || len(raw) == 0 || len(raw) > MaxTokenBytes {
		return nil, ErrInvalidToken
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}
	header, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	var shape struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
		KeyID     string `json:"kid"`
	}
	if decodeProviderJSON(bytes.NewReader(header), 2048, &shape) != nil || shape.Algorithm != "RS256" || shape.Type != v.cfg.TokenType {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}
	// Structural checks do not trust these claims; signature verification below precedes acceptance.
	if decodeProviderJSON(bytes.NewReader(payload), MaxTokenBytes, new(jwt.MapClaims)) != nil {
		return nil, ErrInvalidToken
	}
	rawClaims := jwt.MapClaims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithStrictDecoding(), jwt.WithIssuer(v.cfg.Issuer), jwt.WithAudience(v.cfg.Audience), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(v.cfg.Leeway), jwt.WithTimeFunc(v.cfg.Now))
	token, err := parser.ParseWithClaims(raw, rawClaims, func(token *jwt.Token) (any, error) { return v.signingKey(ctx, token) })
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	claims, err := v.cfg.Claims.normalize(rawClaims)
	if err != nil {
		return nil, err
	}
	lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > v.cfg.MaxLifetime || claims.NotBefore != nil && claims.NotBefore.After(claims.ExpiresAt.Time) {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
