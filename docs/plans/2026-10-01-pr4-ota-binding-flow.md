# PR-4: OTA 设备绑定流程（对齐 xiaozhi-java）

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development per task. Frequent commits.

**Goal:** xiaozhi-esp32-server-go OTA 端点对齐 xiaozhi-java 的设备绑定流程：未绑定设备返回 activation code（设备显示给用户），用户到 admin UI 输入后设备再调 OTA 拿 WS 地址。已有 bind-code 端点 + persona-by-device 端点由 aisaas 提供，本 PR 只动 xiaozhi-esp32-server-go。

**Architecture:**
- POST `/api/device/ota` — 设备首次/激活查询端点
- GET `/api/device/ota/activate` — 设备轮询"我激活了吗"
- 状态机：
  - 设备未开户 → aisaas `RegisterDevice` → 拿到 bindCode → 返回 `activation.code = bindCode`
  - 设备开户但未绑 persona → 返回 `activation.code = bindCode`（无 websocket）
  - 设备绑 persona → 返回 `websocket.url = ...`
- `ota/activate`：调 aisaas 拿 persona-by-device，绑了 → 200，没绑 → 202

**Tech Stack:** Go 1.25.5 · resty/v2 · testify · gin

---

## 参考实现

- xiaozhi-java OTA 业务逻辑：`xiaozhi-esp32-server-java/xiaozhi-server/src/main/java/com/xiaozhi/device/DeviceAppService.java:202-273`
- MAC 校验：`xiaozhi-esp32-server-java/xiaozhi-common/src/main/java/com/xiaozhi/utils/CommonUtils.java:23-29`
- aisaas bind-code：`ykt-aisaas/internal/server/internalapi/internalapi.go:115-142`
- aisaas persona-by-device：`ykt-aisaas/internal/server/internalapi/persona.go:186-207`

---

## Task 1: MAC 地址验证 util

**Files:**
- Create: `internal/api/mac.go`
- Create: `internal/api/mac_test.go`

**Step 1-5: TDD** (见各 task 内部)

---

## Task 2: aisaas.DeviceInfo 扩展字段

**Files:**
- Modify: `internal/aisaas/client.go:29-38` — DeviceInfo 加 `BindCode` + `TenantID`
- Modify: `internal/aisaas/client_test.go` — 验证 GetDevice 返回新字段

---

## Task 3: aisaas.Client.RegisterDevice

**Files:**
- Modify: `internal/aisaas/client.go` — 加 `RegisterDevice(ctx, deviceID) error` 方法
- Modify: `internal/aisaas/client_test.go`

---

## Task 4: aisaas.Client.GetPersonaByDevice 返回 bind 状态

**Files:**
- Modify: `internal/aisaas/client.go:96-125` — 返回 `(Persona, *PersonaBind, error)` 三元组
- Modify: `internal/aisaas/llm.go` — 改 GetPersonaByDevice 调用方（orchestrator）

---

## Task 5: OTA handler 加 MAC 校验

**Files:**
- Modify: `internal/api/ota.go:59-79` — 调 `IsMacAddressValid`
- Modify: `internal/api/ota_test.go`

---

## Task 6: OTA 未绑定设备返回 activation code

**Files:**
- Modify: `internal/api/ota.go` — 加分支：unbound → `{"activation":{"code":bindCode,"message":bindCode,"challenge":deviceID}}`
- Modify: `internal/api/ota_test.go` — 加 `TestOTA_UnboundDevice_ReturnsActivation`

---

## Task 7: OTA 已绑定设备返回 websocket

**Files:**
- Modify: `internal/api/ota.go` — 调 GetPersonaByDevice，bind != nil → 返回 websocket
- Modify: `internal/api/ota_test.go` — `TestOTA_BoundDevice_ReturnsWebSocket`

---

## Task 8: GET /api/device/ota/activate

**Files:**
- Create: `internal/api/ota_activate.go`
- Create: `internal/api/ota_activate_test.go`
- Modify: `internal/api/router.go:33-44` — 注册路由

---

## Task 9: 集成测试 + 重启验证

**Files:**
- Modify: `internal/api/router_integration_test.go` — 加完整 bind flow 测试
- Build: `bin/xiaozhi-server.exe`
- Restart: 验证 8081/8082 healthz
- Smoke: `TestSmoke_*` 全 PASS

---

## Task 10: 真机端到端

- 设备首次激活 → 应显示验证码（不再直接尝试 WS）
- 用户在 admin UI 输入 → 设备轮询 200
- 设备连 WS → 对话