package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"are-api-gateway/internal/gateway"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Config is runtime configuration for API gateway.
type Config struct {
	HTTPSPort                 int
	HTTPPort                  int
	GRPCPort                  int
	InsecureGRPC              int
	HealthPort                int
	MetricsPort               int
	CertFile                  string
	KeyFile                   string
	CAFile                    string
	JWKSURL                   string
	EnableInsecureConformance bool
	PollInterval              time.Duration
}

func main() {
	if err := run(context.Background(), loadConfig(), os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() Config {
	return Config{
		HTTPSPort:                 envInt("ARE_GW_HTTPS_PORT", 443),
		HTTPPort:                  envInt("ARE_GW_HTTP_PORT", 8085),
		GRPCPort:                  envInt("ARE_GW_GRPC_PORT", 9100),
		InsecureGRPC:              envInt("ARE_GW_INSECURE_GRPC_PORT", 9101),
		MetricsPort:               envInt("ARE_GW_METRICS_PORT", 8090),
		HealthPort:                envInt("ARE_GW_HEALTH_PORT", 8080),
		CertFile:                  os.Getenv("ARE_GW_TLS_CERT_FILE"),
		KeyFile:                   os.Getenv("ARE_GW_TLS_KEY_FILE"),
		CAFile:                    os.Getenv("ARE_GW_TLS_CA_FILE"),
		JWKSURL:                   os.Getenv("ARE_GW_JWKS_URL"),
		EnableInsecureConformance: os.Getenv("ARE_GW_ENABLE_INSECURE_CONFORMANCE") == "true",
		PollInterval:              250 * time.Millisecond,
	}
}

func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func buildRESTProxy(base, tlsServerName string, cfg Config) (*gateway.RESTProxyUpstream, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("parse proxy base: %w", err)
	}
	upstreamHTTPTimeout := gateway.HTTPUpstreamTimeoutOrDefault()
	var httpClient *http.Client
	if u.Scheme == "http" {
		httpClient = gateway.NewHTTPClientForRESTProxy(upstreamHTTPTimeout)
	} else {
		httpClient, err = gateway.NewHTTPClientForGatewayMTLS(
			cfg.CertFile,
			cfg.KeyFile,
			cfg.CAFile,
			tlsServerName,
			upstreamHTTPTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf("http proxy client: %w", err)
		}
	}
	return gateway.NewRESTProxyUpstream(base, httpClient)
}

func run(ctx context.Context, cfg Config, loggerWriter io.Writer) error {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return errors.New("tls cert, key and ca are required")
	}
	if cfg.JWKSURL == "" {
		return errors.New("jwks url is required")
	}
	if _, err := os.Stat(cfg.CertFile); err != nil {
		return fmt.Errorf("stat cert file: %w", err)
	}
	if _, err := os.Stat(cfg.KeyFile); err != nil {
		return fmt.Errorf("stat key file: %w", err)
	}
	if _, err := os.Stat(cfg.CAFile); err != nil {
		return fmt.Errorf("stat ca file: %w", err)
	}

	allowInsecureFromEnv := strings.EqualFold(os.Getenv("ARE_GW_ALLOW_INSECURE_TEST_TOKEN"), "true")
	if allowInsecureFromEnv && !gateway.DevBuildSupportsInsecureTestToken() {
		return errors.New(
			"ARE_GW_ALLOW_INSECURE_TEST_TOKEN is set but this binary was built without the Go build tag 'dev' " +
				"(the test-token bypass is omitted from release builds); use a JWT or rebuild with " +
				"--build-arg GO_BUILD_TAGS=dev for local compose (see sx/are-api-gateway/README.md)",
		)
	}
	allowInsecureTestToken := allowInsecureFromEnv && gateway.DevBuildSupportsInsecureTestToken()
	if allowInsecureTestToken && !strings.EqualFold(os.Getenv("ARE_GW_DEV_UNSAFE_MODE"), "true") {
		return errors.New(
			"ARE_GW_ALLOW_INSECURE_TEST_TOKEN is set without ARE_GW_DEV_UNSAFE_MODE=true; refusing to start " +
				"(see sx/are-api-gateway/README.md)",
		)
	}
	if cfg.EnableInsecureConformance && !strings.EqualFold(os.Getenv("ARE_GW_DEV_UNSAFE_MODE"), "true") {
		return errors.New(
			"ARE_GW_ENABLE_INSECURE_CONFORMANCE=true requires ARE_GW_DEV_UNSAFE_MODE=true; plaintext HTTP/gRPC listeners are dev-only " +
				"(see sx/are-api-gateway/README.md)",
		)
	}
	upstreamMode, err := gateway.LoadUpstreamMode()
	if err != nil {
		return err
	}
	if err := gateway.ValidateUpstreamModeRequirements(upstreamMode); err != nil {
		return err
	}

	// Insecure test-token policy is resolved only here at startup (env + dev build tag); TokenValidator has no per-request getenv.
	validator, err := gateway.NewJWKSValidator(ctx, cfg.JWKSURL, allowInsecureTestToken)
	if err != nil {
		return fmt.Errorf("create validator: %w", err)
	}
	staticUpstream := &gateway.StaticUpstreamClient{}
	if opaURL := strings.TrimSpace(os.Getenv("ARE_GW_OPA_URL")); opaURL != "" {
		ms := envInt("ARE_GW_OPA_TIMEOUT_MS", 5000)
		staticUpstream.SetOPA(opaURL, &http.Client{Timeout: time.Duration(ms) * time.Millisecond})
	}
	upstream := gateway.UpstreamClient(staticUpstream)
	var s0s1Proxy *gateway.RESTProxyUpstream
	if base := strings.TrimSpace(os.Getenv("ARE_GW_S0S1_HTTP_PROXY_BASE")); base != "" {
		proxy, err := buildRESTProxy(base, os.Getenv("ARE_GW_S0S1_HTTP_TLS_SERVER_NAME"), cfg)
		if err != nil {
			return fmt.Errorf("s0s1 http proxy: %w", err)
		}
		s0s1Proxy = proxy
	}
	if s0s1Proxy != nil {
		upstream = gateway.NewS0S1SelectingUpstream(s0s1Proxy, staticUpstream, upstreamMode == gateway.UpstreamModeLive)
	}
	gwHandler := gateway.NewGateway(validator, upstream, loggerWriter)
	metricsSrv := (*http.Server)(nil)
	if cfg.MetricsPort > 0 {
		metricsSrv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.MetricsPort),
			Handler: gwHandler.MetricsPrometheusHandler(),
		}
	}
	corsOrigins := gateway.ParseCORSAllowedOrigins(os.Getenv("ARE_GW_CORS_ALLOWED_ORIGINS"))
	httpHandler := gateway.WrapCORSIfConfigured(corsOrigins, gwHandler.Handler())

	tlsCfg, err := newTLSConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		return err
	}

	httpsServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", cfg.HTTPSPort),
		Handler:   httpHandler,
		TLSConfig: tlsCfg,
	}
	healthMux := http.NewServeMux()
	healthPlain := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	}
	healthMux.HandleFunc("/healthz", healthPlain)
	kafkaBrokers := gateway.ParseKafkaBootstrap(os.Getenv("ARE_GW_KAFKA_BOOTSTRAP_SERVERS"))
	readinessCfg := gateway.ReadinessConfig{
		JWKSURL:      cfg.JWKSURL,
		KafkaBrokers: kafkaBrokers,
		ProbeTimeout: 5 * time.Second,
	}
	if strings.EqualFold(os.Getenv("ARE_GW_JWKS_READINESS_USE_MTLS"), "true") {
		jwksHTTP, err := gateway.NewHTTPClientForGatewayMTLS(
			cfg.CertFile,
			cfg.KeyFile,
			cfg.CAFile,
			os.Getenv("ARE_GW_JWKS_TLS_SERVER_NAME"),
			5*time.Second,
		)
		if err != nil {
			return fmt.Errorf("jwks readiness mTLS http client: %w", err)
		}
		readinessCfg.HTTPClient = jwksHTTP
	}
	healthMux.HandleFunc("/readyz", gateway.ServeReadiness(readinessCfg))
	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.HealthPort),
		Handler: healthMux,
	}
	var insecureHTTPServer *http.Server
	var insecureGRPCServer *grpc.Server
	var insecureGRPCListener net.Listener
	if cfg.EnableInsecureConformance {
		insecureHTTPServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.HTTPPort),
			Handler: httpHandler,
		}
		insecureGRPCServer = grpc.NewServer()
		gateway.RegisterPhase1ConformanceGRPC(insecureGRPCServer)
		insecureGRPCListener, err = net.Listen("tcp", fmt.Sprintf(":%d", cfg.InsecureGRPC))
		if err != nil {
			return fmt.Errorf("listen insecure grpc: %w", err)
		}
	}
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))
	gateway.RegisterPhase1ConformanceGRPC(grpcServer)
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen grpc: %w", err)
	}

	proc := gateway.NewOutboxProcessor(cfg.PollInterval)
	go proc.Run(ctx)

	errCh := make(chan error, 6)
	go func() {
		errCh <- httpsServer.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
	}()
	go func() {
		errCh <- healthServer.ListenAndServe()
	}()
	if metricsSrv != nil {
		go func() {
			errCh <- metricsSrv.ListenAndServe()
		}()
	}
	go func() {
		errCh <- grpcServer.Serve(grpcListener)
	}()
	if cfg.EnableInsecureConformance {
		go func() {
			errCh <- insecureHTTPServer.ListenAndServe()
		}()
		go func() {
			errCh <- insecureGRPCServer.Serve(insecureGRPCListener)
		}()
	}

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		if insecureGRPCServer != nil {
			insecureGRPCServer.GracefulStop()
		}
		_ = httpsServer.Shutdown(context.Background())
		_ = healthServer.Shutdown(context.Background())
		if metricsSrv != nil {
			_ = metricsSrv.Shutdown(context.Background())
		}
		if insecureHTTPServer != nil {
			_ = insecureHTTPServer.Shutdown(context.Background())
		}
		return nil
	case err := <-errCh:
		return err
	}
}

func newTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load tls keypair: %w", err)
	}
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("append ca cert failed")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}
