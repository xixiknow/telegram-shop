# ---- build stage ----
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 利用层缓存：先拷贝 go.mod/go.sum
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /telegram-shop ./cmd/bot

# ---- runtime stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

WORKDIR /app
COPY --from=builder /telegram-shop /app/telegram-shop

EXPOSE 8080

ENTRYPOINT ["/app/telegram-shop", "-config", "/app/config.yaml"]
