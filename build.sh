#!/bin/bash
set -e
if ! command -v go &> /dev/null; then
    echo "Go is not installed or not in PATH."
else
    echo "Running build/main.go..."
    go run build/main.go
    echo "Building final executable..."
    go build -o renpy-save-editor main.go
    echo "Running tests..."
    go test -v .
    echo "Build complete."
fi
