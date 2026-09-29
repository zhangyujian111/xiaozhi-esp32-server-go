# xiaozhi-esp32-server-go

A high-performance Go server for the xiaozhi ESP32 voice assistant.

## Quick Start

```bash
go build ./cmd/server
./server.exe
```

## Project Structure

- `cmd/server/` - Application entry point
- `internal/` - Core business logic
- `configs/` - Configuration files
- `deploy/` - Deployment manifests
- `migrations/` - Database migrations
- `test/` - Test suites

## Development

```bash
go mod tidy
go build ./cmd/server
go test ./...
```
