FROM golang:1.25.9-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/are-agent-registry ./cmd/are-agent-registry

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/are-agent-registry /usr/local/bin/are-agent-registry
EXPOSE 9090 8080 8081
ENTRYPOINT ["/usr/local/bin/are-agent-registry"]
