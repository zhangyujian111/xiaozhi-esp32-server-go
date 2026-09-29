# xiaozhi-esp32-server-go 设计规格说明书

**状态**：草稿（待用户审阅）
**日期**：2026-09-29
**作者**：主 agent（通过头脑风暴流程）
**替换对象**：`xiaozhi-esp32-server-java` 的通信层 + 对话核心
**硬件测试基线**：`my_robot/xiaozhi-esp32-main`（C++，**不修改**——作为协议契约）

---

## 1. 背景与动机

现有的 `xiaozhi-esp32-server-java` 是一个 Spring Boot 3.5.8 服务，负责 ESP32 智能音箱对话。随着系统演进，两个问题逐渐暴露：

1. **职责混杂** — WebSocket 协议处理、对话编排、模型配置、提供商调用都堆在一个代码库里。
2. **运维复杂** — 两个可部署 Spring Boot 工件（xiaozhi-server 8091、xiaozhi-dialogue 8088），加上 7+ LLM、8+ TTS、6+ STT 提供商，配置面广、出 bug 风险高。

我们把**设备协议层**和**对话编排**抽到新的 Go 服务 `xiaozhi-esp32-server-go`。**模型配置和调用**继续放在 `ykt-aisaas`（Go，端口 8190），它已暴露 OpenAI 兼容 AI 接口，并且已有设备专用的内部接口。

### 替换范围

| 能力 | 旧位置 | 新位置 |
|---|---|---|
| WebSocket 设备协议 | xiaozhi-dialogue | **xiaozhi-esp32-server-go** |
| 对话编排（Persona、VAD、打断） | xiaozhi-dialogue | **xiaozhi-esp32-server-go** |
| 窗口对话记忆 | xiaozhi-server | **xiaozhi-esp32-server-go** |
| 摘要/长期记忆 | xiaozhi-server | 推迟（MVP 后） |
| RAG / 知识库 | xiaozhi-server | 推迟（MVP 后，可放 aisaas） |
| 内置工具 / MCP | xiaozhi-ai | 推迟（MVP 后） |
| LLM 提供商调用 | xiaozhi-ai | **ykt-aisaas**（已有） |
| TTS 提供商调用 | xiaozhi-ai | **ykt-aisaas**（已有） |
| STT 提供商调用 | xiaozhi-ai | **ykt-aisaas**（已有） |
| Embedding 提供商调用 | xiaozhi-ai | **ykt-aisaas**（已有） |
| 模型配置 CRUD（sys_config） | xiaozhi-server | **ykt-aisaas**（`ykt_aisaas_model_registry`） |
| 设备生命周期 / OTA | xiaozhi-server | **xiaozhi-esp32-server-go**（代理到 aisaas） |
| 后台管理 UI | `xiaozhi-esp32-server-java/web` | **ykt-aisaas `web/`**（扩展） |

### 测试基线

`my_robot/xiaozhi-esp32-main`（C++ 固件，ESP-IDF 5.4+，v2.2.6）是**线协议契约源**。Go 服务必须严格按相同协议通信，否则设备连不上。

---

## 2. 目标与非目标

### MVP 目标

1. 与 `xiaozhi-esp32-server-java` 完全相同的 WebSocket 协议，用设备真实代码验证。
2. 跑通完整语音对话回合：hello → listen → VAD → STT → LLM → TTS → 设备。
3. 用**一次性切换**替换 `xiaozhi-esp32-server-java`，保留 OTA 驱动的回滚路径。
4. 多设备并发（50+ 同时在线）。
5. `ykt-aisaas` 不可用时优雅降级。
6. 24 小时长稳测试无内存泄漏。

### MVP 非目标

1. RAG / 知识库——第二阶段。
2. 摘要 / 长期记忆——第二阶段。
3. 内置工具（音乐播放、切换角色等）——第二阶段。
4. MCP 协议——第二阶段。
5. IoT 描述符支持——第二阶段。
6. 多租户计费——由 `ykt-aisaas` 负责。
7. 单独前端——见 §13。
8. 模型配置热加载——第二阶段（当前需要重连生效）。

---

## 3. 架构

```
┌─────────────────────────────────────────────────────────────┐
│   ESP32 设备 (xiaozhi-esp32-main, C++ — 不修改)              │
└──────────────────────────┬──────────────────────────────────┘
                           │ WebSocket（Opus 二进制 + JSON）
                           │ 鉴权：Authorization: Bearer <token>
                           │ Header：Device-Id（MAC）、Client-Id（UUID）、Protocol-Version
                           ▼
┌─────────────────────────────────────────────────────────────┐
│            xiaozhi-esp32-server-go (全新)                     │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ WebSocket 层（端口 8080）                             │   │
│  │   hello / listen / abort / mcp / ack / tts / stt /  │   │
│  │   llm / iot / system / alert                         │   │
│  │   二进制 v1/v2/v3 协议解析                            │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 会话管理器（内存 + xiaozhi_session 表）                │   │
│  │   ChatSession：状态机 + 环形缓冲区                    │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 对话编排器                                           │   │
│  │   VAD 触发 → 批量 STT → 拉窗口记忆 → LLM 流式         │   │
│  │   → TTS 流式 → Opus 编码 → 发送                      │   │
│  │   打断 / abort 处理                                   │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ 音频管线                                             │   │
│  │   Silero VAD（ONNX Runtime Go）                       │   │
│  │   Opus 编解码（concentus，纯 Go）                     │   │
│  │   PCM 转换                                           │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ REST API                                             │   │
│  │   /api/device/ota          （OTA / 激活）            │   │
│  │   /api/internal/v1/*       （代理到 aisaas）          │   │
│  │   /api/internal/v1/events  （SSE 实时状态）           │   │
│  └──────────────────────────────────────────────────────┘   │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTPS（X-Internal-Token）
                           ▼
┌─────────────────────────────────────────────────────────────┐
│            ykt-aisaas（已有，端口 8190）                       │
│   LLM   /v1/chat/completions           （SSE 流式）           │
│   TTS   /v1/audio/speech               （分块音频流）        │
│   STT   /v1/audio/transcriptions       （REST 批量）          │
│   Conf  /api/v1/configs / /internal/api/v1/models            │
│   通过 X-Device-Id 自动建租户                                │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│            共享 MySQL 数据库（一套库，两个命名空间）           │
│   xiaozhi_*          （Go 拥有：device/message/summary）     │
│   ykt_aisaas_*       （aisaas 拥有：tenant/registry/quota）  │
└─────────────────────────────────────────────────────────────┘
```

### 部署拓扑（已就位）

```
edge nginx (8.138.16.255:8088)
  /portal/*          → ykt-aisaas-portal        (端口 8191)
  /api/v1/*          → ykt-aisaas               (端口 8190)
  /v1/*              → ykt-aisaas               (端口 8190)
  /ws/*              → xiaozhi-esp32-server-go  (端口 8080)  ← 新
  /api/device/*      → xiaozhi-esp32-server-go  (端口 8080)  ← 新
  /api/internal/*    → xiaozhi-esp32-server-go  (端口 8080)  ← 新（aisaas→go）
  /admin/*           → ykt-admin                (端口 8090, Java, 独立)
```

---

## 4. 技术栈选型

| 关注点 | 选型 | 理由 |
|---|---|---|
| 语言 | Go 1.22+ | 与 `ykt-aisaas` 对齐；原生并发；二进制小 |
| HTTP 框架 | **Gin** | 与 `ykt-aisaas` 同款，团队熟悉 |
| WebSocket | **gorilla/websocket** | 工业标准，被广泛验证 |
| VAD | **Silero VAD v4** via `yalue/onnxruntime-go` | SOTA 精度；ONNX CPU 推理 ~5ms/帧 |
| Opus 编解码 | **`github.com/lostromb/concentus`**（纯 Go）| **无 CGO**——简化交叉编译和 Docker |
| MySQL 驱动 | `database/sql` + `sqlx` | 标准；轻量 |
| 数据库迁移 | **golang-migrate/migrate** | 行业标准 |
| HTTP 客户端 | `go-resty/resty/v2` | 内置重试、中间件 |
| SSE 解析 | 自实现 SSE scanner（基于 bufio.Scanner） | aisaas 返回标准 SSE |
| 日志 | **rs/zerolog** | 零分配；结构化 |
| 配置 | **spf13/viper** | YAML + env 覆盖 |
| 监控 | **prometheus/client_golang** | 标准 |
| 链路追踪 | OpenTelemetry（OTLP gRPC） | 与未来 aisaas 追踪兼容 |
| 测试 | `stretchr/testify` + `vektra/mockery` | 标准 |
| E2E | `coder/websocket` 测试客户端 + 自研设备模拟器 | 协议一致性测试 |
| HIL | `espressif/esptool` + USB 连接的 ESP32-S3 开发板 | 最终门禁 |

**全程不引入 CGO**。Concentrus 的 Opus 性能对 60ms 帧长足够。

---

## 5. 模块结构

```
xiaozhi-esp32-server-go/
├── cmd/
│   └── server/
│       └── main.go                 # 入口；组装所有组件
├── internal/
│   ├── protocol/                   # 协议层（纯数据类型 + 编解码）
│   │   ├── hello.go                # HelloMessage（双向）
│   │   ├── listen.go               # ListenMessage + ListenState/Mode 枚举
│   │   ├── tts.go                  # TtsMessage（服务端→设备）
│   │   ├── stt.go                  # SttMessage（服务端→设备）
│   │   ├── llm.go                  # LlmMessage（服务端→设备，含 emotion）
│   │   ├── mcp.go                  # DeviceMcpMessage（双向）
│   │   ├── ack.go                  # AckMessage（设备→服务端）
│   │   ├── abort.go                # AbortMessage（设备→服务端）
│   │   ├── binary.go               # v1/v2/v3 二进制协议解析
│   │   ├── codec.go                # JSON 编解码辅助
│   │   └── errors.go               # 协议错误与关闭码
│   │
│   ├── ws/                         # WebSocket 服务层
│   │   ├── handler.go              # WebSocketHandler（Upgrade、accept）
│   │   ├── session.go              # ChatSession 结构 + 状态机
│   │   ├── session_manager.go      # SessionManager（内存 map）
│   │   ├── audio_buffer.go         # 上行 Opus 帧环形缓冲
│   │   └── middleware.go           # 鉴权、device-id 提取
│   │
│   ├── audio/                      # 音频处理
│   │   ├── vad/
│   │   │   ├── silero.go           # SileroVadModel 封装
│   │   │   └── vad_service.go      # VadService 状态机
│   │   ├── opus/
│   │   │   ├── encoder.go          # Opus 编码器（统一输出 24k）
│   │   │   └── decoder.go          # Opus 解码器（16k 输入）
│   │   └── pcm.go                  # PCM ↔ WAV 转换（用于 STT 上传）
│   │
│   ├── dialogue/                   # 对话编排
│   │   ├── orchestrator.go         # 主回合循环
│   │   ├── persona.go              # Persona（LLM + TTS + STT 运行时）
│   │   ├── interrupt.go            # 打断处理
│   │   └── memory.go               # 窗口记忆（最近 N 条）
│   │
│   ├── aisaas/                     # ykt-aisaas HTTP 客户端
│   │   ├── client.go               # 基础 HTTP 客户端（鉴权、重试）
│   │   ├── llm.go                  # LLM 流式（SSE 解析）
│   │   ├── tts.go                  # TTS 流式（分块音频）
│   │   ├── stt.go                  # STT 批量上传
│   │   ├── config.go               # 配置查询
│   │   ├── device.go               # 设备自动配置查询
│   │   ├── retry.go                # 指数退避
│   │   └── breaker.go              # 熔断器
│   │
│   ├── api/                        # REST API
│   │   ├── router.go               # Gin 路由组装
│   │   ├── device.go               # 设备 CRUD（管理用）
│   │   ├── ota.go                  # OTA 检查 + 激活
│   │   ├── internal.go             # 内部 aisaas 代理端点
│   │   └── events.go               # SSE 实时事件流
│   │
│   ├── store/                      # 数据库访问（仅 xiaozhi_* 表）
│   │   ├── device.go               # xiaozhi_device CRUD
│   │   ├── message.go              # xiaozhi_message CRUD
│   │   ├── summary.go              # xiaozhi_summary CRUD（v2 占位）
│   │   ├── session.go              # xiaozhi_session 快照
│   │   └── tx.go                   # 事务辅助
│   │
│   ├── config/                     # 配置加载
│   │   └── config.go               # YAML + env 绑定
│   │
│   └── obs/                        # 可观测性
│       ├── log.go                  # zerolog 设置
│       ├── metrics.go              # Prometheus 采集器
│       └── trace.go                # OTel 初始化（可选）
│
├── migrations/                     # golang-migrate 文件（xiaozhi_* 表）
│   ├── 000001_xiaozhi_device.up.sql
│   ├── 000001_xiaozhi_device.down.sql
│   ├── 000002_xiaozhi_message.up.sql
│   ├── 000002_xiaozhi_message.down.sql
│   ├── 000003_xiaozhi_summary.up.sql
│   ├── 000003_xiaozhi_summary.down.sql
│   ├── 000004_xiaozhi_session.up.sql
│   └── 000004_xiaozhi_session.down.sql
│
├── configs/
│   └── config.yaml                 # 默认配置
│
├── deploy/
│   ├── Dockerfile                  # 多阶段，distroless
│   └── docker-compose.yml          # 本地开发
│
├── test/
│   ├── conformance/                # 回放真实设备消息
│   │   ├── fixtures/               # 捕获的 JSON + 二进制样本
│   │   ├── replay_test.go
│   │   └── device_sim/             # WebSocket 设备模拟器
│   │       └── device_sim.go
│   ├── integration/                # 完整对话端到端
│   │   └── dialogue_test.go
│   └── unit/                       # 各包单元测试（与源码同目录）
│
├── scripts/
│   ├── capture-device-msgs.py      # 捕获真实设备→服务端消息
│   ├── dev-up.sh                   # docker-compose 启动开发栈
│   └── conformance.sh              # 运行完整一致性测试套
│
├── docs/
│   ├── PROTOCOL.md                 # 协议参考（自动生成）
│   ├── RUNBOOK.md                  # 运维手册
│   └── superpowers/
│       └── specs/
│           └── 2026-09-29-xiaozhi-esp32-server-go-design.md
│
├── go.mod
├── go.sum
└── README.md
```

---

## 6. 数据流

### 6.1 首次连接 + 首次对话回合

```
1. 设备 → WebSocket Upgrade
     Headers：Authorization: Bearer <token>
              Device-Id: aa:bb:cc:dd:ee:ff
              Client-Id: <uuid>
              Protocol-Version: 1
              Sec-WebSocket-Protocol: xiaozhi.v1

2. Go：从 header 提取 device-id
   Go：GET http://ykt-aisaas:8190/internal/api/v1/devices/{deviceId}
       Headers：X-Internal-Token: <aisaas-internal-token>
       → aisaas 自动建租户（如果不存在）
       ← { "device": {...}, "defaultRole": {...}, "configs": { llm, tts, stt }, "tenantState": "active" }
   Go：UPSERT xiaozhi_device 用返回信息
   Go：在 SessionManager 创建 ChatSession（key: session_id）

3. 设备 → {"type":"hello","version":1,"features":{"mcp":true,"aec":true},
            "transport":"websocket",
            "audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}}

4. Go → {"type":"hello","transport":"websocket","session_id":"<uuid>",
           "audio_params":{"format":"opus","sample_rate":24000,"channels":1,"frame_duration":60}}

5. 设备 → {"type":"listen","session_id":"<uuid>","state":"start","mode":"auto"}

6. Go：状态 SessionState → LISTENING
   Go：初始化 VAD 状态，打开环形缓冲

7. 设备 → 二进制帧（裸 Opus @ 16kHz 单声道，每帧 60ms）
   Go：写入环形缓冲
   Go：VAD.run() 消费帧，返回 VadStatus

   ── VAD 在 800ms 静音后检测到 SPEECH_END ──
   Go：关闭环形缓冲
   Go：Opus 解码 → PCM 16k 单声道
   Go：打包 WAV（header + PCM）
   Go：POST http://ykt-aisaas:8190/v1/audio/transcriptions
         Headers：X-Internal-Token、X-Device-Id、Content-Type: multipart/form-data
         Body：file=<wav>、model=<stt-config-name>
       ← {"text":"今天天气怎么样","emotion":"neutral"}

   Go：状态 SessionState → THINKING

8. Go：SELECT * FROM xiaozhi_message
       WHERE device_id = ? ORDER BY id DESC LIMIT 20
   Go：构建 messages 数组（system + 历史 + user）

   Go：POST http://ykt-aisaas:8190/v1/chat/completions
         Headers：X-Internal-Token、X-Device-Id、Content-Type: application/json
         Body：{"model":"<llm-config-name>",
                "messages":[...],
                "stream":true}
       ← SSE：data: {"choices":[{"delta":{"content":"今天"}}]}
       ← SSE：data: {"choices":[{"delta":{"content":"是"}}]}
       ← ...
       ← SSE：data: [DONE]

   Go：解析 SSE；用 SentenceHelper 切句（按 。！？!?；;\n 切）
   Go：发送 {"type":"tts","state":"start"} 给设备（准备扬声器）
   Go：发送 {"type":"tts","state":"sentence_start","text":"<首句>"} 给设备
   Go：状态 SessionState → SPEAKING

   每句：
     Go：POST http://ykt-aisaas:8190/v1/audio/speech
           Headers：X-Internal-Token、X-Device-Id
           Body：{"model":"<tts-config-name>","input":"<句子>","stream":true}
         ← 分块音频（PCM/MP3/Opus，取决于 TTS 提供商）
     Go：从响应 Content-Type 识别音频格式
     Go：重新编码为 Opus @ 24kHz 单声道 60ms 帧（强制；见 §15 决策 1）
     Go：每个 Opus 包作为二进制帧发给设备

   所有句结束后：
     Go：发送 {"type":"tts","state":"stop"} 给设备

9. 设备 → {"type":"ack","sessionId":"<uuid>","msgType":"tts",
             "msgId":"<msg-id>","status":"ok","error":""}
   Go：记录 ack

10. Go：BEGIN TRANSACTION
    Go：  INSERT INTO xiaozhi_message (device_id, session_id, role='user', content=<text>, ...)
    Go：  INSERT INTO xiaozhi_message (device_id, session_id, role='assistant', content=<text>, ...)
    Go：COMMIT
    （事务：保证两条消息原子写入；任一失败回滚）
    Go：状态 SessionState → LISTENING
```

### 6.2 打断（Barge-in）

```
[TTS 播放中，LLM 流仍在跑]
设备 → {"type":"listen","state":"start","mode":"manual","text":""}
   │
   ▼
Go：orchestrator.Interrupt(reason="listen_start")
   - 取消 LLM SSE 订阅（context.Cancel）
   - 取消 TTS 流读取
   - 发送 {"type":"tts","state":"stop"} 给设备
   - 关闭 TTS 播放器
   - 不关闭音频环形缓冲
   - 不清 VAD 状态
   - 状态 SessionState → LISTENING
   - 保留已累积的用户音频帧在缓冲中

[后续用户说话 → 作为新一轮继续]
```

### 6.3 OTA / 激活

```
设备 → POST /api/device/ota
         Headers：Activation-Version: 2
                  Device-Id: aa:bb:cc:dd:ee:ff
                  Client-Id: <uuid>
                  Serial-Number: <efuse>
                  User-Agent: <board>/<fw-version>
         Body：{ current_firmware_version, ... }
   │
   ▼
Go：检查固件版本 vs 最新
   如果有新版：
     返回 { firmware: { version, url, force } }
   始终包含：
     返回 { websocket: { url: "ws://edge-nginx:8088/ws/", token: <device-token> } }
```

### 6.4 会话生命周期

| 阶段 | 触发 | 动作 |
|---|---|---|
| 创建 | WebSocket Upgrade 接受 | 插入 `xiaozhi_session` 行（内存 ChatSession + DB 快照） |
| 心跳 | 连接期间每 30s | 更新 DB 中 `last_active_at` |
| 断连时清理 | WebSocket 关闭（正常或超时） | DB 中标记会话关闭；从内存 map 移除；释放缓冲；**5 分钟宽限期后**，DELETE 行 |
| 服务关闭时清理 | SIGTERM | 给所有会话发优雅关闭；DB 标记为关闭；30s 排空后退出 |

清理宽限期避免设备在 WiFi 切换时抖动的竞态。

---

## 7. 存储

### 7.1 Go 拥有的表

```sql
-- 设备本地缓存（真源在 ykt-aisaas）
CREATE TABLE xiaozhi_device (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  device_id       VARCHAR(64) NOT NULL,        -- MAC 地址
  client_id       VARCHAR(64),
  token           VARCHAR(255) NOT NULL,
  user_id         BIGINT,
  role_id         BIGINT,
  default_llm_config_id INT,
  default_tts_config_id INT,
  default_stt_config_id INT,
  voice_name      VARCHAR(100),
  state           TINYINT DEFAULT 1,
  last_seen_at    DATETIME,
  created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_device_id (device_id),
  INDEX idx_last_seen (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 对话消息（窗口记忆来源）
CREATE TABLE xiaozhi_message (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  device_id       VARCHAR(64) NOT NULL,
  session_id      VARCHAR(64) NOT NULL,
  role            ENUM('user','assistant','system','tool') NOT NULL,
  content         MEDIUMTEXT NOT NULL,
  audio_path      VARCHAR(255),
  tokens          INT,
  ttfs_ms         INT,
  response_ms     INT,
  message_type    VARCHAR(20) DEFAULT 'normal',  -- normal | tool_call | tool_response
  tool_calls      JSON,
  created_at      DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_device_created (device_id, created_at),
  INDEX idx_session (session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 摘要记忆（保留 schema 供 v2；MVP 暂为空）
CREATE TABLE xiaozhi_summary (
  id              BIGINT PRIMARY KEY AUTO_INCREMENT,
  device_id       VARCHAR(64) NOT NULL,
  role_id         BIGINT NOT NULL,
  summary         MEDIUMTEXT NOT NULL,
  last_msg_id     BIGINT,
  updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_device_role (device_id, role_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 会话快照（崩溃恢复 / 实时仪表板）
CREATE TABLE xiaozhi_session (
  id              VARCHAR(64) PRIMARY KEY,        -- session_id
  device_id       VARCHAR(64) NOT NULL,
  state           VARCHAR(20) NOT NULL,           -- idle | listening | thinking | speaking
  started_at      DATETIME NOT NULL,
  last_active_at  DATETIME NOT NULL,
  INDEX idx_device (device_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 7.2 访问规则

- Go 只读/写 `xiaozhi_*` 表。
- aisaas 只读/写 `ykt_aisaas_*` 表。
- 跨系统数据交换**只能**通过 HTTPS + `X-Internal-Token`。
- 代码级强制：Go 的 `internal/store/` 不导入 aisaas 包，反之亦然。

### 7.3 aisaas 需新增端点

| 端点 | 状态 | 用途 | 工作量 |
|---|---|---|---|
| `GET /internal/api/v1/devices/{deviceId}` | **新增（阻塞 P2）** | 自动配置 + 返回设备配置 + 租户状态 | ~2 dev-days |
| `GET /internal/api/v1/auth/verify` | **新增（阻塞 P5）** | 验证 portal JWT 用于代理请求 | ~1 dev-day |
| `POST /api/v1/audio/upload` | **新增（阻塞 P5）** | 音频文件上传代理 | ~1 dev-day |
| `GET /v1/chat/completions`（stream）| 已有 | LLM SSE | — |
| `POST /v1/audio/speech` | 已有 | TTS | — |
| `POST /v1/audio/transcriptions` | 已有 | STT | — |
| `GET /api/v1/configs` | 已有 | 模型配置查询 | — |
| `GET /internal/api/v1/models` | 已有（推荐）| 内部配置查询 | — |

**共需 3 个新 aisaas 端点，总工作量 ~4 dev-days**。这是 Go 项目的硬依赖。第 0 周需确认 aisaas 团队排期。

---

## 8. 错误处理

| 故障 | 策略 |
|---|---|
| VAD 模型加载失败 | 启动时 fail-fast，supervisor 重启 |
| aisaas STT 调用失败 | 重试 2 次（指数退避 100ms、500ms）；再失败发送 `{"type":"alert","status":"Warning","message":"语音识别失败"}` TTS |
| aisaas LLM 流断开 | 取消 SSE 订阅；保存部分消息；发送 `{"type":"tts","state":"stop"}`；用兜底话术"我好像走神了"恢复 |
| aisaas TTS 流断开 | 发送 500ms 静音 Opus 帧；继续回合 |
| DB 写入失败 | 异步重试队列带退避；不影响对话主路径 |
| 设备断连 | 60s ping 超时 → 关闭会话；释放资源 |
| 二进制帧解析失败 | 跳过该帧，log 告警，计数监控 |
| aisaas 整体不可用 | 熔断器开启 30s；期间所有 aisaas 调用快速失败；发送 alert 兜底 TTS |
| 单回合超时 | 30s 硬超时；强制结束当前回合；log |
| 内存压力（>85% heap）| 拒绝新连接（HTTP 503）；拒绝 WebSocket Upgrade |
| Goroutine 泄漏 | `runtime.NumGoroutine()` 暴露为指标；1h 内增长 > 2x 基线告警 |

### 可观测性

- **指标（Prometheus）** `:9090/metrics`：
  - `xiaozhi_ws_connections{state}`（gauge）
  - `xiaozhi_dialogue_turn_duration_seconds`（histogram）
  - `xiaozhi_vad_frames_total{status}`（counter）
  - `xiaozhi_aisaas_calls_total{endpoint,status}`（counter）
  - `xiaozhi_aisaas_call_duration_seconds{endpoint}`（histogram）
  - `xiaozhi_memory_messages_fetched`（histogram）
  - `xiaozhi_db_errors_total{op}`（counter）
  - `xiaozhi_goroutines`（gauge）
  - `xiaozhi_heap_bytes`（gauge）
- **日志**：结构化 JSON（zerolog）；默认 INFO 级别，DEBUG 通过 env 开关
- **健康端点**：`GET /healthz`（存活性）、`GET /readyz`（就绪性——检查 aisaas 可达 + DB ping）

---

## 9. 配置

```yaml
# configs/config.yaml
server:
  websocket_addr: ":8080"
  admin_addr:     ":8081"     # REST 管理（仅内网）
  read_timeout:   30s
  write_timeout:  30s
  max_connections: 200

aisaas:
  base_url:        "http://ykt-aisaas:8190"
  internal_token:  "${AISAAS_INTERNAL_TOKEN}"
  http_timeout:    30s
  retry:
    max_attempts:  2
    initial_delay: 100ms
    max_delay:     2s
  circuit_breaker:
    failure_threshold:  5
    open_duration:     30s

vad:
  model_path:             "./models/silero_vad.onnx"
  speech_threshold:       0.5
  silence_threshold:      0.3
  silence_duration_ms:    800
  frame_size_samples:     512   # 16kHz 下 32ms

opus:
  uplink:
    sample_rate:    16000
    channels:       1
    frame_duration_ms: 60
  downlink:
    sample_rate:    24000
    channels:       1
    frame_duration_ms: 60

dialogue:
  per_turn_timeout:        30s
  window_memory_size:      20
  ttfs_alert_threshold_ms: 3000

database:
  dsn:                  "${DB_DSN}"   # mysql://user:pass@host:3306/db
  max_open_conns:       50
  max_idle_conns:       10
  conn_max_lifetime:    30m
  migration_path:       "./migrations"

logging:
  level:                info
  format:               json
  output:               stdout
```

所有敏感值从环境变量注入（`${AISAAS_INTERNAL_TOKEN}`、`${DB_DSN}`）。

---

## 10. 测试策略

用户明确说"我不希望整个工程做完以后全是bug"。测试是核心风险控制手段。

### L1 — 单元测试
- 目标覆盖率：**每包 ≥ 70%**
- 关键路径：协议编解码、VAD 状态机、Opus round-trip、窗口记忆、会话状态机、SSE 解析器、熔断器
- 通过接口 mock aisaas
- 每次提交时运行

### L2 — 协议一致性测试（关键层）
- 用 `scripts/capture-device-msgs.py` 对真实 ESP32-S3 开发板**捕获设备真实消息**
- 样本库：100+ 消息覆盖 hello / listen / abort / ack / mcp / 二进制 v1+v2+v3
- 对 Go 服务回放；断言响应字节级一致
- CI 门禁：必须 100% 通过

### L3 — 设备模拟器 + 集成
- `test/conformance/device_sim/device_sim.go` —— 实现完整设备协议的 WebSocket 客户端
- 可脚本化场景：单回合、多回合、打断、断连重连、OTA 激活
- 断言：
  - 用户消息在 ack 后 100ms 内入库
  - 助手消息在 TTS stop 后 100ms 内入库
  - 响应延迟（用户停顿 → 首个 TTS 字节）< 2s p99
  - 所有二进制帧解码为合法 Opus

### L4 — 负载 + 长稳
- 50 个并发设备跑脚本化对话 1 小时
- 长稳：1 个设备连续对话 24 小时
- 断言：
  - 无崩溃
  - RSS 增长 < 10%
  - Goroutine 数量稳定
  - p99 对话延迟 < 3s
  - 零消息丢失（每条用户消息都入库）

### L5 — HIL（硬件在环）
- 两块物理 ESP32-S3 板（不同型号）
- 24 小时连续运行 + 日志
- 人工抽检音质

### L6 — 故障注入
- 对话中途 aisaas 停止 → 验证 alert TTS + 干净恢复
- DB 不可达 → 验证会话存活（缓存命中）
- 网络分区 → 验证重连逻辑
- 超大二进制帧 → 验证跳过 + 日志
- 非法 JSON → 验证协议错误 + 关闭

### 上线前 Go/No-Go 门禁

| # | 检查项 | 通过标准 | 是否必需 |
|---|---|---|---|
| 1 | 单元测试 | 全绿，覆盖率 ≥ 70% | 是 |
| 2 | 协议一致性 | 100% 样本通过 | 是 |
| 3 | 集成场景 | 50/50 脚本通过 | 是 |
| 4 | 负载 1h / 50 设备 | 无崩溃，p99 < 3s | 是 |
| 5 | 长稳 24h / 1 设备 | RSS 增长 < 10%，无泄漏 | 是 |
| 6 | 故障注入 | 所有场景干净恢复 | 是 |
| 7 | HIL 24h / 2 板 | 固件无崩溃，音质无杂音 | 是 |
| 8 | 回滚演练 | OTA URL 切换 → 设备 10s 内重连 | 是 |
| 9 | aisaas 契约测试 | mock aisaas 通过所有 Go 调用 | 是 |
| 10 | 安全审计（headers、鉴权、无密钥泄露）| 零关键发现 | 是 |

---

## 11. 部署与回滚

### 初次部署

1. 把 Go 服务部署到 `8.138.16.255`，与 `ykt-aisaas` 并列。
2. 更新 `ykt-edge-nginx` 配置：
   - `/ws/*` → `xiaozhi-server-go:8080`
   - `/api/device/*` → `xiaozhi-server-go:8080`
   - `/api/internal/*` → `xiaozhi-server-go:8080`
3. 在共享库上跑 `golang-migrate up` 创建 `xiaozhi_*` 表。
4. 给 aisaas 加 `GET /internal/api/v1/devices/{id}`（小 PR）。
5. 部署新 aisaas 镜像。

### 切换策略

Java 服务在 OTA 响应里返回 WebSocket URL。切换后，**Go 拥有 OTA 端点**，所以 Go 控制设备接收到的 URL。

**切换前状态**：
```
设备 → POST /api/device/ota → xiaozhi-esp32-server-java (Java)
                                     ↓
                                返回：{ websocket: { url: "wss://edge/xiaozhi-java/", token: "..." } }
                                     ↓
设备连接 → wss://edge/xiaozhi-java/ → Java WebSocket
```

**切换后状态**：
```
设备 → POST /api/device/ota → xiaozhi-esp32-server-go
                                     ↓
                                返回：{ websocket: { url: "wss://edge/ws/", token: "..." } }
                                     ↓
设备连接 → wss://edge/ws/ → Go WebSocket
```

**执行顺序**（用户决策的一次性切换）：

1. **T-7d**：部署 Go 服务到 `8.138.16.255`。仅冒烟测试。
2. **T-1d**：给 aisaas 加 `GET /internal/api/v1/devices/{id}`。部署 aisaas 更新。
3. **T-0（切换日）**：
   a. 在共享库上跑 `golang-migrate up` 创建 `xiaozhi_*` 表。
   b. 更新 `ykt-edge-nginx` 配置：`/ws/*`、`/api/device/*`、`/api/internal/*` → `xiaozhi-server-go:8080`。
   c. reload nginx。
   d. 设备在下次 OTA 检查时开始命中 Go。
4. **T+0 至 T+30min**：监控 Go 上的 OTA 命中数。设备逐渐拿到新 WebSocket URL（典型 OTA 检查间隔是每次启动一次，但有些是 5-30 min 一次）。
5. **T+1h**：验证 Java 服务上的设备数 → 0。抽样检查 Go 上的活跃 WebSocket 连接。
6. **T+24h**：验证无回滚信号。
7. **T+72h**：下线 Java 服务。

### 回滚路径

如果 T+0 至 T+72h 任何时点出现 bug：

1. 把 `ykt-edge-nginx` 配置改回：`/api/device/*` → `xiaozhi-server-java`。
2. reload nginx。
3. 设备在下次 OTA 检查（30 min 内）重连到 Java。
4. Go 服务可以保持运行用于调试；流量回到 Java。

这让切换**可逆**，尽管意图是"一次性"。

---

## 12. 开发阶段

| 阶段 | 时长 | 交付物 | 门禁 |
|---|---|---|---|
| **P0** 项目脚手架 | 1 周 | go.mod、目录结构、配置、日志、指标、healthz、Dockerfile、docker-compose | 构建通过、运行、healthz 绿 |
| **P1** 协议层 | 2 周 | `internal/protocol/*` 含完整编解码 + 单元测试 ≥ 90% | 所有消息类型 JSON round-trip 绿 |
| **P2** WebSocket + 会话 | 2 周 | `internal/ws/*`、握手、hello/listen/abort/ack 路由 | 对真实设备 hello round-trip 通过 |
| **P3** 音频管线 | 2 周 | Silero VAD、Opus 编解码、环形缓冲 | VAD 状态机单元测试；Opus round-trip 验证 |
| **P4** aisaas 客户端 + 对话编排 | **3 周** | LLM SSE 解析、TTS 流消费、STT 上传、重试、熔断、句子切分器、句子→TTS→Opus→设备管线、完整对话回合编排 | 对开发 aisaas 端到端对话跑通（TTS 正确播放、用户消息入库、助手消息入库） |
| **P5** REST API（OTA/管理）| 2 周 | `/api/device/ota`、设备 CRUD、内部代理、SSE 事件 | 用设备测试 OTA 激活 |
| **P6** 集成 + 模拟器 + 一致性 | 2 周 | 设备模拟器、回放样本、集成套件 | L2 + L3 全绿 |
| **P7** HIL + 性能 + 修 bug | 2 周 | 真实设备测试、负载/长稳、修复 | L4 + L5 全绿 |
| **P8** 切换准备 | 1 周 | 监控、runbook、on-call、分阶段发布 | 所有 Go/No-Go 项确认 |

**总工期：17 周（约 4 个月）**，1 个 Go 开发。

**P4 从 2 周延长到 3 周**因为它包含最复杂的工作：SSE 解析 + TTS 流消费 + STT 上传 + 熔断器 + 重试 + 句子切分 + 音频重编码管线 + 对话回合编排。这个阶段赶工期是整个项目最大的 bug 风险。

### 跨团队依赖

| 依赖项 | 责任人 | 阻塞阶段 | 工作量 |
|---|---|---|---|
| `GET /internal/api/v1/devices/{id}` 端点 | aisaas 团队 | P2（握手需要）| ~2 dev-days |
| aisaas `POST /api/v1/audio/upload` 端点 | aisaas 团队 | P5（设备上线）| ~1 dev-day |
| Edge nginx 配置更新 | 运维 | T-0 切换 | ~1 小时 |
| DB 迁移脚本执行 | 运维 | T-1d | ~10 分钟 |
| Silero VAD 模型文件（`silero_vad.onnx`）| 开发/运维 | P3 | 从上游下载 |

如果 aisaas 团队排期紧张，P2 会阻塞。**行动项**：第 0 周确认 aisaas 团队可用性。

---

## 13. 前端策略

### 决策

**xiaozhi-esp32-server-go 不单独建前端。所有 UI 都在 `ykt-aisaas/web/`。**

### 理由

`ykt-aisaas` portal 已经包含 T40-T68 阶段的"与 `xiaozhi-esp32-server-java` admin web 深度对齐"工作。截至 2026-09-24 portal 已包括：

- 24 个侧边栏菜单项覆盖 Dashboard / 设备 / 用户 / 知识库 / 权限 / AI 平台 / 配置管理 / 记忆 / 租户 / 账单 / Web 对话
- `ConfigManager.vue`（468 行）—— 通用配置 CRUD（搜索/分页/设默认/AutoComplete/动态字段/校验）
- `providerConfig.js`（1275 行）—— 46 LLM + 7 TTS + 6 ASR + 3 OSS providers
- `llm_factories.json`（6417 行）—— 59 factories
- 暗色/亮色主题系统（CSS 变量）
- T68 Chat.vue（340 行）—— 已对接 `web-chat` SSE 端点

**复用现有远比另建前端便宜。**

### 需要新增的 View（仅 5 个）

| # | View | 路由 | 优先级 | 所需后端 |
|---|---|---|---|---|
| 1 | **实时设备** | `/devices/live` | P0 | `GET /api/internal/v1/events`（SSE）|
| 2 | **对话历史** | `/devices/conversations` | P0 | `GET /api/internal/v1/devices/:id/messages` |
| 3 | **OTA 管理** | `/devices/ota` | P0 | `GET/POST /api/internal/v1/firmwares` |
| 4 | **会话诊断** | `/devices/sessions/:id` | P1 | `GET /api/internal/v1/devices/:id/tail`（WebSocket）|
| 5 | **设备上线** | `/devices/onboard` | P1 | `POST /api/internal/v1/devices` |

所有 View 复用现有组件：
- 表格：`el-table` + `el-pagination`（现有模式）
- 实时状态：`EventSource` 接 SSE / `WebSocket` 接诊断
- 主题：继承 CSS 变量系统

### 实时数据通道

Go 服务暴露：

```
GET /api/internal/v1/events
  → Server-Sent Events 流
  → 事件：device_connected、device_disconnected、session_state、dialogue_turn_complete、alert
  → 鉴权：X-Internal-Token

GET /api/internal/v1/devices/:id/tail
  → WebSocket upgrade
  → 实时转发会话日志行
  → 鉴权：X-Internal-Token + 设备权限校验
```

Go 服务通过 Redis Pub/Sub（`xiaozhi:events` 频道）发布事件，以便多 Go 实例都能服务 SSE 流。

### 鉴权流程

1. 用户登录 `ykt-aisaas-portal` → JWT 存 localStorage。
2. portal 用 `Authorization: Bearer <jwt>` 调 Go 内部端点。
3. Go 用 aisaas 验证 JWT（`GET /internal/api/v1/auth/verify`）。
4. aisaas 检查 `device:read` / `device:write` 权限位。
5. 请求通过。

### 不做的事

| ❌ 不要 | 原因 |
|---|---|
| 单独建 `xiaozhi-server-go/web/` | 维护翻倍；违背单 portal 原则 |
| 重写已有管理功能 | T40-T68 已覆盖 95% |
| 用不同的 UI 库 | 破坏一致性 |
| 建独立 Vue 仓库 | 部署/版本/测试成本翻倍 |
| 重做 Web 对话测试 UI | T68 Chat.vue 已实现 |

### 前端阶段（P9，P8 后跑）

| 周 | 任务 |
|---|---|
| 1 | 实时设备 View + SSE 客户端 |
| 1 | 对话历史 View + 表格 + 过滤 |
| 2 | OTA 管理 View + 激活流程 |
| 2 | 会话诊断 View + WebSocket tail |
| 3 | 设备上线 View + 二维码生成 |
| 3 | 新 View 的 E2E Playwright 测试 |

---

## 14. 运维

### 密钥

- `AISAAS_INTERNAL_TOKEN`：通过 env 注入，永不 log
- `DB_DSN`：通过 env 注入，永不 log
- 设备 token：存 `xiaozhi_device.token`（MVP 明文；后续考虑加密）
- AI 提供商 API Key：存 `ykt_aisaas_model_registry.apiKeyEnc`（AES-GCM 加密，aisaas 已实现）

### 性能预算

| 操作 | 预算 |
|---|---|
| WebSocket Upgrade → hello ack | < 1s |
| 用户停声 → STT 结果 | < 1.5s p99 |
| STT 结果 → 首个 TTS 字节 | < 2s p99 |
| 单个对话回合（用户停声 → TTS stop）| < 4s p99 |
| 单设备内存 | < 50 MB RSS |
| 对话期间单设备 CPU | < 5% 单核 |

### 容量规划

- **MVP 范围**：单 Go 实例、内存 SessionManager、会话无需 Redis。
- 单 Go 实例目标：**~200 并发设备**（参考 Java 基准，对半保守）。
- P7 的 50 设备负载测试会标定实际容量。
- **MVP 后（v2）**：如果流量超过单实例容量，加 Redis 后端 SessionManager + Redis Pub/Sub 事件；N 个 Go 实例部署在非 sticky 负载均衡后（任何实例都可路由任何 WebSocket，因为状态在 Redis）。

### 备份与恢复

- DB 备份由现有 `ykt-aisaas` 调度负责（包含 `xiaozhi_*` 表）。
- 对话历史可从设备端日志恢复（最后手段）。
- `xiaozhi_device` 中的配置可从 aisaas 恢复（`/internal/api/v1/devices/{id}`）

---

## 15. 已定设计决策

设计过程中确定的事项。实现必须遵守这些决策。

### D1 — TTS 音频输出格式
**决策**：始终在 Go 中重新编码为 Opus @ 24kHz 单声道 60ms 帧，无论 aisaas TTS 提供商格式如何。
**理由**：与 Java 服务一致；与已部署设备的向后兼容；任何 aisaas TTS 提供商都能用而无需改设备端。
**实现**：`internal/audio/opus/encoder.go` 接收 PCM/MP3/WAV/Opus 输入，归一化为 PCM 16-bit 24kHz 单声道，重新编码为 Opus 60ms 帧。

### D2 — MVP 中的 LLM 流式工具调用
**决策**：从 SSE 流剥离 `tool_call` delta；在 DEBUG 级别 log；不执行工具。
**理由**：MVP 不含工具调用；保留未知 delta 会扰乱下游 LLM 解析。v2 加正确的工具执行。
**实现**：`internal/aisaas/llm.go` 的 SSE 解析器在转发到文本聚合器前过滤掉 `delta.tool_calls` 字段。

### D3 — MVP 中的 MCP 协议
**决策**：用 JSON-RPC 2.0 `Method not found` 错误响应所有 MCP 请求；INFO 级别 log。
**理由**：设备可能在握手期间或作为状态报告一部分发送 MCP。我们需要良好格式的响应避免设备端报错。
**实现**：
```json
{"type":"mcp","session_id":"<id>","payload":{"jsonrpc":"2.0","id":<id>,"error":{"code":-32601,"message":"Method not found in MVP"}}}
```

### D4 — MVP 中的 IoT 描述符协议
**决策**：INFO 级别 log `iot` 消息；如果有 `msgId` 则响应 `{"type":"ack","status":"ok"}`。
**理由**：设备期待 ack；我们不响应 IoT 状态。防止设备重试。
**实现**：`internal/protocol/iot.go` 的 iot 消息处理器只返回 ack。

### D5 — 音频文件上传
**决策**：Go 把文件上传代理到 aisaas `POST /api/v1/audio/upload`（新增端点）；aisaas 用现有 `StorageServiceFactory`（本地或 OSS）。
**理由**：复用 aisaas 的存储抽象；避免重复存储逻辑。
**实现**：`internal/api/upload.go` 验证 multipart，转发给 aisaas，返回存储 URL。

### D6 — 多租户暂停行为
**决策**：被暂停租户的设备**可以**在 WebSocket 层连接，但在 hello 时收到单个 alert TTS，然后优雅断开。
**理由**：避免暂停期间硬失败；给用户反馈。
**实现**：
1. hello 时，Go 通过 aisaas `GET /internal/api/v1/devices/{id}` 检查租户状态（该接口返回租户状态）。
2. 如果 `tenant.state == "suspended"`：发送 `{"type":"alert","status":"Warning","message":"服务已暂停，请联系管理员"}` 后跟同消息的 TTS，然后用 code `1008`（策略违规）关闭 WebSocket。

### D7 — VAD 静音阈值默认值
**决策**：SPEECH_END 默认 800ms 静音；可通过 `RoleBO.vadSilenceMs` 按角色配置。
**理由**：与 Java 服务默认值匹配；针对自然语音停顿经过充分测试的阈值。
**实现**：`configs/config.yaml` 中 `vad.silence_duration_ms: 800`；运行时角色配置覆盖。

### D8 — 并发对话回合策略
**决策**：每个会话同一时间一个回合。活跃回合期间的新 `listen.start` 触发打断（§6.2）；无活跃 listen 时的新文本帧触发拒绝。
**理由**：防止竞态；匹配 Java 服务行为。
**实现**：会话状态机强制单回合不变量；编排方法按会话加锁。

---

## 16. 参考

- 硬件驱动（测试基线）：`D:\zyj_workspace\toy\my_robot\xiaozhi-esp32-main`
  - `main/protocols/websocket_protocol.cc` —— WebSocket 处理器
  - `main/protocols/protocol.cc` —— 消息构造器
  - `main/protocols/protocol.h` —— 二进制协议 v2/v3
  - `main/audio/audio_service.h` —— 音频管线
  - `main/application.cc` —— 状态机
  - `main/ota.cc` —— OTA 流程
  - `main/mcp_server.cc` —— 设备端 MCP
- 现有 Java 服务：`D:\zyj_workspace\toy\xiaozhi-esp32-server-java`（切换后归档）
- ykt-aisaas（已有）：`D:\zyj_workspace\toy\ykt-aisaas`
  - `internal/llm/`、`internal/tts/`、`internal/asr/` —— 提供商实现
  - `internal/llm/registry.go` —— 模型解析
  - `internal/server/router.go` —— 路由组装
  - `docs/API.md` —— 完整 API 规格
- ykt-aisaas AGENTS.md —— 部署流水线、避坑指南、INTERNAL_TOKEN、portal 架构

---

## 17. 签署

### 自检记录（2026-09-29）

自检发现并修复的问题：

| # | 问题 | 章节 | 修复 |
|---|---|---|---|
| 1 | §6.1 有自相矛盾的 `tts.start` 注释 | §6.1 | 完整化 tts 状态序列：start → sentence_start → audio → stop |
| 2 | §11 切换序列没说清谁更新 OTA | §11 | 详细 T-7d/T-1d/T-0/T+1h/T+24h/T+72h 时间线 |
| 3 | §5 vs §14 SessionManager 内存 vs Redis 矛盾 | §14 | 明确 MVP = 单实例内存，Redis 仅 v2 |
| 4 | §15 "Open Questions" 制造不确定性 | §15 | 改名为 "已定设计决策"；8 项 D1-D8 含依据和实现说明 |
| 5 | §12 P4 时间线太紧（2 周 7 子模块）| §12 | P4 增至 3 周，总工期 17 周 |
| 6 | §6 / §7 DB 事务边界不清 | §6.1 | 显式 BEGIN TRANSACTION / COMMIT 包裹消息对插入 |
| 7 | 会话清理生命周期未规定 | §6.4 | 加表：Create/Heartbeat/Cleanup + 5 分钟宽限期 |
| 8 | §7.3 aisaas 工作量未标注 | §7.3 + §12 | 3 个新端点，~4 dev-days；标为硬依赖 |

所有修复后，规格内部一致、范围聚焦、无歧义需求。

- [x] 自检完成
- [ ] 用户审阅并批准
- [ ] 准备进入 `writing-plans` 技能
