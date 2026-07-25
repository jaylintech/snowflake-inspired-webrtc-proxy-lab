# Testing

This project uses the standard Go testing toolchain.

## Quick Start

```bash
go test ./...
```

## Test Categories

### Unit Tests

Unit tests live alongside the code they test in `_test.go` files.

| Package | What's tested |
| --- | --- |
| `internal/lab` | Signaling helpers, SDP serialization, ICE configuration validation |
| `internal/relay` | Target URL bounding, response chunking, response body encoding |
| `cmd/broker` | HTTP signaling handler, session CRUD, CORS, path parsing |
| `cmd/webclient` | Chunked response assembler |
| `cmd/relay` | Target URL bounding, redirect validation, body encoding, path parsing |

### Integration Tests

Integration tests in `internal/lab` verify the signaling layer end-to-end:

- `TestSignalRoundTripThroughBroker` - POST/GET round trip through broker
- `TestWebRTCEndToEndThroughBrokerSignaling` - Full WebRTC DataChannel through broker signaling
- `TestPollSignalReturnsSignal` - Polling behavior
- Other signal lifecycle tests

### Running Tests

```bash
# All tests
go test -count=1 ./...

# With race detector
go test -race -count=1 ./...

# Verbose output
go test -v -count=1 ./...

# Specific package
go test -v -count=1 ./internal/lab/

# Specific test
go test -v -count=1 -run TestSignalRoundTrip ./internal/lab/
```

### Race Detector

Always run with `-race` before submitting changes:

```bash
go test -race -count=1 ./...
```

## CI

The GitHub Actions workflow (`build.yml`) runs:

1. `go test ./...` with the race detector
2. `golangci-lint` (config: `.golangci.yml`)
3. `go build -o bin ./cmd/...`
