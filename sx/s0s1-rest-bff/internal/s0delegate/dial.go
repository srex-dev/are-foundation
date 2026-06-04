// Package s0delegate provides optional gRPC clients to ARE-A-S0-001 / ARE-A-S0-005 (iter-006).
package s0delegate

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	registryv1 "github.com/srex-dev/are-foundation/s0/agent-registry-service/proto"
	passportv1 "github.com/srex-dev/are-foundation/s0/passport-issuance-engine/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Backend holds optional gRPC clients. Zero value is valid (no delegation).
type Backend struct {
	Reg  registryv1.AgentRegistryServiceClient
	Pass passportv1.PassportIssuanceServiceClient
	// conns closed on Close
	conns []*grpc.ClientConn
}

// Close closes dialled gRPC connections.
func (b *Backend) Close() error {
	var first error
	for _, c := range b.conns {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Config for Dial.
type Config struct {
	RegistryAddr string
	PassportAddr string
	// mTLS (default): use these PEM files for client cert and server trust.
	CertFile, KeyFile, CAFile string
	// TLSServerName overrides SNI / verify hostname (optional).
	TLSServerName string
	// Insecure skips TLS (local dev only).
	Insecure bool
}

// Dial opens gRPC connections for any non-empty addresses. Same cert material as s0s1 BFF HTTPS server is typical.
func Dial(cfg Config) (*Backend, error) {
	regAddr := strings.TrimSpace(cfg.RegistryAddr)
	passAddr := strings.TrimSpace(cfg.PassportAddr)
	if regAddr == "" && passAddr == "" {
		return &Backend{}, nil
	}
	dialOne := func(addr string) (*grpc.ClientConn, error) {
		if cfg.Insecure {
			return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
		if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
			return nil, errors.New("s0delegate: cert, key, and ca required when not insecure")
		}
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("s0delegate client tls: %w", err)
		}
		caBytes, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("s0delegate read ca: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, errors.New("s0delegate append ca failed")
		}
		tlsCfg := &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
		}
		if sn := strings.TrimSpace(cfg.TLSServerName); sn != "" {
			tlsCfg.ServerName = sn
		}
		creds := credentials.NewTLS(tlsCfg)
		return grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	}

	var b Backend
	if regAddr != "" {
		c, err := dialOne(regAddr)
		if err != nil {
			return nil, fmt.Errorf("registry grpc: %w", err)
		}
		b.conns = append(b.conns, c)
		b.Reg = registryv1.NewAgentRegistryServiceClient(c)
	}
	if passAddr != "" {
		c, err := dialOne(passAddr)
		if err != nil {
			_ = b.Close()
			return nil, fmt.Errorf("passport grpc: %w", err)
		}
		b.conns = append(b.conns, c)
		b.Pass = passportv1.NewPassportIssuanceServiceClient(c)
	}
	return &b, nil
}
