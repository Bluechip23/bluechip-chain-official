# ---- Build Stage ----
FROM golang:1.23-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make install && cp $(go env GOPATH)/bin/bluechipd /usr/local/bin/bluechipd

# ---- Runtime Stage ----
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y ca-certificates curl jq && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/bluechipd /usr/local/bin/bluechipd

ENV NODE_HOME=/root/.bluechipChain

EXPOSE 26656 26657 1317 9090

ENTRYPOINT ["bluechipd"]
CMD ["start", "--home", "/root/.bluechipChain"]
