FROM rust:1.41.1 as builder
RUN mkdir -p /src
WORKDIR /src
COPY . .
RUN rustup component add rustfmt
RUN cargo build --bin statagg --release

FROM debian:buster-slim
COPY --from=builder /src/target/release/statagg /
RUN apt update -y && apt install -y libssl-dev
CMD ["/statagg"]
