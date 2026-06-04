module are-api-gateway

go 1.25.9

require (
	github.com/MicahParks/keyfunc/v3 v3.7.0
	github.com/golang-jwt/jwt/v5 v5.2.3
	github.com/prometheus/client_golang v1.23.2
	github.com/segmentio/kafka-go v0.4.51
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.10
)

replace github.com/srex-dev/are-foundation/tools/testing/policyfixtures => ../../tools/testing/policyfixtures

require (
	github.com/MicahParks/jwkset v0.11.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/time v0.9.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)
