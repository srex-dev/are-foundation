FROM golang:1.25.9-bookworm AS build
WORKDIR /src
# Build context must be the ARE repo root (see tools/deployment/build-local-compose-images.py)
# so ../../tools/testing/policyfixtures from go.mod replace is present.
COPY sx/are-api-gateway/go.mod sx/are-api-gateway/go.sum ./sx/are-api-gateway/
COPY tools/testing/policyfixtures ./tools/testing/policyfixtures
WORKDIR /src/sx/are-api-gateway
RUN go mod download

COPY sx/are-api-gateway/ ./
# Omit GO_BUILD_TAGS for release images (no test-token bypass). Local compose passes GO_BUILD_TAGS=dev.
ARG GO_BUILD_TAGS=
RUN set -e; \
  if [ -n "$GO_BUILD_TAGS" ]; then \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags "$GO_BUILD_TAGS" -o /out/are-api-gateway ./cmd/are-api-gateway; \
  else \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/are-api-gateway ./cmd/are-api-gateway; \
  fi

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/are-api-gateway /app/are-api-gateway
EXPOSE 443 9100 8090 8080
ENTRYPOINT ["/app/are-api-gateway"]
