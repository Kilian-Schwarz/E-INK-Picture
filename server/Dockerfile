FROM golang:1.22-alpine AS builder

ARG GOARCH=arm64

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${GOARCH} go build -ldflags="-s -w" -o server .

FROM alpine:3.19

RUN apk add --no-cache wget ca-certificates tzdata

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /build/server .

RUN mkdir -p /app/data/designs /app/data/uploaded_images /app/data/fonts /app/data/weather_styles && \
    chown -R app:app /app

USER app

ENV TZ=Europe/Berlin
ENV PORT=5000
ENV DATA_DIR=/app/data

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5000/health || exit 1

CMD ["./server"]
