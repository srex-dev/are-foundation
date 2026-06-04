FROM rust:1.90-bookworm AS builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY build.rs ./
COPY proto ./proto
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/target/release/are-scope-evaluator /usr/local/bin/are-scope-evaluator
EXPOSE 9097 8080 8088
ENTRYPOINT ["/usr/local/bin/are-scope-evaluator"]
