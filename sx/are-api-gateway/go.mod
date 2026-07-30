module are-api-gateway

go 1.25.9

require (
	github.com/MicahParks/keyfunc/v3 v3.8.1
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/prometheus/client_golang v1.24.1
	github.com/segmentio/kafka-go v0.4.51
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.11
)

replace github.com/srex-dev/are-foundation/tools/testing/policyfixtures => ../../tools/testing/policyfixtures

require (
	github.com/MicahParks/jwkset v0.11.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)
