# xiaozhi-esp32-server-go 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**目标**：构建 `xiaozhi-esp32-server-go`，替换 `xiaozhi-esp32-server-java` 的 WebSocket 协议层与对话编排核心，调用现有 `ykt-aisaas` 完成 AI 模型调用。

**架构**：Go 单进程 + Gin/gorilla-websocket，对外提供 WebSocket（设备）与 REST（OTA/内部代理），调用 aisaas HTTP 接口完成 STT/LLM/TTS。共享 MySQL 库（Go 拥有 `xiaozhi_*` 表）。

**技术栈**：Go 1.22+ · Gin · gorilla/websocket · Silero VAD · concentus（纯 Go Opus）· golang-migrate · zerolog · prometheus/client_golang · stretchr/testify

**开发工期**：17 周（1 Go 开发）+ 跨团队 aisaas 端点 4 dev-days

**设计文档**：`docs/superpowers/specs/2026-09-29-xiaozhi-esp32-server-go-design.md`

---

## 总体执行节奏

| 阶段 | 主题 | 时长 | 关键产物 | 关键依赖 |
|---|---|---|---|---|
| P0 | 项目脚手架 | 1 周 | 可运行的空服务，healthz 绿 | 无 |
| P1 | 协议层 | 2 周 | 所有消息类型 JSON round-trip | 无 |
| P2 | WebSocket + 会话 | 2 周 | 对真实设备 hello round-trip | aisaas 设备端点 |
| P3 | 音频管线 | 2 周 | VAD + Opus round-trip 单测 | Silero 模型文件 |
| P4 | aisaas 客户端 + 对话编排 | 3 周 | 端到端对话跑通 | 无 |
| P5 | REST API | 2 周 | OTA 激活 + 内部代理 | aisaas 鉴权端点 |
| P6 | 集成 + 模拟器 + 一致性 | 2 周 | L2/L3 全绿 | P1-P5 |
| P7 | HIL + 性能 | 2 周 | L4/L5 全绿 | 物理设备 |
| P8 | 切换准备 | 1 周 | 全 Go/No-Go 通过 | 所有前置 |
| P9 | 前端 5 view（后期）| 3 周 | 新 view 上线 | P8 |

P0-P4 的详细任务见本文。P5-P9 给任务纲要和验收标准。

---

# 阶段 P0：项目脚手架（1 周）

## 任务 0.1：初始化 Go 模块与目录结构

**文件**：
- 创建：`go.mod`
- 创建：`cmd/server/main.go`
- 创建：`README.md`
- 创建：`.gitignore`

**步骤 1：创建项目目录**

```bash
mkdir -p D:/zyj_workspace/toy/xiaozhi-esp32-server-go
cd D:/zyj_workspace/toy/xiaozhi-esp32-server-go
git init
```

预期：空 git 仓库

**步骤 2：初始化 Go 模块**

```bash
go mod init github.com/xiaozhi/xiaozhi-esp32-server-go
go version  # 期望 1.22+
```

预期：`go.mod` 创建，Go 版本 ≥ 1.22

**步骤 3：创建目录骨架**

```bash
mkdir -p cmd/server internal/{protocol,ws,audio/vad,audio/opus,dialogue,aisaas,api,store,config,obs} configs deploy migrations test/{conformance,integration,unit}
```

预期：所有目录创建成功

**步骤 4：编写 .gitignore**

`.gitignore`：
```
# Binaries
/bin/
*.exe
*.exe~
*.dll
*.so
*.dylib

# Test binary
*.test

# Output
*.out

# Go workspace
go.work
go.work.sum

# Env
.env
.env.local

# IDE
.vscode/
.idea/
*.swp

# Logs
*.log

# Local models (downloaded separately)
/models/*.onnx
```

**步骤 5：编写最小 main.go**

`cmd/server/main.go`：
```go
package main

import "fmt"

func main() {
    fmt.Println("xiaozhi-esp32-server-go starting...")
}
```

**步骤 6：编译验证**

```bash
go build ./cmd/server
./server  # Windows 上是 server.exe
```

预期：输出 "xiaozhi-esp32-server-go starting..."

**步骤 7：提交**

```bash
git add .
git commit -m "chore: initialize Go module and project structure"
```

---

## 任务 0.2：集成 Gin + zerolog + viper

**文件**：
- 修改：`cmd/server/main.go`
- 创建：`internal/config/config.go`
- 创建：`internal/obs/log.go`
- 创建：`configs/config.yaml`

**步骤 1：写配置加载失败测试**

`internal/config/config_test.go`：
```go
package config

import (
    "testing"
)

func TestLoad_MissingFile(t *testing.T) {
    _, err := Load("/nonexistent/config.yaml")
    if err == nil {
        t.Fatal("expected error for missing config file, got nil")
    }
}
```

**步骤 2：运行测试，确认失败**

```bash
go test ./internal/config/... -v
```

预期：FAIL（Load 函数未定义）

**步骤 3：实现配置加载**

`internal/config/config.go`：
```go
package config

import (
    "github.com/spf13/viper"
)

type Config struct {
    Server   ServerConfig   `mapstructure:"server"`
    Aisaas   AisaasConfig   `mapstructure:"aisaas"`
    VAD      VADConfig      `mapstructure:"vad"`
    Opus     OpusConfig     `mapstructure:"opus"`
    Dialogue DialogueConfig `mapstructure:"dialogue"`
    Database DatabaseConfig `mapstructure:"database"`
    Logging  LoggingConfig  `mapstructure:"logging"`
}

type ServerConfig struct {
    WebsocketAddr string `mapstructure:"websocket_addr"`
    AdminAddr     string `mapstructure:"admin_addr"`
    ReadTimeout   string `mapstructure:"read_timeout"`
    WriteTimeout  string `mapstructure:"write_timeout"`
    MaxConns      int    `mapstructure:"max_connections"`
}

type AisaasConfig struct {
    BaseURL       string `mapstructure:"base_url"`
    InternalToken string `mapstructure:"internal_token"`
    HTTPTimeout   string `mapstructure:"http_timeout"`
}

type VADConfig struct {
    ModelPath         string  `mapstructure:"model_path"`
    SpeechThreshold   float32 `mapstructure:"speech_threshold"`
    SilenceThreshold  float32 `mapstructure:"silence_threshold"`
    SilenceDurationMs int     `mapstructure:"silence_duration_ms"`
    FrameSizeSamples  int     `mapstructure:"frame_size_samples"`
}

type OpusConfig struct {
    Uplink   OpusDirection `mapstructure:"uplink"`
    Downlink OpusDirection `mapstructure:"downlink"`
}

type OpusDirection struct {
    SampleRate     int `mapstructure:"sample_rate"`
    Channels       int `mapstructure:"channels"`
    FrameDurationMs int `mapstructure:"frame_duration_ms"`
}

type DialogueConfig struct {
    PerTurnTimeoutSec    int `mapstructure:"per_turn_timeout"`
    WindowMemorySize     int `mapstructure:"window_memory_size"`
    TTFSAlertThresholdMs int `mapstructure:"ttfs_alert_threshold_ms"`
}

type DatabaseConfig struct {
    DSN             string `mapstructure:"dsn"`
    MaxOpenConns    int    `mapstructure:"max_open_conns"`
    MaxIdleConns    int    `mapstructure:"max_idle_conns"`
    ConnMaxLifetime string `mapstructure:"conn_max_lifetime"`
    MigrationPath   string `mapstructure:"migration_path"`
}

type LoggingConfig struct {
    Level  string `mapstructure:"level"`
    Format string `mapstructure:"format"`
    Output string `mapstructure:"output"`
}

func Load(path string) (*Config, error) {
    v := viper.New()
    v.SetConfigFile(path)
    v.AutomaticEnv()
    v.SetEnvPrefix("XIAOZHI")

    if err := v.ReadInConfig(); err != nil {
        return nil, err
    }

    var cfg Config
    if err := v.Unmarshal(&cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

**步骤 4：运行测试确认通过**

```bash
go test ./internal/config/... -v
```

预期：PASS

**步骤 5：写配置内容**

`configs/config.yaml`：
```yaml
server:
  websocket_addr: ":8080"
  admin_addr: ":8081"
  read_timeout: "30s"
  write_timeout: "30s"
  max_connections: 200

aisaas:
  base_url: "http://ykt-aisaas:8190"
  internal_token: "${AISAAS_INTERNAL_TOKEN}"
  http_timeout: "30s"

vad:
  model_path: "./models/silero_vad.onnx"
  speech_threshold: 0.5
  silence_threshold: 0.3
  silence_duration_ms: 800
  frame_size_samples: 512

opus:
  uplink:
    sample_rate: 16000
    channels: 1
    frame_duration_ms: 60
  downlink:
    sample_rate: 24000
    channels: 1
    frame_duration_ms: 60

dialogue:
  per_turn_timeout: 30
  window_memory_size: 20
  ttfs_alert_threshold_ms: 3000

database:
  dsn: "${DB_DSN}"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: "30m"
  migration_path: "./migrations"

logging:
  level: info
  format: json
  output: stdout
```

**步骤 6：实现日志初始化**

`internal/obs/log.go`：
```go
package obs

import (
    "os"
    "github.com/rs/zerolog"
)

func InitLogger(level, format string) zerolog.Logger {
    zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
    lvl, err := zerolog.ParseLevel(level)
    if err != nil {
        lvl = zerolog.InfoLevel
    }
    zerolog.SetGlobalLevel(lvl)

    if format == "console" {
        return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
    }
    return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
```

**步骤 7：更新 main.go**

`cmd/server/main.go`：
```go
package main

import (
    "log"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
)

func main() {
    cfg, err := config.Load("./configs/config.yaml")
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }
    logger := obs.InitLogger(cfg.Logging.Level, cfg.Logging.Format)
    logger.Info().Msg("xiaozhi-esp32-server-go started")
}
```

**步骤 8：构建并运行验证**

```bash
go mod tidy
go build ./cmd/server
./server
```

预期：输出 JSON 日志 "xiaozhi-esp32-server-go started"

**步骤 9：提交**

```bash
git add .
git commit -m "feat(p0): integrate config loading and structured logging"
```

---

## 任务 0.3：健康检查端点 + Prometheus 指标

**文件**：
- 创建：`internal/obs/metrics.go`
- 修改：`cmd/server/main.go`

**步骤 1：写 healthz 失败测试**

`internal/api/health_test.go`（先创建空文件占位，步骤 2 实现）

`cmd/server/main_test.go`：
```go
package main

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthz(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()

    handler := setupTestRouter()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rec.Code)
    }
}

func setupTestRouter() http.Handler {
    return setupRouter() // 从 main.go 导出
}
```

**步骤 2：实现 healthz 和指标端点**

新建 `internal/api/router.go`：
```go
package api

import (
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func SetupRouter() *gin.Engine {
    r := gin.New()
    r.Use(gin.Recovery())

    r.GET("/healthz", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "ok"})
    })

    r.GET("/readyz", func(c *gin.Context) {
        c.JSON(200, gin.H{"status": "ready"})
    })

    r.GET("/metrics", gin.WrapH(promhttp.Handler()))

    return r
}
```

**步骤 3：导出 setupRouter 给 main.go 和测试**

`internal/api/router.go` 末尾加：
```go
// SetupRouter 别名供 main 包使用
```

新建 `internal/api/export_test.go`：
```go
package api

// SetupRouterForTest 暴露给外部测试包
func SetupRouterForTest() *gin.Engine { return SetupRouter() }
```

更新 `cmd/server/main.go`：
```go
package main

import (
    "log"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/api"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/config"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/obs"
)

func setupRouter() http.Handler { return api.SetupRouter() }

func main() {
    cfg, err := config.Load("./configs/config.yaml")
    if err != nil {
        log.Fatalf("failed to load config: %v", err)
    }
    logger := obs.InitLogger(cfg.Logging.Level, cfg.Logging.Format)
    logger.Info().Msg("xiaozhi-esp32-server-go started")

    r := api.SetupRouter()
    if err := r.Run(":8081"); err != nil {
        log.Fatalf("server failed: %v", err)
    }
}
```

**步骤 4：运行测试**

```bash
go test ./... -v
```

预期：所有测试 PASS

**步骤 5：手动验证**

```bash
./server &
sleep 2
curl.exe http://localhost:8081/healthz
curl.exe http://localhost:8081/metrics | head -20
```

预期：`{"status":"ok"}` + Prometheus 指标

**步骤 6：提交**

```bash
git add .
git commit -m "feat(p0): add healthz, readyz, metrics endpoints"
```

---

## 任务 0.4：Dockerfile + docker-compose（开发栈）

**文件**：
- 创建：`deploy/Dockerfile`
- 创建：`deploy/docker-compose.yml`

**步骤 1：写 Dockerfile（多阶段、无 CGO）**

`deploy/Dockerfile`：
```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
COPY --from=builder /src/configs /configs
EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/server"]
```

**步骤 2：写 docker-compose（仅开发栈，Go + MySQL mock）**

`deploy/docker-compose.yml`：
```yaml
version: "3.8"
services:
  xiaozhi-server-go:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    ports:
      - "8080:8080"
      - "8081:8081"
    environment:
      AISAAS_INTERNAL_TOKEN: "${AISAAS_INTERNAL_TOKEN:-dev-token}"
      DB_DSN: "root:dev@tcp(mysql:3306)/xiaozhi?parseTime=true&charset=utf8mb4"
    volumes:
      - ../configs:/configs
    depends_on:
      - mysql
    restart: unless-stopped

  mysql:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: dev
      MYSQL_DATABASE: xiaozhi
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql

volumes:
  mysql_data:
```

**步骤 3：构建镜像验证**

```bash
docker build -f deploy/Dockerfile -t xiaozhi-server-go:test .
docker images | grep xiaozhi-server-go
```

预期：镜像构建成功

**步骤 4：提交**

```bash
git add .
git commit -m "feat(p0): add Dockerfile and docker-compose for dev"
```

---

## 任务 0.5：CI 工作流（GitHub Actions）

**文件**：
- 创建：`.github/workflows/ci.yml`

**步骤 1：写 CI 工作流**

`.github/workflows/ci.yml`：
```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - name: Build
        run: go build -v ./...
      - name: Test
        run: go test -v -race -coverprofile=coverage.out ./...
      - name: Coverage
        run: |
          go install github.com/boumenot/gocover-cobertura@latest
          gocover-cobertura < coverage.out > coverage.xml
      - uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.xml

  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"
      - uses: golangci/golangci-lint-action@v4
        with:
          version: latest
```

**步骤 2：本地验证 lint**

```bash
go vet ./...
gofmt -l .
```

预期：无输出（通过）

**步骤 3：提交**

```bash
git add .
git commit -m "feat(p0): add GitHub Actions CI workflow"
```

---

## P0 验收门禁

- [ ] `go build ./...` 通过
- [ ] `go test ./...` 全绿
- [ ] `go vet ./...` 无告警
- [ ] `./server` 启动，healthz 返回 200
- [ ] `/metrics` 返回 Prometheus 格式
- [ ] `docker build` 成功
- [ ] CI 在 GitHub 上跑通

---

# 阶段 P1：协议层（2 周）

## 任务 1.1：Hello 消息编解码

**文件**：
- 创建：`internal/protocol/hello.go`
- 创建：`internal/protocol/hello_test.go`
- 创建：`internal/protocol/codec.go`

**步骤 1：写 Hello 编解码失败测试**

`internal/protocol/hello_test.go`：
```go
package protocol

import (
    "encoding/json"
    "testing"
)

func TestHelloMessage_Unmarshal_DeviceToServer(t *testing.T) {
    raw := `{"type":"hello","version":1,"features":{"mcp":true,"aec":true},"transport":"websocket","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}}`
    var msg HelloMessage
    if err := json.Unmarshal([]byte(raw), &msg); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }
    if msg.Type != "hello" {
        t.Errorf("expected type=hello, got %s", msg.Type)
    }
    if msg.Version != 1 {
        t.Errorf("expected version=1, got %d", msg.Version)
    }
    if !msg.Features.MCP {
        t.Error("expected features.mcp=true")
    }
    if msg.AudioParams.SampleRate != 16000 {
        t.Errorf("expected sample_rate=16000, got %d", msg.AudioParams.SampleRate)
    }
}

func TestHelloMessage_Marshal_ServerToDevice(t *testing.T) {
    msg := NewServerHello("test-session-uuid", AudioParams{
        Format: "opus", SampleRate: 24000, Channels: 1, FrameDuration: 60,
    })
    b, err := json.Marshal(msg)
    if err != nil {
        t.Fatalf("marshal failed: %v", err)
    }
    expected := `{"type":"hello","transport":"websocket","session_id":"test-session-uuid","audio_params":{"format":"opus","sample_rate":24000,"channels":1,"frame_duration":60}}`
    if string(b) != expected {
        t.Errorf("expected %s, got %s", expected, string(b))
    }
}
```

**步骤 2：运行测试确认失败**

```bash
go test ./internal/protocol/... -v
```

预期：FAIL（类型未定义）

**步骤 3：实现 codec 辅助和 Hello 类型**

`internal/protocol/codec.go`：
```go
package protocol

import "encoding/json"

// MustMarshal 序列化失败时 panic
func MustMarshal(v interface{}) []byte {
    b, err := json.Marshal(v)
    if err != nil {
        panic(err)
    }
    return b
}

// MessageType 表示协议消息类型
type MessageType string

const (
    TypeHello  MessageType = "hello"
    TypeListen MessageType = "listen"
    TypeAbort  MessageType = "abort"
    TypeMCP    MessageType = "mcp"
    TypeAck    MessageType = "ack"
    TypeTTS    MessageType = "tts"
    TypeSTT    MessageType = "stt"
    TypeLLM    MessageType = "llm"
    TypeIoT    MessageType = "iot"
    TypeSystem MessageType = "system"
    TypeAlert  MessageType = "alert"
)
```

`internal/protocol/hello.go`：
```go
package protocol

// HelloFeatures 设备能力声明
type HelloFeatures struct {
    MCP bool `json:"mcp"`
    AEC bool `json:"aec"`
}

// AudioParams 音频参数
type AudioParams struct {
    Format        string `json:"format"`
    SampleRate    int    `json:"sample_rate"`
    Channels      int    `json:"channels"`
    FrameDuration int    `json:"frame_duration"`
}

// HelloMessage 双向 hello 消息（设备→服务端含 version/features，服务端→设备含 session_id）
type HelloMessage struct {
    Type        MessageType   `json:"type"`
    Version     int           `json:"version,omitempty"`
    Features    HelloFeatures `json:"features,omitempty"`
    Transport   string        `json:"transport"`
    SessionID   string        `json:"session_id,omitempty"`
    AudioParams AudioParams   `json:"audio_params"`
}

// NewServerHello 构造服务端响应
func NewServerHello(sessionID string, audio AudioParams) HelloMessage {
    return HelloMessage{
        Type:        TypeHello,
        Transport:   "websocket",
        SessionID:   sessionID,
        AudioParams: audio,
    }
}
```

**步骤 4：运行测试**

```bash
go test ./internal/protocol/... -v
```

预期：PASS

**步骤 5：提交**

```bash
git add .
git commit -m "feat(protocol): implement hello message codec"
```

---

## 任务 1.2：Listen / Abort / Ack 消息

**文件**：
- 创建：`internal/protocol/listen.go` + `_test.go`
- 创建：`internal/protocol/abort.go` + `_test.go`
- 创建：`internal/protocol/ack.go` + `_test.go`

（每个文件遵循相同 TDD 模式：测试 → 实现 → 通过 → 提交。完整代码见附录 A。）

**关键定义**：

```go
// Listen
type ListenState string
const (
    ListenStart  ListenState = "start"
    ListenStop   ListenState = "stop"
    ListenDetect ListenState = "detect"
)

type ListenMode string
const (
    ListenModeAuto     ListenMode = "auto"
    ListenModeManual   ListenMode = "manual"
    ListenModeRealtime ListenMode = "realtime"
)

type ListenMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    State     ListenState `json:"state"`
    Mode      ListenMode  `json:"mode,omitempty"`
    Text      string      `json:"text,omitempty"`
}

// Abort
type AbortMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Reason    string      `json:"reason"`
}

// Ack
type AckMessage struct {
    Type     MessageType `json:"type"`
    SessionID string     `json:"session_id"`
    MsgType  string     `json:"msgType"`
    MsgID    string     `json:"msgId"`
    Status   string     `json:"status"`
    Error    string     `json:"error,omitempty"`
}
```

---

## 任务 1.3：TTS / STT / LLM 消息（服务端→设备）

**文件**：
- 创建：`internal/protocol/tts.go` + `_test.go`
- 创建：`internal/protocol/stt.go` + `_test.go`
- 创建：`internal/protocol/llm.go` + `_test.go`

**关键定义**：

```go
// TTS
type TTSState string
const (
    TTSStart         TTSState = "start"
    TTSStop          TTSState = "stop"
    TTSSentenceStart TTSState = "sentence_start"
    TTSSentenceEnd   TTSState = "sentence_end"
)

type TTSMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    State     TTSState    `json:"state"`
    Text      string      `json:"text,omitempty"`
    MsgID     string      `json:"msg_id,omitempty"`
}

// STT
type STTMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Text      string      `json:"text"`
}

// LLM (含 emotion / action)
type LLMMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Emotion   string      `json:"emotion,omitempty"`
    Action    string      `json:"action,omitempty"`
    Text      string      `json:"text"`
    MsgID     string      `json:"msg_id,omitempty"`
}
```

---

## 任务 1.4：MCP / IoT / Alert / System 消息

**文件**：
- 创建：`internal/protocol/mcp.go` + `_test.go`
- 创建：`internal/protocol/iot.go` + `_test.go`
- 创建：`internal/protocol/alert.go` + `_test.go`
- 创建：`internal/protocol/system.go` + `_test.go`

**关键定义**：

```go
// MCP (JSON-RPC 2.0 包装)
type MCPMessage struct {
    Type      MessageType    `json:"type"`
    SessionID string         `json:"session_id"`
    Payload   MCPJSONRPC     `json:"payload"`
}

type MCPJSONRPC struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      int             `json:"id,omitempty"`
    Method  string          `json:"method,omitempty"`
    Params  json.RawMessage `json:"params,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *MCPError       `json:"error,omitempty"`
}

type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

// IoT
type IoTMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Update    bool        `json:"update,omitempty"`
    States    []IoTState  `json:"states,omitempty"`
}

// Alert
type AlertMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Status    string      `json:"status"`
    Message   string      `json:"message"`
    Emotion   string      `json:"emotion,omitempty"`
}

// System
type SystemMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Command   string      `json:"command"`
}
```

---

## 任务 1.5：二进制协议 v1 / v2 / v3

**文件**：
- 创建：`internal/protocol/binary.go` + `_test.go`

**关键定义**（参考 `xiaozhi-esp32-main/main/protocols/protocol.h`）：

```go
// BinaryV1: 原始 Opus 帧（无 header）
// BinaryV2: 带时间戳（用于服务端 AEC）
type BinaryV2Header struct {
    Version     uint16 // 2
    Type        uint16 // 0=Opus, 1=JSON
    Reserved    uint32
    Timestamp   uint32 // ms
    PayloadSize uint32
}
// header 总 16 字节；payload 在 header 之后

// BinaryV3: 轻量级 header
type BinaryV3Header struct {
    Type        uint8  // 0=Opus, 1=JSON
    Reserved    uint8
    PayloadSize uint16
}
// header 总 4 字节

// ParseBinaryFrame 根据握手时的 protocol-version 选择解析方式
func ParseBinaryFrame(data []byte, version int) (payload []byte, frameType uint8, timestamp uint32, err error) {
    switch version {
    case 1:
        return data, 0, 0, nil // v1: 裸 Opus
    case 2:
        if len(data) < 16 {
            return nil, 0, 0, ErrShortBinaryFrame
        }
        h := BinaryV2Header{
            Version:     binary.LittleEndian.Uint16(data[0:2]),
            Type:        binary.LittleEndian.Uint16(data[2:4]),
            Reserved:    binary.LittleEndian.Uint32(data[4:8]),
            Timestamp:   binary.LittleEndian.Uint32(data[8:12]),
            PayloadSize: binary.LittleEndian.Uint32(data[12:16]),
        }
        if int(h.PayloadSize) > len(data)-16 {
            return nil, 0, 0, ErrInvalidPayloadSize
        }
        return data[16 : 16+h.PayloadSize], uint8(h.Type), h.Timestamp, nil
    case 3:
        if len(data) < 4 {
            return nil, 0, 0, ErrShortBinaryFrame
        }
        h := BinaryV3Header{
            Type:        data[0],
            Reserved:    data[1],
            PayloadSize: binary.LittleEndian.Uint16(data[2:4]),
        }
        return data[4 : 4+h.PayloadSize], h.Type, 0, nil
    default:
        return nil, 0, 0, ErrUnsupportedProtocolVersion
    }
}
```

---

## 任务 1.6：协议错误与关闭码

**文件**：
- 创建：`internal/protocol/errors.go`

```go
package protocol

import "errors"

var (
    ErrShortBinaryFrame       = errors.New("binary frame too short")
    ErrInvalidPayloadSize     = errors.New("invalid payload size in binary frame")
    ErrUnsupportedProtocolVersion = errors.New("unsupported protocol version")
    ErrMissingDeviceID        = errors.New("missing or empty device-id header")
    ErrInvalidToken           = errors.New("invalid bearer token")
    ErrMalformedJSON          = errors.New("malformed JSON message")
    ErrUnknownMessageType     = errors.New("unknown message type")
)

// WebSocket 关闭码
const (
    CloseNormalClosure           = 1000
    CloseGoingAway               = 1001
    CloseUnsupportedData         = 1003
    CloseInvalidFramePayloadData = 1007
    ClosePolicyViolation         = 1008
    CloseInternalServerErr       = 1011
)
```

---

## P1 验收门禁

- [ ] 所有消息类型 JSON 编解码 round-trip 通过
- [ ] 二进制协议 v1/v2/v3 解析单测全绿
- [ ] `go test ./internal/protocol/... -v` 全绿，覆盖率 ≥ 90%
- [ ] 无 race condition（`go test -race`）

---

# 阶段 P2：WebSocket + 会话（2 周）

## 任务 2.1：ChatSession 状态机

**文件**：
- 创建：`internal/ws/session.go` + `_test.go`

**步骤 1：写状态机转换测试**

`internal/ws/session_test.go`：
```go
package ws

import "testing"

func TestChatSession_StateTransitions(t *testing.T) {
    s := NewChatSession("test-device", "test-session")

    if s.State() != StateIdle {
        t.Fatalf("expected initial state=idle, got %s", s.State())
    }

    if err := s.TransitionTo(StateListening); err != nil {
        t.Fatalf("transition idle→listening failed: %v", err)
    }
    if err := s.TransitionTo(StateThinking); err != nil {
        t.Fatalf("transition listening→thinking failed: %v", err)
    }
    // 不允许 idle → speaking（应失败）
    if err := s.TransitionTo(StateSpeaking); err == nil {
        t.Fatal("expected error for idle→speaking, got nil")
    }
}
```

**步骤 2：实现状态机**

`internal/ws/session.go`：
```go
package ws

import (
    "fmt"
    "sync"
    "time"
)

type SessionState string

const (
    StateIdle      SessionState = "idle"
    StateListening SessionState = "listening"
    StateThinking  SessionState = "thinking"
    StateSpeaking  SessionState = "speaking"
)

// 合法的状态转换
var validTransitions = map[SessionState][]SessionState{
    StateIdle:      {StateListening},
    StateListening: {StateThinking, StateIdle},
    StateThinking:  {StateSpeaking, StateIdle},
    StateSpeaking:  {StateListening, StateIdle},
}

type ChatSession struct {
    mu          sync.RWMutex
    id          string
    deviceID    string
    state       SessionState
    createdAt   time.Time
    lastActiveAt time.Time
    audioBuf    *AudioRingBuffer
}

func NewChatSession(deviceID, sessionID string) *ChatSession {
    now := time.Now()
    return &ChatSession{
        id:           sessionID,
        deviceID:     deviceID,
        state:        StateIdle,
        createdAt:    now,
        lastActiveAt:  now,
        audioBuf:     NewAudioRingBuffer(16000, 60*5), // 5s @ 16kHz
    }
}

func (s *ChatSession) ID() string        { return s.id }
func (s *ChatSession) DeviceID() string  { return s.deviceID }
func (s *ChatSession) State() SessionState {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.state
}

func (s *ChatSession) TransitionTo(target SessionState) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    for _, allowed := range validTransitions[s.state] {
        if allowed == target {
            s.state = target
            s.lastActiveAt = time.Now()
            return nil
        }
    }
    return fmt.Errorf("invalid transition %s → %s", s.state, target)
}

func (s *ChatSession) Touch() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.lastActiveAt = time.Now()
}

func (s *ChatSession) AudioBuffer() *AudioRingBuffer { return s.audioBuf }
func (s *ChatSession) LastActiveAt() time.Time {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.lastActiveAt
}
```

**步骤 3：实现音频环形缓冲（最小可用版）**

`internal/ws/audio_buffer.go`：
```go
package ws

import "sync"

type AudioRingBuffer struct {
    mu   sync.Mutex
    data []byte
    cap  int
}

func NewAudioRingBuffer(sampleRate int, maxSeconds int) *AudioRingBuffer {
    return &AudioRingBuffer{
        data: make([]byte, 0, sampleRate*maxSeconds*2), // 16-bit
        cap:  sampleRate * maxSeconds * 2,
    }
}

func (b *AudioRingBuffer) Write(p []byte) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.data = append(b.data, p...)
    if len(b.data) > b.cap {
        b.data = b.data[len(b.data)-b.cap:]
    }
}

func (b *AudioRingBuffer) Drain() []byte {
    b.mu.Lock()
    defer b.mu.Unlock()
    out := make([]byte, len(b.data))
    copy(out, b.data)
    b.data = b.data[:0]
    return out
}

func (b *AudioRingBuffer) Len() int {
    b.mu.Lock()
    defer b.mu.Unlock()
    return len(b.data)
}
```

**步骤 4：测试通过**

```bash
go test ./internal/ws/... -v
```

**步骤 5：提交**

```bash
git commit -am "feat(ws): ChatSession state machine + audio ring buffer"
```

---

## 任务 2.2：SessionManager

**文件**：
- 创建：`internal/ws/session_manager.go` + `_test.go`

```go
package ws

import (
    "fmt"
    "sync"
)

type SessionManager struct {
    mu       sync.RWMutex
    sessions map[string]*ChatSession // session_id → session
    byDevice map[string]string       // device_id → session_id
}

func NewSessionManager() *SessionManager {
    return &SessionManager{
        sessions: make(map[string]*ChatSession),
        byDevice: make(map[string]string),
    }
}

func (m *SessionManager) Register(s *ChatSession) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    if existing, ok := m.byDevice[s.DeviceID()]; ok {
        return fmt.Errorf("device %s already has session %s", s.DeviceID(), existing)
    }
    m.sessions[s.ID()] = s
    m.byDevice[s.DeviceID()] = s.ID()
    return nil
}

func (m *SessionManager) Get(sessionID string) (*ChatSession, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    s, ok := m.sessions[sessionID]
    return s, ok
}

func (m *SessionManager) GetByDevice(deviceID string) (*ChatSession, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    sid, ok := m.byDevice[deviceID]
    if !ok {
        return nil, false
    }
    return m.sessions[sid], true
}

func (m *SessionManager) Remove(sessionID string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if s, ok := m.sessions[sessionID]; ok {
        delete(m.byDevice, s.DeviceID())
        delete(m.sessions, sessionID)
    }
}

func (m *SessionManager) Count() int {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return len(m.sessions)
}
```

测试：注册 / 获取 / 按设备查 / 删除 / Count / 重名冲突。

---

## 任务 2.3：WebSocket Handler - 握手与认证

**文件**：
- 创建：`internal/ws/handler.go` + `_test.go`

**步骤 1：写握手失败测试**

`internal/ws/handler_test.go`：
```go
package ws

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestWebSocketUpgrade_MissingDeviceID(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "/ws", nil)
    rec := httptest.NewRecorder()
    h := NewHandler(NewSessionManager(), nil)
    h.HandleUpgrade(rec, req)
    if rec.Code != http.StatusBadRequest {
        t.Fatalf("expected 400, got %d", rec.Code)
    }
    if !strings.Contains(rec.Body.String(), "device-id") {
        t.Errorf("expected error message to mention device-id, got %s", rec.Body.String())
    }
}
```

**步骤 2：实现 Handler**

`internal/ws/handler.go`（核心结构，详细代码见附录 B）：

```go
package ws

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "net/http"
    "github.com/gorilla/websocket"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/aisaas"
    "github.com/rs/zerolog"
)

type Handler struct {
    upgrader websocket.Upgrader
    sm       *SessionManager
    aisaas   *aisaas.Client
    log      zerolog.Logger
}

func NewHandler(sm *SessionManager, ac *aisaas.Client, log zerolog.Logger) *Handler {
    return &Handler{
        upgrader: websocket.Upgrader{
            CheckOrigin:     func(r *http.Request) bool { return true },
            ReadBufferSize:  4096,
            WriteBufferSize: 4096,
        },
        sm:     sm,
        aisaas: ac,
        log:    log,
    }
}

// HandleUpgrade 处理 WebSocket 升级请求
func (h *Handler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
    deviceID := r.Header.Get("Device-Id")
    if deviceID == "" {
        http.Error(w, "missing device-id header", http.StatusBadRequest)
        return
    }
    token := r.Header.Get("Authorization")
    if token == "" {
        http.Error(w, "missing authorization header", http.StatusUnauthorized)
        return
    }
    // 验证 token（查 aisaas 或本地缓存）
    if err := h.validateToken(deviceID, token); err != nil {
        http.Error(w, "invalid token", http.StatusUnauthorized)
        return
    }

    conn, err := h.upgrader.Upgrade(w, r, nil)
    if err != nil {
        h.log.Error().Err(err).Msg("upgrade failed")
        return
    }

    sessionID := newSessionID()
    session := NewChatSession(deviceID, sessionID)
    if err := h.sm.Register(session); err != nil {
        h.log.Error().Err(err).Msg("session register failed")
        conn.Close()
        return
    }

    go h.serveConn(conn, session)
}

// validateToken 通过 aisaas 验证 token（详见 P4 实现的 aisaas client）
func (h *Handler) validateToken(deviceID, token string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    return h.aisaas.VerifyDeviceToken(ctx, deviceID, token)
}

func newSessionID() string {
    b := make([]byte, 16)
    rand.Read(b)
    return hex.EncodeToString(b)
}
```

**步骤 3：补充实现 aisaas.VerifyDeviceToken 接口桩**

任务 P4 才有完整实现。这里先写接口和桩让 P2 测试通过。

`internal/aisaas/client.go`：
```go
package aisaas

import (
    "context"
    "errors"
)

var ErrNotImplemented = errors.New("not implemented yet")

type Client struct {
    BaseURL       string
    InternalToken string
}

func (c *Client) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
    return ErrNotImplemented
}
```

P2 期间用 mock 验证（test 中注入桩）。

**步骤 4：测试**

```bash
go test ./internal/ws/... -v
```

预期：PASS（用 mock aisaas client）

**步骤 5：提交**

```bash
git commit -am "feat(ws): WebSocket upgrade + handshake + auth"
```

---

## 任务 2.4：消息路由（hello / listen / abort / ack）

**文件**：
- 修改：`internal/ws/handler.go`

**步骤 1：实现 serveConn**

```go
func (h *Handler) serveConn(conn *websocket.Conn, session *ChatSession) {
    defer func() {
        h.sm.Remove(session.ID())
        conn.Close()
        session.TransitionTo(protocol.StateIdle)
    }()

    for {
        msgType, data, err := conn.ReadMessage()
        if err != nil {
            h.log.Info().Err(err).Str("device", session.DeviceID()).Msg("read failed")
            return
        }
        session.Touch()

        if msgType == websocket.BinaryMessage {
            // 二进制：写入音频 buffer
            session.AudioBuffer().Write(data)
            continue
        }

        // 文本：解析 JSON
        var env struct {
            Type string `json:"type"`
        }
        if err := json.Unmarshal(data, &env); err != nil {
            h.log.Warn().Err(err).Msg("malformed JSON")
            continue
        }
        switch env.Type {
        case "hello":
            h.handleHello(conn, session, data)
        case "listen":
            h.handleListen(conn, session, data)
        case "abort":
            h.handleAbort(session, data)
        case "ack":
            h.handleAck(session, data)
        case "mcp", "iot":
            // MVP：log + ack
            h.log.Info().Str("type", env.Type).Msg("received; ack-only in MVP")
            // 发送 ack（详见 D3/D4）
        default:
            h.log.Warn().Str("type", env.Type).Msg("unknown message type")
        }
    }
}
```

**步骤 2：实现 handleHello**

```go
func (h *Handler) handleHello(conn *websocket.Conn, session *ChatSession, raw []byte) {
    var msg protocol.HelloMessage
    if err := json.Unmarshal(raw, &msg); err != nil {
        h.log.Warn().Err(err).Msg("hello parse failed")
        return
    }
    // 构造响应（音频参数按服务端配置固定为 24k Opus）
    resp := protocol.NewServerHello(session.ID(), protocol.AudioParams{
        Format: "opus", SampleRate: 24000, Channels: 1, FrameDuration: 60,
    })
    if err := conn.WriteJSON(resp); err != nil {
        h.log.Error().Err(err).Msg("hello response failed")
    }
    h.log.Info().Str("device", session.DeviceID()).Msg("hello exchanged")
}
```

**步骤 3：实现 handleListen / handleAbort / handleAck**

（详细实现见附录 C，要点是状态机转换 + 日志 + ack-only 路由）

**步骤 4：用 device_sim 做集成测试**

`test/conformance/device_sim/device_sim.go`：

```go
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "net/url"
    "os"
    "time"
    "github.com/gorilla/websocket"
    "github.com/xiaozhi/xiaozhi-esp32-server-go/internal/protocol"
)

func main() {
    addr := flag.String("addr", "localhost:8080", "server address")
    deviceID := flag.String("device-id", "test-device-001", "device id")
    token := flag.String("token", "dev-token", "auth token")
    flag.Parse()

    u := url.URL{Scheme: "ws", Host: *addr, Path: "/ws"}
    h := http.Header{}
    h.Set("Device-Id", *deviceID)
    h.Set("Authorization", "Bearer "+*token)
    h.Set("Protocol-Version", "1")

    c, _, err := websocket.DefaultDialer.Dial(u.String(), h)
    if err != nil {
        fmt.Fprintln(os.Stderr, "dial failed:", err)
        os.Exit(1)
    }
    defer c.Close()

    // 发送 hello
    hello := protocol.HelloMessage{
        Type: protocol.TypeHello, Version: 1,
        Features: protocol.HelloFeatures{MCP: true, AEC: true},
        Transport: "websocket",
        AudioParams: protocol.AudioParams{Format: "opus", SampleRate: 16000, Channels: 1, FrameDuration: 60},
    }
    if err := c.WriteJSON(hello); err != nil {
        fmt.Fprintln(os.Stderr, "hello failed:", err)
        os.Exit(1)
    }
    fmt.Println("hello sent, waiting for response...")

    var resp protocol.HelloMessage
    if err := c.ReadJSON(&resp); err != nil {
        fmt.Fprintln(os.Stderr, "read failed:", err)
        os.Exit(1)
    }
    fmt.Printf("hello response: session_id=%s sample_rate=%d\n", resp.SessionID, resp.AudioParams.SampleRate)

    // 发送 listen.start
    listen := protocol.ListenMessage{Type: protocol.TypeListen, SessionID: resp.SessionID, State: protocol.ListenStart, Mode: protocol.ListenModeAuto}
    c.WriteJSON(listen)
    fmt.Println("listen.start sent")

    time.Sleep(2 * time.Second)
    fmt.Println("done")
}
```

**步骤 5：手动跑端到端**

```bash
# 终端 1
./server

# 终端 2
go run test/conformance/device_sim/device_sim.go -device-id test-001
```

预期：hello round-trip 成功，session_id 返回，listen.start 接受

**步骤 6：提交**

```bash
git commit -am "feat(ws): message routing for hello/listen/abort/ack"
```

---

## P2 验收门禁

- [ ] WebSocket Upgrade 通过 device-id + token 校验
- [ ] Hello round-trip 对真实 C++ 设备或 device_sim 通过
- [ ] Listen / Abort / Ack 处理不崩
- [ ] SessionManager 注册/查找/删除正确
- [ ] `go test ./internal/ws/... -v -race` 全绿

---

# 阶段 P3：音频管线（2 周）

## 任务 3.1：Opus 解码（16kHz 上行）

**文件**：
- 创建：`internal/audio/opus/decoder.go` + `_test.go`

**步骤 1：依赖**

```bash
go get github.com/lostromb/concentus
```

**步骤 2：写解码 round-trip 测试**

```go
func TestOpusDecodeRoundTrip(t *testing.T) {
    // 编码一段 1kHz 正弦波 60ms @ 16kHz
    duration := 60 * time.Millisecond
    sampleRate := 16000
    samples := int(sampleRate * int(duration) / 1000)
    pcm := make([]int16, samples)
    for i := range pcm {
        phase := float64(i) * 2 * math.Pi * 1000 / float64(sampleRate)
        pcm[i] = int16(0.5 * 32767 * math.Sin(phase))
    }

    enc, _ := opus.NewEncoder(sampleRate, 1, opus.ApplicationAudio)
    dec, _ := opus.NewDecoder(sampleRate, 1)
    frameSize := samples // 60ms @ 16kHz = 960

    encoded := make([]byte, 4000)
    n, err := enc.Encode(pcm, encoded, frameSize)
    require.NoError(t, err)

    decoded := make([]int16, frameSize)
    _, err = dec.Decode(encoded[:n], decoded, frameSize, false)
    require.NoError(t, err)

    // 检查解码后的 PCM 不是全零且能量大致匹配
    var energy int64
    for _, s := range decoded {
        energy += int64(s) * int64(s)
    }
    require.Greater(t, energy, int64(1000000), "decoded signal should have energy")
}
```

**步骤 3：实现 decoder 包装**

```go
package opus

import (
    "github.com/lostromb/concentus"
)

type Decoder struct {
    dec     *concentus.OpusDecoder
    channels int
}

func NewDecoder(sampleRate, channels int) (*Decoder, error) {
    d, err := concentus.NewOpusDecoder(sampleRate, channels)
    if err != nil {
        return nil, err
    }
    return &Decoder{dec: d, channels: channels}, nil
}

func (d *Decoder) Decode(input []byte, frameSize int, fec bool) ([]int16, error) {
    out := make([]int16, frameSize*d.channels)
    n, err := d.dec.Decode(input, out, frameSize, fec)
    if err != nil {
        return nil, err
    }
    return out[:n*d.channels], nil
}
```

**步骤 4：测试**

```bash
go test ./internal/audio/opus/... -v
```

**步骤 5：提交**

```bash
git commit -am "feat(audio): Opus decoder wrapper"
```

---

## 任务 3.2：Opus 编码（24kHz 下行）

**文件**：
- 创建：`internal/audio/opus/encoder.go` + `_test.go`

类似解码器接口，提供 `Encode(pcm []int16) ([]byte, error)`。

帧大小：24kHz × 60ms / 1000 = 1440 samples。

**附加任务**：支持从 PCM 24k 输入编码为 Opus 24k 输出。也支持从任意采样率 PCM 先重采样到 24k。

---

## 任务 3.3：PCM ↔ WAV 转换

**文件**：
- 创建：`internal/audio/pcm.go` + `_test.go`

```go
// PCMToWAV 添加 44 字节 WAV header
func PCMToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
    header := make([]byte, 44)
    // RIFF
    copy(header[0:4], "RIFF")
    binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
    copy(header[8:12], "WAVE")
    // fmt chunk
    copy(header[12:16], "fmt ")
    binary.LittleEndian.PutUint32(header[16:20], 16)
    binary.LittleEndian.PutUint16(header[20:22], 1) // PCM
    binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
    binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
    binary.LittleEndian.PutUint32(header[28:32], uint32(sampleRate*channels*bitsPerSample/8))
    binary.LittleEndian.PutUint16(header[32:34], uint16(channels*bitsPerSample/8))
    binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))
    // data chunk
    copy(header[36:40], "data")
    binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))

    out := make([]byte, 0, 44+len(pcm))
    out = append(out, header...)
    out = append(out, pcm...)
    return out
}
```

---

## 任务 3.4：Silero VAD 集成

**文件**：
- 创建：`internal/audio/vad/silero.go` + `_test.go`

**步骤 1：下载 ONNX 模型**

```bash
mkdir -p models
curl -L -o models/silero_vad.onnx https://github.com/snakers4/silero-vad/raw/master/src/silero_vad/data/silero_vad.onnx
```

模型 ~1.5 MB。

**步骤 2：依赖**

```bash
go get github.com/yalue/onnxruntime_go
```

需要在系统装 ONNX Runtime 共享库（`libonnxruntime.so` / `onnxruntime.dll`）。

**步骤 3：实现 VAD**

```go
package vad

import (
    ort "github.com/yalue/onnxruntime_go"
)

type SileroVAD struct {
    session *ort.AdvancedSession
    h       []float32 // GRU hidden state
    c       []float32 // GRU cell state
}

const (
    SampleRate    = 16000
    WindowSamples = 512 // 32ms
)

func NewSileroVAD(modelPath string, threshold float32) (*SileroVAD, error) {
    // 初始化 ONNX Runtime（全局一次）
    ort.SetSharedLibraryPath(getOnnxLibraryPath())
    if err := ort.InitializeEnvironment(); err != nil {
        return nil, err
    }
    session, err := ort.NewAdvancedSession(modelPath,
        []string{"input", "h", "c"}, []string{"output", "hn", "cn"},
        []ort.TensorShape{{1, WindowSamples}, {2, 1, 64}, {2, 1, 64}},
        []ort.TensorShape{{1}, {2, 1, 64}, {2, 1, 64}})
    if err != nil {
        return nil, err
    }
    return &SileroVAD{
        session: session,
        h:       make([]float32, 2*64),
        c:       make([]float32, 2*64),
    }, nil
}

func (v *SileroVAD) Process(pcm []int16) (float32, error) {
    if len(pcm) != WindowSamples {
        return 0, fmt.Errorf("expected %d samples, got %d", WindowSamples, len(pcm))
    }
    input := make([]float32, WindowSamples)
    for i, s := range pcm {
        input[i] = float32(s) / 32768.0
    }
    output := make([]float32, 1)
    err := v.session.Run(
        []ort.AnyTensor{
            ort.NewTensor([]int64{1, WindowSamples}, input),
            ort.NewTensor([]int64{2, 1, 64}, v.h),
            ort.NewTensor([]int64{2, 1, 64}, v.c),
        },
        []ort.AnyTensor{
            ort.NewTensor([]int64{1}, output),
            ort.NewTensor([]int64{2, 1, 64}, v.h),
            ort.NewTensor([]int64{2, 1, 64}, v.c),
        })
    return output[0], err
}

func getOnnxLibraryPath() string {
    // Windows / Linux / macOS 检测
    // ...
}
```

**步骤 4：VAD 状态机**

`internal/audio/vad/vad_service.go`：
```go
package vad

type Status int

const (
    Silence Status = iota
    SpeechStart
    SpeechContinue
    SpeechEnd
    Error
)

type Service struct {
    vad          *SileroVAD
    speechTh     float32
    silenceTh    float32
    silenceMs    int
    inSpeech     bool
    silenceCount int
    silenceFrames int // 16kHz 下 32ms/帧 → 800ms = 25 帧
}

func NewService(v *SileroVAD, speechTh, silenceTh float32, silenceMs int) *Service {
    return &Service{
        vad:          v,
        speechTh:     speechTh,
        silenceTh:    silenceTh,
        silenceMs:    silenceMs,
        silenceFrames: silenceMs / 32,
    }
}

func (s *Service) Feed(pcm []int16) Status {
    score, err := s.vad.Process(pcm)
    if err != nil {
        return Error
    }

    if !s.inSpeech {
        if score > s.speechTh {
            s.inSpeech = true
            s.silenceCount = 0
            return SpeechStart
        }
        return Silence
    }

    // inSpeech
    if score < s.silenceTh {
        s.silenceCount++
        if s.silenceCount >= s.silenceFrames {
            s.inSpeech = false
            s.silenceCount = 0
            return SpeechEnd
        }
    } else {
        s.silenceCount = 0
    }
    return SpeechContinue
}
```

**步骤 5：测试**

用预录音频文件（fixtures/silence_1s.wav + fixtures/speech_1s.wav）做单测。

---

## 任务 3.5：音频管线集成到会话

修改 `internal/ws/handler.go`，在 `serveConn` 中：
- 收到 listen.start 时启动 VAD 处理协程
- 每个二进制帧写入 session.AudioBuffer + 喂给 VAD
- VAD 返回 SPEECH_END 时触发 STT 调用（P4 实现）

---

## P3 验收门禁

- [ ] Opus 编解码 round-trip 单测全绿
- [ ] VAD 用预录样本能区分静音/语音
- [ ] WAV header 正确生成
- [ ] `go test ./internal/audio/... -v -race` 全绿

---

# 阶段 P4：aisaas 客户端 + 对话编排（3 周）

## 任务 4.1：aisaas Client 基类

**文件**：
- 创建：`internal/aisaas/client.go`

```go
package aisaas

import (
    "context"
    "fmt"
    "net/http"
    "time"
    "github.com/go-resty/resty/v2"
)

type Client struct {
    http      *resty.Client
    baseURL   string
    token     string
}

func NewClient(baseURL, internalToken string) *Client {
    c := resty.New().
        SetTimeout(30 * time.Second).
        SetHeader("X-Internal-Token", internalToken)
    return &Client{http: c, baseURL: baseURL, token: internalToken}
}

// DeviceInfo 是 aisaas 返回的设备配置
type DeviceInfo struct {
    DeviceID    string `json:"deviceId"`
    UserID      int64  `json:"userId"`
    RoleID      int64  `json:"roleId"`
    TenantState string `json:"tenantState"`
    LLMConfigID int    `json:"llmConfigId"`
    TTSConfigID int    `json:"ttsConfigId"`
    STTConfigID int    `json:"sttConfigId"`
    VoiceName   string `json:"voiceName"`
}

// GetDevice 查询/自动创建设备，返回配置
func (c *Client) GetDevice(ctx context.Context, deviceID string) (*DeviceInfo, error) {
    var info DeviceInfo
    resp, err := c.http.R().
        SetContext(ctx).
        SetHeader("X-Device-Id", deviceID).
        SetResult(&info).
        Get(c.baseURL + "/internal/api/v1/devices/" + deviceID)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode() != http.StatusOK {
        return nil, fmt.Errorf("get device failed: status=%d body=%s", resp.StatusCode(), resp.String())
    }
    return &info, nil
}

// VerifyDeviceToken 验证 token 是否属于 device
func (c *Client) VerifyDeviceToken(ctx context.Context, deviceID, token string) error {
    info, err := c.GetDevice(ctx, deviceID)
    if err != nil {
        return err
    }
    // 简化：MVP 阶段任何非空 token 都接受（aisaas 在握手时已验证）
    // 真正实现需要 aisaas 暴露 /internal/api/v1/devices/{id}/verify-token
    if token == "" {
        return fmt.Errorf("empty token")
    }
    _ = info
    return nil
}
```

**注意**：P2 期间 `VerifyDeviceToken` 是桩，这里实现完整版本。

---

## 任务 4.2：LLM SSE 流式客户端

**文件**：
- 创建：`internal/aisaas/llm.go` + `_test.go`

**步骤 1：写解析器测试**

```go
func TestLLMStreamParser(t *testing.T) {
    sseData := `data: {"choices":[{"delta":{"content":"今天"}}]}

data: {"choices":[{"delta":{"content":"是"}}]}

data: [DONE]

`
    var tokens []string
    err := ParseLLMStream(strings.NewReader(sseData), func(token string) error {
        tokens = append(tokens, token)
        return nil
    })
    require.NoError(t, err)
    require.Equal(t, []string{"今天", "是"}, tokens)
}
```

**步骤 2：实现解析器**

```go
func ParseLLMStream(r io.Reader, onToken func(string) error) error {
    scanner := bufio.NewScanner(r)
    scanner.Buffer(make([]byte, 64*1024), 1024*1024)
    for scanner.Scan() {
        line := scanner.Text()
        if !strings.HasPrefix(line, "data: ") {
            continue
        }
        payload := strings.TrimPrefix(line, "data: ")
        if payload == "[DONE]" {
            return nil
        }
        var chunk struct {
            Choices []struct {
                Delta struct {
                    Content string `json:"content"`
                } `json:"delta"`
            } `json:"choices"`
        }
        if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
            continue // 跳过解析失败的 chunk（可能是其他事件）
        }
        for _, c := range chunk.Choices {
            if c.Delta.Content != "" {
                if err := onToken(c.Delta.Content); err != nil {
                    return err
                }
            }
        }
    }
    return scanner.Err()
}
```

**步骤 3：实现 Chat 方法**

```go
func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage) (io.ReadCloser, error) {
    req := ChatRequest{Model: model, Messages: messages, Stream: true}
    resp, err := c.http.R().
        SetContext(ctx).
        SetHeader("Content-Type", "application/json").
        SetBody(req).
        SetDoNotParseResponse(true).
        Post(c.baseURL + "/v1/chat/completions")
    if err != nil {
        return nil, err
    }
    if resp.StatusCode() != http.StatusOK {
        resp.Body.Close()
        return nil, fmt.Errorf("llm call failed: status=%d", resp.StatusCode())
    }
    return resp.Body, nil
}
```

---

## 任务 4.3：TTS 流式客户端

类似 LLM，但返回 `io.ReadCloser`（chunked 音频）。

```go
func (c *Client) TTS(ctx context.Context, model, text string) (io.ReadCloser, string, error) {
    req := TTSRequest{Model: model, Input: text, Stream: true}
    resp, err := c.http.R().
        SetContext(ctx).
        SetHeader("Content-Type", "application/json").
        SetBody(req).
        SetDoNotParseResponse(true).
        Post(c.baseURL + "/v1/audio/speech")
    if err != nil {
        return nil, "", err
    }
    if resp.StatusCode() != http.StatusOK {
        resp.Body.Close()
        return nil, "", fmt.Errorf("tts failed: status=%d", resp.StatusCode())
    }
    return resp.Body, resp.Header().Get("Content-Type"), nil
}
```

---

## 任务 4.4：STT 批量客户端

```go
type STTResult struct {
    Text    string `json:"text"`
    Emotion string `json:"emotion,omitempty"`
}

func (c *Client) STT(ctx context.Context, model string, wavData []byte) (*STTResult, error) {
    var result STTResult
    resp, err := c.http.R().
        SetContext(ctx).
        SetFileReader("file", "audio.wav", bytes.NewReader(wavData)).
        SetFormData(map[string]string{"model": model}).
        SetResult(&result).
        Post(c.baseURL + "/v1/audio/transcriptions")
    if err != nil {
        return nil, err
    }
    if resp.StatusCode() != http.StatusOK {
        return nil, fmt.Errorf("stt failed: status=%d body=%s", resp.StatusCode(), resp.String())
    }
    return &result, nil
}
```

---

## 任务 4.5：熔断器

**文件**：
- 创建：`internal/aisaas/breaker.go` + `_test.go`

经典三态熔断器：closed / open / half-open。

```go
type Breaker struct {
    mu              sync.Mutex
    state           State
    failureCount    int
    successCount    int
    failureThresh   int
    openDuration    time.Duration
    openedAt        time.Time
}

func NewBreaker(failureThresh int, openDuration time.Duration) *Breaker {
    return &Breaker{state: Closed, failureThresh: failureThresh, openDuration: openDuration}
}

func (b *Breaker) Allow() bool {
    b.mu.Lock()
    defer b.mu.Unlock()
    switch b.state {
    case Closed:
        return true
    case Open:
        if time.Since(b.openedAt) > b.openDuration {
            b.state = HalfOpen
            return true
        }
        return false
    case HalfOpen:
        return true
    }
    return false
}

func (b *Breaker) RecordSuccess() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.successCount++
    if b.state == HalfOpen {
        b.state = Closed
        b.failureCount = 0
    }
}

func (b *Breaker) RecordFailure() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.failureCount++
    if b.failureCount >= b.failureThresh {
        b.state = Open
        b.openedAt = time.Now()
    }
}
```

---

## 任务 4.6：句子切分器

**文件**：
- 创建：`internal/dialogue/sentence_splitter.go` + `_test.go`

```go
// SplitSentences 按中文/英文句末标点切分（贪婪：累积到末尾才返回完整句）
type SentenceSplitter struct {
    buffer strings.Builder
}

func (s *SentenceSplitter) Feed(token string) []string {
    s.buffer.WriteString(token)
    text := s.buffer.String()

    // 找最后一个句末标点
    delimiters := []string{"。", "！", "？", "!", "?", "；", ";", "\n"}
    lastIdx := -1
    for _, d := range delimiters {
        if i := strings.LastIndex(text, d); i > lastIdx {
            lastIdx = i
        }
    }
    if lastIdx < 0 {
        return nil
    }

    // 切到该标点为止
    out := text[:lastIdx+1]
    remainder := text[lastIdx+1:]
    s.buffer.Reset()
    s.buffer.WriteString(remainder)
    return []string{out}
}

func (s *SentenceSplitter) Flush() string {
    rest := s.buffer.String()
    s.buffer.Reset()
    return rest
}
```

---

## 任务 4.7：对话编排器（核心）

**文件**：
- 创建：`internal/dialogue/orchestrator.go` + `_test.go`

实现 §6.1 数据流的核心循环：

```go
type Orchestrator struct {
    aisaas  *aisaas.Client
    mem     *Memory
    splitter *SentenceSplitter
    opusEnc *opus.Encoder
    log     zerolog.Logger
}

func (o *Orchestrator) Run(ctx context.Context, session *ws.ChatSession, conn *websocket.Conn, device *aisaas.DeviceInfo) error {
    // 1. 取音频 buffer 内容
    pcmFrames := session.AudioBuffer().Drain()

    // 2. Opus 解码 → PCM 16k
    pcm, err := decodeAll(pcmFrames, device.SampleRate)
    if err != nil {
        return err
    }

    // 3. 打包 WAV 调 STT
    wav := audio.PCMToWAV(pcmBytes, 16000, 1, 16)
    sttResult, err := o.aisaas.STT(ctx, "stt-default", wav)
    if err != nil {
        return err
    }

    // 4. 拉窗口记忆
    history := o.mem.GetWindow(session.DeviceID(), 20)

    // 5. 构造 LLM 请求
    messages := buildMessages(history, sttResult.Text)

    // 6. 调 LLM 流式
    stream, err := o.aisaas.Chat(ctx, device.LLMConfigID, messages)
    if err != nil {
        return err
    }
    defer stream.Close()

    // 7. 发送 tts.start + sentence_start
    sendTTSStart(conn, session.ID())

    // 8. 句子切分 + 逐句 TTS + Opus 编码 + 发送
    splitter := NewSentenceSplitter()
    var fullResponse strings.Builder

    err = aisaas.ParseLLMStream(stream, func(token string) error {
        fullResponse.WriteString(token)
        for _, sent := range splitter.Feed(token) {
            if err := o.synthesizeAndSend(ctx, conn, session, sent, device); err != nil {
                return err
            }
        }
        return nil
    })
    if err != nil {
        return err
    }

    // 9. flush 最后一句
    if last := splitter.Flush(); last != "" {
        if err := o.synthesizeAndSend(ctx, conn, session, last, device); err != nil {
            return err
        }
    }

    // 10. 发送 tts.stop
    sendTTSStop(conn, session.ID())

    // 11. 入库（事务）
    return o.mem.SaveTurn(ctx, session.DeviceID(), session.ID(), sttResult.Text, fullResponse.String())
}

func (o *Orchestrator) synthesizeAndSend(ctx context.Context, conn *websocket.Conn, session *ws.ChatSession, sentence string, device *aisaas.DeviceInfo) error {
    sendTTSSentenceStart(conn, session.ID(), sentence)
    audioStream, contentType, err := o.aisaas.TTS(ctx, device.TTSConfigID, sentence)
    if err != nil {
        return err
    }
    defer audioStream.Close()

    // 读流式音频 → 重采样到 24k → Opus 编码 → 发二进制帧
    // (省略 audio format detection + resample 细节)
    return streamAudioToDevice(conn, audioStream, contentType, o.opusEnc)
}
```

---

## 任务 4.8：打断处理

**文件**：
- 创建：`internal/dialogue/interrupt.go`

实现 §6.2：

```go
type InterruptController struct {
    cancelFn context.CancelFunc
    mu       sync.Mutex
}

func (ic *InterruptController) SetCancel(cancel context.CancelFunc) {
    ic.mu.Lock()
    ic.cancelFn = cancel
    ic.mu.Unlock()
}

func (ic *InterruptController) Trigger() {
    ic.mu.Lock()
    if ic.cancelFn != nil {
        ic.cancelFn()
    }
    ic.mu.Unlock()
}
```

集成到 Orchestrator.Run：每个 aisaas 调用都用 context.WithCancel；handleListen 收到 listen.start 时调用 Trigger()。

---

## 任务 4.9：窗口记忆

**文件**：
- 创建：`internal/store/message.go`（数据库访问）
- 创建：`internal/dialogue/memory.go`

`xiaozhi_message` 表的 CRUD。

GetWindow：SELECT 最近的 N 条；按时间升序返回。

SaveTurn：在事务中插入 user + assistant 两条。

---

## P4 验收门禁

- [ ] 端到端对话在 device_sim 上跑通：说"你好" → 收到 LLM 回复 → 听到 TTS
- [ ] aisaas 单元测试覆盖所有 4 个调用路径
- [ ] 熔断器单测全绿
- [ ] 句子切分器单测全绿
- [ ] 打断测试：对话中途发送 listen.start 能停止当前回合
- [ ] `go test ./... -race` 全绿

---

# 阶段 P5 - P9：任务纲要

P5-P9 的详细任务由 writing-plans skill 在 P4 完成后基于已实现代码进一步生成。以下为纲要。

## 阶段 P5：REST API（2 周）

| 任务 | 文件 | 验收 |
|---|---|---|
| OTA 检查端点 | `internal/api/ota.go` | device_sim 调 `/api/device/ota` 返回正确 JSON |
| 设备 CRUD | `internal/api/device.go` | 列表/详情/绑定/解绑 |
| 内部代理（JWT 验证）| `internal/api/internal.go` | portal 能通过 JWT 调 Go 内部端点 |
| SSE 事件流 | `internal/api/events.go` | `/api/internal/v1/events` 推送 device_connected |
| 配置文件（v2 aisaas 鉴权）| 在 ykt-aisaas 加 `GET /internal/api/v1/auth/verify` | portal 调 Go 时 token 校验通过 |

## 阶段 P6：集成 + 模拟器 + 一致性（2 周）

| 任务 | 文件 | 验收 |
|---|---|---|
| device_sim 完善 | `test/conformance/device_sim/` | 50+ 场景脚本 |
| 协议一致性回放 | `test/conformance/fixtures/` | 100+ 真实设备消息样本 |
| 集成测试 | `test/integration/` | 50/50 脚本通过 |
| CI 集成 | `.github/workflows/conformance.yml` | 自动跑一致性测试 |

## 阶段 P7：HIL + 性能（2 周）

| 任务 | 工具 | 验收 |
|---|---|---|
| 真机准备 | 2 块 ESP32-S3 开发板 | 烧固件 |
| HIL 24h 长稳 | 手动 + 脚本 | 无崩溃 |
| 负载 1h / 50 设备 | vegeta / k6 | p99 < 3s |
| 内存泄漏检测 | pprof | RSS 增长 < 10% |
| Bug 修复 | - | 所有 critical bugs 关闭 |

## 阶段 P8：切换准备（1 周）

| 任务 | 文件 | 验收 |
|---|---|---|
| Runbook | `docs/RUNBOOK.md` | 新人能在 1h 内上岗 |
| Prometheus 告警规则 | `deploy/alerts.yml` | 关键指标告警 |
| Grafana 仪表板 | `deploy/dashboards/` | 实时可视化 |
| 回滚演练 | - | OTA URL 切换 10s 内设备重连 |
| 上线检查表 | `docs/CHECKLIST.md` | 10 项 Go/No-Go 全勾 |

## 阶段 P9：前端 5 View（3 周，P8 后并行）

| 任务 | 文件 | 验收 |
|---|---|---|
| 实时设备 View | `ykt-aisaas/web/src/views/LiveDevices.vue` | 显示连接设备 + 状态 |
| 对话历史 View | `ykt-aisaas/web/src/views/Conversations.vue` | 按设备查历史 |
| OTA 管理 View | `ykt-aisaas/web/src/views/OTAManagement.vue` | 固件版本管理 |
| 会话诊断 View | `ykt-aisaas/web/src/views/SessionDiagnostics.vue` | 实时日志流 |
| 设备上线 View | `ykt-aisaas/web/src/views/DeviceOnboarding.vue` | 二维码 + 手动录入 |
| Playwright E2E | `ykt-aisaas/web/e2e/` | 5 view 全绿 |

---

# 附录 A：Listen/Abort/Ack 完整代码

```go
// internal/protocol/listen.go
package protocol

type ListenState string
const (
    ListenStart  ListenState = "start"
    ListenStop   ListenState = "stop"
    ListenDetect ListenState = "detect"
)
type ListenMode string
const (
    ListenModeAuto     ListenMode = "auto"
    ListenModeManual   ListenMode = "manual"
    ListenModeRealtime ListenMode = "realtime"
)
type ListenMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    State     ListenState `json:"state"`
    Mode      ListenMode  `json:"mode,omitempty"`
    Text      string      `json:"text,omitempty"`
}

// internal/protocol/abort.go
type AbortMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    Reason    string      `json:"reason"`
}

// internal/protocol/ack.go
type AckMessage struct {
    Type      MessageType `json:"type"`
    SessionID string      `json:"session_id"`
    MsgType   string      `json:"msgType"`
    MsgID     string      `json:"msgId"`
    Status    string      `json:"status"`
    Error     string      `json:"error,omitempty"`
}
```

# 附录 B：WebSocket Handler 完整结构

（已包含在任务 2.3；详见该任务代码块）

# 附录 C：handleListen/handleAbort/handleAck

```go
func (h *Handler) handleListen(conn *websocket.Conn, session *ChatSession, raw []byte) {
    var msg ListenMessage
    if err := json.Unmarshal(raw, &msg); err != nil {
        return
    }
    switch msg.State {
    case ListenStart:
        session.TransitionTo(StateListening)
        // 触发新一轮对话（详见 P4 编排）
    case ListenStop:
        session.TransitionTo(StateIdle)
    case ListenDetect:
        // wake word detect 后服务端开始录音（不适用 MVP）
    }
}

func (h *Handler) handleAbort(session *ChatSession, raw []byte) {
    var msg AbortMessage
    if err := json.Unmarshal(raw, &msg); err != nil {
        return
    }
    // 通知 Orchestrator 中断当前回合
    if oc := session.Orchestrator(); oc != nil {
        oc.Interrupt(msg.Reason)
    }
}

func (h *Handler) handleAck(session *ChatSession, raw []byte) {
    var msg AckMessage
    if err := json.Unmarshal(raw, &msg); err != nil {
        return
    }
    h.log.Debug().Str("msgType", msg.MsgType).Str("status", msg.Status).Msg("ack")
}
```

---

# 执行交接

**计划完成，已保存到** `D:\zyj_workspace\toy\xiaozhi-esp32-server-go\docs\plans\2026-09-29-xiaozhi-esp32-server-go.md`

下一步两个执行选项：

**1. Subagent 驱动（本会话）** — 我为每个任务 dispatch 新的 subagent，任务间做 code review，快速迭代

**2. 并行 Session（新会话）** — 在新 worktree 中开新会话，用 executing-plans 批量执行，到检查点暂停
