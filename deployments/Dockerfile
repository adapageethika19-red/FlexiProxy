# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy module files first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy full source tree
COPY . .

# Build static CGO-disabled binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o flexiproxy ./cmd/flexiproxy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o backend-demo ./cmd/backend-demo

# Runtime Stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries and dashboard assets from builder stage
COPY --from=builder /app/flexiproxy .
COPY --from=builder /app/backend-demo .
COPY --from=builder /app/configs ./configs
COPY --from=builder /app/dashboard ./dashboard

EXPOSE 8080 8081

ENTRYPOINT ["./flexiproxy"]
CMD ["--config", "configs/flexiproxy.yaml"]
