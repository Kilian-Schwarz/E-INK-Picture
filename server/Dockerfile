FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .

FROM alpine:3.19

RUN apk --no-cache add ca-certificates wget tzdata

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /build/server .

RUN mkdir -p data/designs data/uploaded_images data/fonts data/weather_styles && \
    chown -R app:app /app

USER app

EXPOSE 5000

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget --spider -q http://localhost:5000/health || exit 1

ENTRYPOINT ["./server"]
