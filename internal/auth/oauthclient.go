package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthClientConfig fixes the token endpoint and resource origin to prevent credential forwarding.
type OAuthClientConfig struct {
	TokenURL, ResourceURL, Resource, ClientID, ClientSecret string
	ResourceParameter, AuthMethod                           string
	Scopes                                                  []string
	AllowHTTPResource                                       bool
	RefreshBefore                                           time.Duration
	HTTPClient                                              *http.Client
	Now                                                     func() time.Time
}

// ClientSecretBasic and ClientSecretPost name the supported token endpoint authentication methods.
const (
	ClientSecretBasic = "client_secret_basic"
	ClientSecretPost  = "client_secret_post"
)

type tokenFlight struct {
	done  chan struct{}
	token string
	err   error
}

// OAuthClient is a shared, cancellation-aware client-credentials cache and outbound transport.
type OAuthClient struct {
	cfg               OAuthClientConfig
	tokenHTTP         *http.Client
	resourceTransport http.RoundTripper
	mu                sync.Mutex
	token             string
	renewAt           time.Time
	flight            *tokenFlight
}

// NewOAuthClient supports standard client credentials with optional resource/audience parameters.
func NewOAuthClient(cfg OAuthClientConfig) (*OAuthClient, error) {
	if cfg.AuthMethod == "" {
		cfg.AuthMethod = ClientSecretBasic
	}
	if cfg.ResourceParameter == "" {
		cfg.ResourceParameter = "resource"
	}
	if ValidateEndpoint(cfg.TokenURL, false) != nil || ValidateEndpoint(cfg.ResourceURL, cfg.AllowHTTPResource) != nil || !validValue(cfg.ClientID) || (cfg.Resource != "" && !validValue(cfg.Resource)) || len(cfg.ClientSecret) == 0 || len(cfg.ClientSecret) > 1024 || !validValues(cfg.Scopes) || (cfg.AuthMethod != ClientSecretBasic && cfg.AuthMethod != ClientSecretPost) || (cfg.ResourceParameter != "resource" && cfg.ResourceParameter != "audience") {
		return nil, errors.New("auth OAuth client: invalid configuration")
	}
	if cfg.RefreshBefore == 0 {
		cfg.RefreshBefore = 30 * time.Second
	}
	if cfg.RefreshBefore < 0 || cfg.RefreshBefore > time.Minute {
		return nil, errors.New("auth OAuth client: invalid renewal margin")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.Scopes = append([]string(nil), cfg.Scopes...)
	client := boundedHTTPClient(cfg.HTTPClient)
	return &OAuthClient{cfg: cfg, tokenHTTP: client, resourceTransport: client.Transport}, nil
}

// AccessToken coalesces concurrent renewals; waiting callers can cancel independently.
func (c *OAuthClient) AccessToken(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.mu.Lock()
	if c.token != "" && c.cfg.Now().Before(c.renewAt) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	if flight := c.flight; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.token, flight.err
		}
	}
	flight := &tokenFlight{done: make(chan struct{})}
	c.flight = flight
	c.mu.Unlock()
	token, renewAt, err := c.fetchToken(ctx)
	c.mu.Lock()
	if err == nil {
		c.token = token
		c.renewAt = renewAt
	}
	flight.token, flight.err = token, err
	c.flight = nil
	close(flight.done)
	c.mu.Unlock()
	return token, err
}

func (c *OAuthClient) fetchToken(ctx context.Context) (string, time.Time, error) {
	form := url.Values{"grant_type": {"client_credentials"}}
	if c.cfg.Resource != "" {
		form.Set(c.cfg.ResourceParameter, c.cfg.Resource)
	}
	if len(c.cfg.Scopes) > 0 {
		form.Set("scope", strings.Join(c.cfg.Scopes, " "))
	}
	if c.cfg.AuthMethod == ClientSecretPost {
		form.Set("client_id", c.cfg.ClientID)
		form.Set("client_secret", c.cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, errors.New("auth OAuth client: request creation failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.cfg.AuthMethod == ClientSecretBasic {
		req.SetBasicAuth(url.QueryEscape(c.cfg.ClientID), url.QueryEscape(c.cfg.ClientSecret))
	}
	started := c.cfg.Now()
	response, err := c.tokenHTTP.Do(req)
	if err != nil {
		return "", time.Time{}, errors.New("auth OAuth client: token endpoint unavailable")
	}
	defer response.Body.Close() // Never print provider error bodies, which may contain credentials or tokens.
	if response.StatusCode != http.StatusOK {
		return "", time.Time{}, errors.New("auth OAuth client: token request rejected")
	}
	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Scope       string `json:"scope"`
	}
	if decodeProviderJSON(response.Body, MaxTokenBytes+2048, &result) != nil || len(result.AccessToken) == 0 || len(result.AccessToken) > MaxTokenBytes || !strings.EqualFold(result.TokenType, "Bearer") || result.ExpiresIn < 1 || result.ExpiresIn > 86400 || strings.ContainsAny(result.AccessToken, " \r\n\t") {
		return "", time.Time{}, errors.New("auth OAuth client: invalid token response")
	}
	ttl := time.Duration(result.ExpiresIn) * time.Second
	renewAt := started.Add(ttl - min(c.cfg.RefreshBefore, ttl/2))
	if !c.cfg.Now().Before(renewAt) {
		return "", time.Time{}, errors.New("auth OAuth client: token arrived too late")
	}
	return result.AccessToken, renewAt, nil
}

// RoundTrip clones the request and adds a token only for the configured resource origin.
func (c *OAuthClient) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("auth OAuth client: missing request URL")
	}
	handedOff := false
	defer func() {
		if !handedOff && req.Body != nil {
			req.Body.Close()
		}
	}() // Transport owns request-body closure even on authentication failures.
	base, err := url.Parse(c.cfg.ResourceURL)
	if err != nil || req.URL.Scheme != base.Scheme || req.URL.Host != base.Host || req.URL.User != nil || req.URL.Opaque != "" || (req.Host != "" && req.Host != base.Host) {
		return nil, errors.New("auth OAuth client: untrusted resource origin")
	}
	token, err := c.AccessToken(req.Context())
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+token)
	handedOff = true
	return c.resourceTransport.RoundTrip(clone)
}

// HTTPClient returns a client sharing this token cache, without implicit API retries or redirects.
func (c *OAuthClient) HTTPClient() *http.Client {
	return &http.Client{Transport: c, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// Close releases pooled idle connections; there are no cache-owned goroutines.
func (c *OAuthClient) Close() { c.tokenHTTP.CloseIdleConnections() }
