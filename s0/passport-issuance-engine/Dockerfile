FROM golang:1.25.9-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/are-passport-issuance ./cmd/are-passport-issuance

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/are-passport-issuance /usr/local/bin/are-passport-issuance
EXPOSE 9094 8080 8085
ENTRYPOINT ["/usr/local/bin/are-passport-issuance"]
