FROM golang:1.25.9-bookworm AS build
WORKDIR /src
# Build context must be the ARE repo root so ../../tools/testing/policyfixtures from go.mod replace is present.
COPY sx/s0s1-rest-bff/go.mod sx/s0s1-rest-bff/go.sum ./sx/s0s1-rest-bff/
COPY tools/testing/policyfixtures ./tools/testing/policyfixtures
# go.mod replace() targets (gRPC proto modules); must exist before go mod download
COPY s0/agent-registry-service ./s0/agent-registry-service
COPY s0/passport-issuance-engine ./s0/passport-issuance-engine
WORKDIR /src/sx/s0s1-rest-bff
RUN go mod download

COPY sx/s0s1-rest-bff/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/are-s0s1-rest-bff ./cmd/are-s0s1-rest-bff

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/are-s0s1-rest-bff /app/are-s0s1-rest-bff
EXPOSE 8443 8099
ENTRYPOINT ["/app/are-s0s1-rest-bff"]
