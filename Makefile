.PHONY: all build test test-race clean run-proxy run-backend

BINARY_PROXY=bin/flexiproxy
BINARY_BACKEND=bin/backend-demo

all: build test

build:
	go build -o $(BINARY_PROXY) ./cmd/flexiproxy
	go build -o $(BINARY_BACKEND) ./cmd/backend-demo

test:
	go test -v ./...

test-race:
	go test -v -race ./...

clean:
	rm -rf bin/

run-proxy: build
	./$(BINARY_PROXY) --config configs/flexiproxy.yaml

run-backend: build
	./$(BINARY_BACKEND) --port 8001 --id backend-1
