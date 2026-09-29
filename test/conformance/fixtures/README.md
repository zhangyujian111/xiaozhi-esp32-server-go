# Protocol Fixtures

This directory contains 100+ JSON fixtures for protocol conformance testing.

## Format

Each fixture is a JSON file with this structure:

```json
{
  "name": "001_hello_basic_v1",
  "type": "hello",
  "input_hex": "7b2274797065223a2268656c6c6f22...",
  "expected": {
    "type": "hello",
    "version": 1,
    "transport": "websocket"
  },
  "expect_error": false
}
```

- `input_hex`: hex-encoded JSON (or binary) message payload
- `expected`: map of expected parsed field values
- `expect_error`: true if this fixture is an error case

## Naming Convention

`<index>_<message_type>_<variant>.json`

## Categories

| Type | Count | Description |
|------|-------|-------------|
| hello | 5 | Device hello with version/features |
| listen | 10 | Listen state transitions |
| abort | 3 | Abort speaking |
| ack | 5 | Message acknowledgment |
| tts | 8 | Text-to-speech events |
| stt | 5 | Speech-to-text results |
| llm | 5 | LLM response events |
| mcp | 10 | MCP protocol messages |
| iot | 5 | IoT control messages |
| alert | 3 | Device alerts |
| system | 3 | System events |
| binary_v1 | 5 | Raw binary frames v1 |
| binary_v2 | 15 | Binary protocol v2 |
| binary_v3 | 15 | Binary protocol v3 |
| malformed | 8 | Error/malformed cases |

## Running Tests

```bash
# Run all fixture tests (default with !nolong tag)
go test ./internal/protocol/... -run TestFixture

# Run specific category
go test ./internal/protocol/... -run TestFixture_Hello

# Skip long-running fixture tests
go test ./internal/protocol/... -tags nolong -run TestFixture
```
