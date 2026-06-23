# RalphSpec command deck. Pretty output is enabled only for terminals that can show it.

set dotenv-load := true
set windows-shell := ["pwsh.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command"]
set shell := ["pwsh.exe", "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command"]

_run *args:
    @& "{{justfile_directory()}}/scripts/just.ps1" {{args}}

# Show the riced-up command deck
default:
    @just _run help

# Verify local toolchain and release prerequisites
doctor:
    @just _run doctor

# Download Go modules
deps:
    @just _run deps

# Format Go sources
fmt:
    @just _run fmt

# Run go vet
vet:
    @just _run vet

# Run golangci-lint when installed
lint:
    @just _run lint

# Run the race-enabled test suite with coverage
test:
    @just _run test

# Generate coverage.out and coverage.html
coverage:
    @just _run coverage

# Build the local ralph binary
build:
    @just _run build

# Cross-compile release assets into dist/
dist:
    @just _run dist

# Run the full local CI gate
ci:
    @just _run ci

# Remove generated build and coverage artifacts
clean:
    @just _run clean

# Create a local snapshot release in dist/
snapshot:
    @just _run snapshot

# Publish a GitHub release and update LISSTech's Scoop bucket
release version="":
    @just _run release "{{version}}"

# Update only the Scoop manifest for an existing version
scoop version="":
    @just _run scoop "{{version}}"
