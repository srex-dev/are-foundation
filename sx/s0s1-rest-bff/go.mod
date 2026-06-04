module are-s0s1-rest-bff

go 1.25.9

require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/prometheus/client_golang v1.23.2
	github.com/srex-dev/are-foundation/s0/agent-registry-service v0.0.0
	github.com/srex-dev/are-foundation/s0/passport-issuance-engine v0.0.0
	github.com/srex-dev/are-foundation/tools/testing/policyfixtures v0.0.0
	google.golang.org/grpc v1.80.0
)

replace github.com/srex-dev/are-foundation/s0/agent-registry-service => ../../s0/agent-registry-service

replace github.com/srex-dev/are-foundation/s0/passport-issuance-engine => ../../s0/passport-issuance-engine

replace github.com/srex-dev/are-foundation/tools/testing/policyfixtures => ../../tools/testing/policyfixtures

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260209200024-4cfbd4190f57 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
