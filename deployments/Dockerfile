# ============================================================
# BUILD STAGE
# ============================================================

FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" \
    -o flexiproxy \
    ./cmd/flexiproxy

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" \
    -o backend-demo \
    ./cmd/backend-demo


# ============================================================
# RUNTIME STAGE
# ============================================================

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/flexiproxy .
COPY --from=builder /app/backend-demo .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/dashboard ./dashboard
COPY --from=builder /app/deployments/start.sh ./start.sh

RUN chmod +x ./start.sh

EXPOSE 10000

ENTRYPOINT ["./start.sh"]