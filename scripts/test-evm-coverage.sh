#!/bin/bash
# EVM Test Coverage Report Generator
# This script runs tests and generates coverage reports

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
PKG_DIR="$PROJECT_ROOT/pkg/evm"
OUTPUT_DIR="$PROJECT_ROOT/docs/reports"

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "=== EVM Test Coverage Report ==="
echo "Project Root: $PROJECT_ROOT"
echo "Package: $PKG_DIR"
echo "Output: $OUTPUT_DIR"
echo ""

# Check if go is available
if ! command -v /usr/local/go/bin/go &> /dev/null; then
    echo "Error: Go not found. Please install Go 1.24+"
    exit 1
fi

# Set GOBIN
export PATH="/usr/local/go/bin:$PATH"
export GOPATH="$PROJECT_ROOT"

# Run tests with coverage
echo "Running tests with coverage..."
cd "$PROJECT_ROOT"

# Run unit tests first
echo ""
echo "=== Unit Tests ==="
go test -v ./pkg/aal/... -count=1 || true

# Run tests with coverage
echo ""
echo "=== Coverage Report ==="
go test -v -coverprofile="$OUTPUT_DIR/coverage.out" ./pkg/aal/... -count=1 || true

# Generate HTML coverage report
if [ -f "$OUTPUT_DIR/coverage.out" ]; then
    go tool cover -html="$OUTPUT_DIR/coverage.out" -o "$OUTPUT_DIR/coverage.html"
    echo "Coverage report generated: $OUTPUT_DIR/coverage.html"
    echo "HTTPS link: https://www.aib.one:51200/reports/coverage.html"
fi

# Display coverage summary
if [ -f "$OUTPUT_DIR/coverage.out" ]; then
    echo ""
    echo "=== Coverage Summary ==="
    go tool cover -func="$OUTPUT_DIR/coverage.out" | tail -1
fi

# Run benchmarks
echo ""
echo "=== Benchmark Results ==="
go test -bench=. -benchmem ./pkg/aal/... -count=3 | tee "$OUTPUT_DIR/benchmarks.txt"

echo ""
echo "=== Report Generation Complete ==="
echo "Files generated:"
ls -la "$OUTPUT_DIR"

exit 0
