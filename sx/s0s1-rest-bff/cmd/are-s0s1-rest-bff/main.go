package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"are-s0s1-rest-bff/internal/api"
	"are-s0s1-rest-bff/internal/policy"
	"are-s0s1-rest-bff/internal/repo"
	"are-s0s1-rest-bff/internal/s0delegate"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	certFile := os.Getenv("ARE_S0S1_TLS_CERT_FILE")
	keyFile := os.Getenv("ARE_S0S1_TLS_KEY_FILE")
	caFile := os.Getenv("ARE_S0S1_TLS_CA_FILE")
	if certFile == "" || keyFile == "" || caFile == "" {
		return errors.New("ARE_S0S1_TLS_CERT_FILE, ARE_S0S1_TLS_KEY_FILE, and ARE_S0S1_TLS_CA_FILE are required")
	}
	httpsPort := envInt("ARE_S0S1_HTTPS_PORT", 8443)
	healthPort := envInt("ARE_S0S1_HEALTH_PORT", 8099)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var repository repo.Repository
	var pg *repo.Postgres
	if dsn := strings.TrimSpace(os.Getenv("ARE_S0S1_DATABASE_URL")); dsn != "" {
		var err error
		pg, err = repo.NewPostgres(ctx, dsn)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		defer pg.Close()
		repository = pg
		log.Printf("s0s1-rest-bff using Postgres backend")
	} else {
		repository = repo.NewMemory()
		log.Printf("s0s1-rest-bff using in-memory backend")
	}

	keyPair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load server cert: %w", err)
	}
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return fmt.Errorf("read ca: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return errors.New("append ca pem failed")
	}
	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{keyPair},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}

	var pol policy.Evaluator = policy.Stub{}
	if opaBase := strings.TrimSpace(os.Getenv("ARE_S0S1_OPA_URL")); opaBase != "" {
		ms := envInt("ARE_S0S1_OPA_TIMEOUT_MS", 5000)
		pol = policy.NewOPAClient(opaBase, time.Duration(ms)*time.Millisecond)
		log.Printf("s0s1-rest-bff policy via OPA at %s", opaBase)
	}
	var s0BE *s0delegate.Backend
	if strings.TrimSpace(os.Getenv("ARE_S0S1_S0_DELEGATE")) != "0" {
		regAddr := strings.TrimSpace(os.Getenv("ARE_S0S1_REGISTRY_GRPC_ADDR"))
		passAddr := strings.TrimSpace(os.Getenv("ARE_S0S1_PASSPORT_GRPC_ADDR"))
		if regAddr != "" || passAddr != "" {
			b, derr := s0delegate.Dial(s0delegate.Config{
				RegistryAddr:  regAddr,
				PassportAddr:  passAddr,
				CertFile:      certFile,
				KeyFile:       keyFile,
				CAFile:        caFile,
				TLSServerName: strings.TrimSpace(os.Getenv("ARE_S0S1_S0_GRPC_TLS_SERVER_NAME")),
				Insecure:      os.Getenv("ARE_S0S1_S0_GRPC_INSECURE") == "1",
			})
			if derr != nil {
				return fmt.Errorf("s0 delegate dial: %w", derr)
			}
			s0BE = b
			defer func() { _ = s0BE.Close() }()
			if regAddr != "" {
				log.Printf("s0s1-rest-bff registry gRPC delegate at %s", regAddr)
			}
			if passAddr != "" {
				log.Printf("s0s1-rest-bff passport gRPC delegate at %s", passAddr)
			}
		}
	}
	apiHandler, err := api.Handler(api.Config{Repo: repository, Policy: pol, S0: s0BE})
	if err != nil {
		return fmt.Errorf("api handler: %w", err)
	}

	httpsSrv := &http.Server{
		Addr:      fmt.Sprintf(":%d", httpsPort),
		Handler:   withRequiredHeaders(apiHandler),
		TLSConfig: tlsCfg,
	}
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", healthPort),
		Handler: healthMux,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("s0s1-rest-bff mTLS listening on :%d", httpsPort)
		errCh <- httpsSrv.ListenAndServeTLS("", "")
	}()
	go func() {
		log.Printf("s0s1-rest-bff health on :%d", healthPort)
		errCh <- healthSrv.ListenAndServe()
	}()
	if serveErr := <-errCh; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

// Client-supplied "stage" headers must never influence authorization; strip before handlers.
func stripUntrustedStageSpoofHeaders(r *http.Request) {
	for _, h := range []string{
		"X-Are-Ui-Stage",
		"X-Are-Session-Stage",
		"X-Are-Client-Stage",
		"X-Are-Deployment-Stage",
	} {
		r.Header.Del(h)
	}
}

// Gateway always sends these; enforce for direct callers too.
func withRequiredHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripUntrustedStageSpoofHeaders(r)
		if r.Header.Get("X-Request-ID") == "" {
			http.Error(w, "missing request id", http.StatusBadRequest)
			return
		}
		if strings.EqualFold(r.Method, http.MethodPost) {
			switch r.URL.Path {
			case "/v1/identity/agents", "/v1/passports", "/v1/passports:verify", "/v1/enforcement/scope:evaluate", "/v1/policy/evaluations":
				if r.Header.Get("Idempotency-Key") == "" {
					http.Error(w, "missing idempotency key", http.StatusBadRequest)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
