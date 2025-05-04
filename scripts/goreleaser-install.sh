#!/bin/bash
set -e

# Script to install GoReleaser locally for testing

echo "Installing GoReleaser for local release testing..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go first."
    exit 1
fi

# Create directory for the script if it doesn't exist
mkdir -p "$(dirname "$0")"

# Install latest version of GoReleaser
echo "Installing GoReleaser..."
go install github.com/goreleaser/goreleaser@latest

# Verify installation
if command -v goreleaser &> /dev/null; then
    echo "GoReleaser successfully installed!"
    echo "Version: $(goreleaser --version)"
    echo ""
    echo "To test a release locally (without publishing):"
    echo "goreleaser release --snapshot --clean --skip=publish"
    echo ""
    echo "To check your .goreleaser.yml configuration:"
    echo "goreleaser check"
else
    echo "Error: GoReleaser installation failed."
    echo "Please ensure your GOPATH/bin is in your PATH."
    exit 1
fi 