FROM golang:1.24-alpine AS builder

# Optional: custom CA certs for SSL-inspecting proxies
# Place .pem files in server/certs/ before building
COPY certs/ /tmp/certs/
RUN if ls /tmp/certs/*.pem 1>/dev/null 2>&1; then \
        cp /tmp/certs/*.pem /usr/local/share/ca-certificates/ && \
        update-ca-certificates; \
    fi

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server . && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o healthcheck ./cmd/healthcheck

# Minimal runtime - static Go binary, no package manager needed
FROM scratch

# Timezone data (Go's embedded zoneinfo)
COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /usr/local/go/lib/time/zoneinfo.zip
ENV ZONEINFO=/usr/local/go/lib/time/zoneinfo.zip

# CA certificates (includes custom CAs if added in builder)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Binaries
COPY --from=builder /build/server /server
COPY --from=builder /build/healthcheck /healthcheck

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD ["/healthcheck"]

ENTRYPOINT ["/server"]
