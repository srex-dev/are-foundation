package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const phase1StaticDisabledBody = `{"error":{"code":"UPSTREAM_UNAVAILABLE","message":"foundation route is not forwarded via ARE_GW_S0S1_HTTP_PROXY_BASE; StaticUpstream disabled"}}`

func envLiveNoStaticPhase1() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ARE_GW_LIVE_NO_STATIC_PHASE1")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func HTTPUpstreamTimeoutOrDefault() time.Duration {
	v := strings.TrimSpace(os.Getenv("ARE_GW_S0S1_HTTP_TIMEOUT_SECONDS"))
	if v == "" {
		return 60 * time.Second
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec <= 0 {
		return 60 * time.Second
	}
	return time.Duration(sec) * time.Second
}

func NewHTTPClientForRESTProxy(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = HTTPUpstreamTimeoutOrDefault()
	}
	return &http.Client{Timeout: timeout, Transport: newRESTProxyTransport(nil)}
}

func NewHTTPClientForGatewayMTLS(certFile, keyFile, caFile, tlsServerName string, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = HTTPUpstreamTimeoutOrDefault()
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("s0s1 http proxy client tls keypair: %w", err)
	}
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("s0s1 http proxy client read ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("s0s1 http proxy client append ca failed")
	}
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
	}
	if sn := strings.TrimSpace(tlsServerName); sn != "" {
		tlsCfg.ServerName = sn
	}
	return &http.Client{Timeout: timeout, Transport: newRESTProxyTransport(tlsCfg)}, nil
}

func newRESTProxyTransport(tlsCfg *tls.Config) *http.Transport {
	connectTimeout := durationFromMillisEnv("ARE_GW_HTTP_CONNECT_TIMEOUT_MS", 250)
	keepAlive := durationFromSecondsEnv("ARE_GW_HTTP_KEEPALIVE_SECONDS", 30)
	idleTimeout := durationFromSecondsEnv("ARE_GW_HTTP_IDLE_CONN_TIMEOUT_SECONDS", 90)
	tlsHandshakeTimeout := durationFromSecondsEnv("ARE_GW_HTTP_TLS_HANDSHAKE_TIMEOUT_SECONDS", 10)
	expectContinueTimeout := durationFromSecondsEnv("ARE_GW_HTTP_EXPECT_CONTINUE_TIMEOUT_SECONDS", 1)
	maxIdleConns := intFromEnv("ARE_GW_HTTP_MAX_IDLE_CONNS", 512)
	maxIdleConnsPerHost := intFromEnv("ARE_GW_HTTP_MAX_IDLE_CONNS_PER_HOST", 128)
	maxConnsPerHost := intFromEnv("ARE_GW_HTTP_MAX_CONNS_PER_HOST", 0)
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   connectTimeout,
			KeepAlive: keepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		MaxConnsPerHost:       maxConnsPerHost,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
		TLSClientConfig:       tlsCfg,
	}
}

func intFromEnv(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func durationFromMillisEnv(name string, fallbackMillis int) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return time.Duration(fallbackMillis) * time.Millisecond
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return time.Duration(fallbackMillis) * time.Millisecond
	}
	return time.Duration(parsed) * time.Millisecond
}

func durationFromSecondsEnv(name string, fallbackSeconds int) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return time.Duration(fallbackSeconds) * time.Second
	}
	parsed, err := strconv.Atoi(v)
	if err != nil || parsed <= 0 {
		return time.Duration(fallbackSeconds) * time.Second
	}
	return time.Duration(parsed) * time.Second
}

type RESTProxyUpstream struct {
	base   *url.URL
	client *http.Client
}

func NewRESTProxyUpstream(baseURL string, client *http.Client) (*RESTProxyUpstream, error) {
	if client == nil {
		return nil, errors.New("http client is required")
	}
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid ARE_GW_S0S1_HTTP_PROXY_BASE: %q", baseURL)
	}
	return &RESTProxyUpstream{base: u, client: client}, nil
}

func (p *RESTProxyUpstream) Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	if req.Path == "" || req.Path[0] != '/' {
		return UpstreamResponse{}, fmt.Errorf("upstream path must be absolute: %q", req.Path)
	}
	bu := *p.base
	prefix := strings.TrimSuffix(bu.Path, "/")
	bu.Path = prefix + req.Path
	bu.RawQuery = req.RawQuery
	hr, err := http.NewRequestWithContext(ctx, req.Method, bu.String(), bytes.NewReader(req.Body))
	if err != nil {
		return UpstreamResponse{}, err
	}
	if req.Authorization != "" {
		hr.Header.Set("Authorization", req.Authorization)
	}
	hr.Header.Set("X-Request-ID", req.RequestID)
	hr.Header.Set("X-ARE-Agent-ID", req.AgentID)
	if req.IdempotencyKey != "" {
		hr.Header.Set("Idempotency-Key", req.IdempotencyKey)
	}
	if req.Traceparent != "" {
		hr.Header.Set("traceparent", req.Traceparent)
	}
	if ct := strings.TrimSpace(req.ContentType); ct != "" {
		hr.Header.Set("Content-Type", ct)
	} else if len(req.Body) > 0 {
		hr.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(hr)
	if err != nil {
		return UpstreamResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UpstreamResponse{}, err
	}
	return UpstreamResponse{StatusCode: resp.StatusCode, Body: body}, nil
}

type s0s1SelectingUpstream struct {
	s0s1Proxy            *RESTProxyUpstream
	fallback             UpstreamClient
	denyUnknownPhase1    bool
	rejectPhase1Static   bool
	upstreamUnavailable  UpstreamClient
	upstreamPhase1Static UpstreamClient
}

func NewS0S1SelectingUpstream(proxy *RESTProxyUpstream, fallback UpstreamClient, denyUnknownPhase1REST bool) UpstreamClient {
	if proxy == nil {
		return fallback
	}
	if fallback == nil {
		return proxy
	}
	return &s0s1SelectingUpstream{
		s0s1Proxy:            proxy,
		fallback:             fallback,
		denyUnknownPhase1:    denyUnknownPhase1REST,
		rejectPhase1Static:   denyUnknownPhase1REST && envLiveNoStaticPhase1(),
		upstreamUnavailable:  NewUpstreamUnavailable503(),
		upstreamPhase1Static: newUpstreamFixed503(phase1StaticDisabledBody),
	}
}

func NewPhase1SelectingUpstream(s0s1Proxy, _ *RESTProxyUpstream, fallback UpstreamClient, denyUnknownPhase1REST bool) UpstreamClient {
	return NewS0S1SelectingUpstream(s0s1Proxy, fallback, denyUnknownPhase1REST)
}

func (s *s0s1SelectingUpstream) Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	if IsS0S1RESTProxyRoute(req.Method, req.Path) && s.s0s1Proxy != nil {
		return s.s0s1Proxy.Call(ctx, req)
	}
	if s.denyUnknownPhase1 {
		if _, ok := matchPhase1Route(req.Method, req.Path); !ok {
			return s.upstreamUnavailable.Call(ctx, req)
		}
	}
	if s.rejectPhase1Static {
		return s.upstreamPhase1Static.Call(ctx, req)
	}
	return s.fallback.Call(ctx, req)
}

type upstreamUnavailable503 struct{}

func NewUpstreamUnavailable503() UpstreamClient {
	un := upstreamUnavailable503{}
	return &un
}

func (un *upstreamUnavailable503) Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	_, _ = ctx, req
	body := `{"error":{"code":"UPSTREAM_UNAVAILABLE","message":"unknown REST path under live upstream mode; not part of the ARE Foundation S0/S1 contract"}}`
	return UpstreamResponse{StatusCode: http.StatusServiceUnavailable, Body: []byte(body)}, nil
}

type upstreamFixed503 struct {
	body []byte
}

func newUpstreamFixed503(jsonBody string) UpstreamClient {
	return &upstreamFixed503{body: []byte(jsonBody)}
}

func (u *upstreamFixed503) Call(ctx context.Context, req UpstreamRequest) (UpstreamResponse, error) {
	_, _ = ctx, req
	return UpstreamResponse{StatusCode: http.StatusServiceUnavailable, Body: u.body}, nil
}
