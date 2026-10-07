# xiaozhi-esp32-server-go

> ESP32 设备 ↔ aisaas AI 后端的 Go gateway / proxy
> —— xiaozhi-esp32-server 的 Go 重写版，与 [ykt-aisaas](https://github.com/zhangyujian111/ykt-aisaas) 配套使用。

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](go.mod)
[![Build](https://img.shields.io/badge/build-passing-brightgreen?style=flat)](.github/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat)](LICENSE)
[![Backend: aisaas](https://img.shields.io/badge/backend-aisaas-orange?style=flat)](https://github.com/zhangyujian111/ykt-aisaas)
[![ESP32 protocol](https://img.shields.io/badge/protocol-xiaozhi--esp32-green?style=flat)](https://github.com/zhangyujian111/xiaozhi-esp32-server-java)

---

## Architecture

xz-go 是部署在 ESP32 设备与 aisaas AI 后端之间的轻量网关，职责是：

- 接收 ESP32 WebSocket（JSON 控制消息 + Opus 音频帧）
- 服务端 VAD 自动断句
- 调 aisaas 完成 ASR → LLM → TTS 流式对话
- 把 aisaas SSE event 翻译成 ESP32 协议（`{type, state, msgId, text}` / opus 帧）
- OTA 设备绑定 + RBAC 鉴权

```
+----------+        JSON + Opus          +-----------------+         HTTP/SSE          +-----------+
|  ESP32   |  <----------------------->  |  xz-go (8082)   |  <-------------------->  |  aisaas   |
| firmware |   WS :8082 (device gw)     |  (this server)  |   :8190 (AI backend)    |  (8190)   |
+----------+                             +-----------------+                          +-----+-----+
                                                                                            |
                                                                                  +---------v---------+
                                                                                  | ASR / LLM / TTS   |
                                                                                  | (aliyun-nls etc.) |
                                                                                  +-------------------+
```

完整字段对照 / 协议约定见 [docs/superpowers/specs/](docs/superpowers/specs/2026-09-29-xiaozhi-esp32-server-go-design.md)。

---

## Features

| 能力 | 说明 |
|---|---|
| WebSocket device gateway | `:8082` 接收 ESP32 JSON 控制消息 + opus 音频帧 |
| Server-side VAD | silero_vad.onnx 自动断句，触发 dialogue turn |
| Streaming dialogue | aisaas SSE 流式：LLM 边生成 → TTS 边合成 → 边下发 opus |
| Legacy dialogue | 兼容老路径（LLM full → TTS 一次合成） |
| aisaas HTTP/SSE client | `internal/aisaas/stream_client.go` 调 ASR/LLM/TTS |
| Stream orchestrator | `internal/dialogue/stream_orchestrator.go` —— aisaas SSE → ESP32 JSON adapter（含 STT / tts-* event 翻译） |
| Opus codec | 16 kHz / mono / 60 ms 帧，集成 [gopus](https://github.com/hraban/opus) |
| OTA device binding | ESP32 首次激活 + 租户绑定（API + 设备注册） |
| RBAC auth | 内部 API 鉴权（`internal/api/`） |
| Pluggable device store | file / MySQL 可切换（`internal/store/`） |
| Protocol codec | ESP32 binary frame 编码（`internal/protocol/binary_writer.go`） |
| Window-memory conversation | per-turn window + persona memory（`internal/dialogue/`） |
| Structured logging | zerolog tee stdout + `<working_dir>/logs/xiaozhi-server.log` |

---

## Quick Start

### Prerequisites

- **Go 1.25+**
- **MySQL 8.x**（设备元数据，DSN 配在 `configs/config.yaml`）
- **aisaas backend** running on `:8190`（见 [ykt-aisaas](https://github.com/zhangyujian111/ykt-aisaas)）
- **silero_vad.onnx** 放到 `./models/silero_vad.onnx`（VAD 模型，运行时按需下载）

### Build & Run

```bash
# Build
go build -o bin/xiaozhi-server-go ./cmd/server

# Run with default config
./bin/xiaozhi-server-go -config configs/config.yaml
```

启动成功后会看到：

```
... listening on :8082 (ws) :8081 (admin)
... connected to aisaas: http://127.0.0.1:8190
... silero VAD loaded: ./models/silero_vad.onnx
```

### Connect an ESP32 device

把设备端的 WebSocket URL 指向 xz-go 的 `:8082`（`public_ws_url`），首次启动会触发 OTA binding 流程（详见 `docs/` 下的 OTA binding plan）。

---

## Configuration

`configs/config.yaml` 关键段：

```yaml
server:
  websocket_addr: ":8082"      # ESP32 WS 端口
  admin_addr: ":8081"          # 管理 API 端口
  public_ws_url: "ws://<host>:8082"
  read_timeout: "30s"
  write_timeout: "30s"
  max_connections: 200
  internal_token: "dev-internal-token"

aisaas:
  base_url: "http://127.0.0.1:8190"
  internal_token: "dev-internal-token"   # 与 aisaas server.internal_token 对齐
  http_timeout: "30s"

vad:
  model_path: "./models/silero_vad.onnx"
  speech_threshold: 0.15
  silence_threshold: 0.10
  silence_duration_ms: 800
  frame_size_samples: 512

opus:
  uplink:   { sample_rate: 16000, channels: 1, frame_duration_ms: 60 }
  downlink: { sample_rate: 16000, channels: 1, frame_duration_ms: 60 }

dialogue:
  per_turn_timeout: 30
  window_memory_size: 20
  ttfs_alert_threshold_ms: 3000
  streaming_enabled: true     # true → SSE 流式；false → 走 legacy orchestrator

database:
  dsn: "root:<password>@tcp(127.0.0.1:3306)/xiaozhi?charset=utf8mb4&parseTime=true&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: "30m"
  migration_path: "./migrations"
```

完整字段参见 `internal/config/config.go`。

---

## Project Structure

```
xiaozhi-esp32-server-go/
├── cmd/
│   └── server/             # 应用入口 (main.go)
├── internal/
│   ├── aisaas/             # aisaas HTTP/SSE client (stream_client, client, llm, stt, tts)
│   ├── api/                # REST API (OTA binding / RBAC)
│   ├── app/                # 应用装配（DI 容器）
│   ├── audio/
│   │   ├── opus/           # Opus codec
│   │   └── vad/            # silero VAD
│   ├── config/             # 配置加载
│   ├── dialogue/           # 对话编排
│   │   ├── orchestrator.go       # legacy full-response 路径
│   │   └── stream_orchestrator.go # SSE 流式路径
│   ├── event/              # 内部事件总线
│   ├── obs/                # 日志 / observability
│   ├── protocol/           # ESP32 协议编解码 (binary / JSON)
│   ├── server/             # HTTP / WebSocket server 装配
│   ├── store/              # 设备存储（file / MySQL）
│   └── ws/                 # WebSocket handler (音频 pipeline / handshake)
├── configs/                # 配置
├── docs/                   # 设计文档 / plans / runbook
├── models/                 # VAD 模型 (.onnx)
├── scripts/                # 构建 / 部署脚本
├── test/                   # 集成 / E2E / conformance 测试
├── bin/                    # 构建产物（gitignored）
└── go.mod
```

---

## Development

### Build

```bash
go build ./cmd/server          # 标准构建
go build -tags noopus ./...    # 不含 opus（用于无 cgo 环境跑单元测试）
```

### Test

```bash
# 全部
go test ./...

# 关键子包（race detector 推荐）
go test -race ./internal/dialogue/...
go test -race ./internal/aisaas/...
go test -race ./internal/ws/...

# 单个测试
go test -v -run TestStreamOrchestrator_TranslatesSttEvent ./internal/dialogue/

# E2E
go test -tags=e2e ./test/e2e_phase1/...
```

### Lint / Vet

```bash
go vet ./...
gofmt -l .
```

---

## ESP32 Protocol Compatibility

xz-go 在 wire-format 层与 [xiaozhi-esp32-server-java](https://github.com/zhangyujian111/xiaozhi-esp32-server-java) 兼容：

| 方向 | aisaas 事件 | ESP32 收到的 JSON / 帧 |
|---|---|---|
| aisaas → xz-go | `tts-start` | `{type:"tts", state:"start", msgId}` |
| aisaas → xz-go | `sentence-start` | `{type:"tts", state:"sentence_start", msgId, text}` |
| aisaas → xz-go | `tts-audio` | (binary opus frame, 60 ms) |
| aisaas → xz-go | `sentence-end` | `{type:"tts", state:"sentence_end", msgId}` |
| aisaas → xz-go | `tts-stop` | `{type:"tts", state:"stop", msgId}` |
| aisaas → xz-go | `stt` | `{type:"stt", text}` |
| aisaas → xz-go | `done` / `error` | (orchestrator 内部处理，不下发) |

V8 实施：aisaas + xz-go 补齐 STT event（对齐 java `MessageSender.sendSttMessage`）。

---

## Related

- [xiaozhi-esp32-server-java](https://github.com/zhangyujian111/xiaozhi-esp32-server-java) —— Java 原版（协议兼容参照）
- [ykt-aisaas](https://github.com/zhangyujian111/ykt-aisaas) —— AI 后端（ASR / LLM / TTS / RAG / RBAC）
- [xiaozhi-esp32](https://github.com/78/xiaozhi-esp32) —— ESP32 firmware

---

## License

[MIT](LICENSE) © 2026 zhangyujian111
