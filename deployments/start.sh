#!/bin/sh

set -e

echo "=========================================="
echo "Starting FlexiProxy cloud deployment"
echo "=========================================="

echo "Starting backend-1 on port 8001..."
./backend-demo --port 8001 --id backend-1 &

echo "Starting backend-2 on port 8002..."
./backend-demo --port 8002 --id backend-2 &

echo "Starting backend-3 on port 8003..."
./backend-demo --port 8003 --id backend-3 &

echo "Waiting for backend servers..."

sleep 2

echo "Starting FlexiProxy..."

exec ./flexiproxy --config configs/flexiproxy.yaml