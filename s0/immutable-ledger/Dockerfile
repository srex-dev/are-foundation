FROM rust:1.90-bookworm AS builder

WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
COPY Cargo.toml build.rs ./
COPY proto ./proto
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/target/release/are-immutable-ledger /usr/local/bin/are-immutable-ledger
EXPOSE 9092 8080 8083
ENTRYPOINT ["/usr/local/bin/are-immutable-ledger"]

