# Eliis · 项目状态（接力交接）

> **给下一个 AI / 下一个会话看的"我们走到哪了"。**
> 与 `DESIGN.md`（蓝图）、`CLAUDE.md`（红线）配套使用。
> 进度有变化时**先更新本文件**，再继续干活。

- **最后更新**：2026-06-08（M1 垂直切片 · Anthropic Messages 入站 → OpenAI 上游非流式闭环）
- **当前阶段**：M1.1 · HTTP 网关已支持 `/v1/messages` 非流式请求，经 OpenAI-compatible upstream 返回 Anthropic Messages 响应；流式仍待实现
- **owner**：@lansonsam
- **协作模式**：人类主导 + 多 AI 助手（Cursor / Claude Code / 其他）轮流接力

---

## 0. 三十秒理解 Eliis

**Eliis 是一个用 Go 写的、模块化、多协议的 LLM 网关 / 协议适配框架。**

参考代码（**全部只读，禁止修改**，详见 `CLAUDE.md §1`）：

| 目录 | 用途 |
| --- | --- |
| `new-api/` | 协议适配 / Relay 架构 / Channel 实现 |
| `eino/` | IR / Schema / Stream 抽象 / Agent 编排（仅借鉴） |

核心定位见 `DESIGN.md` 第 0–1 节。

要点（与 New API 的关键差异）：

1. **真正中性的 IR**（`UnifiedRequest/Response/Chunk`），而非用 OpenAI dto 当 IR
2. **三正交架构**：Codec（wire↔IR）+ Lens（IR→IR 原子变换）+ Backend（IR→上游），替代 N×N converter
3. **协议层（codec）≠ 后端层（backend）**，让 Bedrock / Vertex 能复用 Anthropic / Gemini codec
4. **同协议入口=出口走 raw passthrough 旁路**：byte-level 直通，零开销
5. **嵌入式 API 已有最小入口**：`pkg/embed.TranslateJSON` 支持 `input=openai`、`output=anthropic/a社` 的请求 JSON 转义
6. **HTTP 网关已有首条真实闭环**：`POST /v1/messages`（Anthropic Messages 非流式）→ OpenAI-compatible `/v1/chat/completions` → Anthropic Messages 响应
7. **零强依赖**：默认无 DB / 无 Redis / 单二进制可跑

---

## 1. 已完成 ✅

### 1.1 仓库与隔离

| 项 | 路径 | 状态 |
| --- | --- | --- |
| Git 仓库初始化 | `.git/` | ✅ master 分支，已首次提交（root-commit `3e04ad0`） |
| 参考代码不入库 | `.gitignore` | ✅ `new-api/` + `eino/` 不入库 |
| 参考代码不索引 | `.cursorindexingignore` | ✅ `new-api/` + `eino/` 不参与 codebase 搜索 |
| AI 完全屏蔽列表 | `.cursorignore` | ✅ 当前仅占位（参考代码改用"可读不可写"策略） |
| AI 红线指令 | `CLAUDE.md` | ✅ 允许 Read，禁止 Write/Edit `new-api/` + `eino/` |
| 设计蓝图 | `DESIGN.md` | ✅ v0.1 草稿（待补 §1.3 项目本质口头描述） |

### 1.2 目录骨架（22 个，全部建好）

```text
cmd/eliis/                      含 main.go
pkg/embed/                      含 embed.go（嵌入式 JSON 转义入口）
internal/core/{contract,bus,config,types,pipeline}/    部分已写
internal/protocol/{openai,anthropic}/    含 dto.go + request-side codec 辅助函数
internal/protocol/lens/                  含 ensure_max_tokens / override_model
internal/protocol/gemini/                仅 .gitkeep
internal/backend/                        仅 .gitkeep（M2 起逐步填充各 backend）
internal/{router,auth,ratelimit,cache,log,metrics,failover,storage}/  仅 .gitkeep
configs/                        含 eliis.yaml
docs/                           含 IR.md
test/e2e/                       仅 .gitkeep
```

### 1.3 Go 模块

- `go.mod`: `module github.com/lansonsam/eliis`，`go 1.26.2`
- 直接依赖：
  - `gopkg.in/yaml.v3 v3.0.1` —— 配置加载
  - `github.com/gin-gonic/gin v1.12.0` —— HTTP 框架（详见决策 #4）

### 1.4 已写代码（M0 占位骨架，**全部是 placeholder，不是最终实现**）

| 文件 | 行数 | 状态 | 说明 |
| --- | --- | --- | --- |
| `internal/core/types/unified.go` | ~220 | ✅ **IR 已定型** | 完整 IR：`UnifiedRequest/Response/Chunk` + `Message/ContentPart/ToolUseBlock/ToolResultBlock/MediaData/ToolDef/ThinkingConfig/ResponseFormat/TokenUsage` + `Role/ContentType` 枚举 |
| `internal/core/types/context.go` | 10 | 🟡 最小 | 含 `Request *http.Request` + `RequestID string` |
| `internal/core/contract/codec.go` | 17 | ✅ **接口已冻结** | `Codec` 接口完整：Decode/Encode Request/Response/StreamChunk |
| `internal/core/contract/lens.go` | ~30 | ✅ **接口已冻结** | `Lens` 接口 + `LensChain` 辅助类型（替代旧 Converter） |
| `internal/core/contract/backend.go` | ~25 | ✅ **接口已冻结** | `Backend` + `StreamReader` 契约，真实 backend 尚未实现 |
| `internal/core/contract/middleware.go` | 12 | ✅ **接口已冻结** | `Middleware` + `Handler` 类型 |
| `internal/core/bus/bus.go` | 10 | 🟡 占位 | 只有 `Bus struct{}` + `New()` |
| `internal/core/config/config.go` | ~130 | ✅ 可用 | YAML 加载实现，含 `Server` / `Log` / OpenAI upstream / Anthropic route 配置默认值与 env 展开 |
| `internal/core/pipeline/pipeline.go` | 17 | 🟡 占位 | `Pipeline.Handle()` 直接返回 nil |
| `cmd/eliis/main.go` | ~280 | ✅ 可用 | Gin server，监听配置 addr，含 `/health` + `/` + `/v1/messages` 非流式路由，SIGINT/SIGTERM 优雅关闭 |
| `configs/eliis.yaml` | ~35 | ✅ 可用 | 默认监听 `:8090`，含 OpenAI-compatible upstream 与 Anthropic Messages route 示例 |
| `docs/IR.md` | ~255 | ✅ 完整 | 三协议字段对照表 + Extra 约定 + 未决问题清单 |
| `internal/protocol/openai/dto.go` | ~340 | ✅ 完整 | OpenAI Chat Completions 协议 DTO（snapshot 2026-05），无业务方法 |
| `internal/protocol/anthropic/dto.go` | ~330 | ✅ 完整 | Anthropic Messages API DTO（snapshot 2026-05），无业务方法 |
| `internal/protocol/openai/codec.go` | ~700 | 🟡 非流式可用 | OpenAI Chat Completions 请求 decode/encode、响应 decode；流式仍是占位 |
| `internal/protocol/anthropic/codec.go` | ~470 | 🟡 非流式可用 | Anthropic Messages 请求 decode/encode、响应 encode、error envelope；流式仍是占位 |
| `internal/backend/openai/backend.go` | ~150 | ✅ 非流式可用 | OpenAI-compatible `/chat/completions` HTTP client，实现 `contract.Backend.Invoke` |
| `internal/protocol/lens/*.go` | ~60 | ✅ 可用 | `EnsureMaxTokens` + `OverrideModel` 两个首批 request lens |
| `pkg/embed/embed.go` | ~90 | 🟡 最小可用 | `TranslateJSON` 支持 OpenAI → Anthropic/a社 请求 JSON 转义，同协议 raw JSON 原样返回 |
| `*_test.go` | 多个 | ✅ 可用 | 覆盖 config、OpenAI/Anthropic codec、OpenAI backend、`/v1/messages` e2e |
| `docs/EMBED.md` | ~80 | ✅ 可用 | 嵌入式 API 调用示例与当前能力边界 |

> 标记说明：✅ 可用 · 🟡 占位（需要扩展） · ❌ 未开始

---

## 2. 未完成 ❌（按优先级）

### 2.1 阻塞下一步的（必做）

- [x] **`cmd/eliis/main.go`** —— 已建，Gin server 可启动 ✅
- [x] **填充 `UnifiedRequest/Response/Chunk` 字段** —— IR 已定型，覆盖三协议字段 ✅
- [x] **`docs/IR.md`** —— 三协议字段对照表 + Extra 约定 + Parts/ToolCalls 不变量 ✅
- [x] **首次 git commit** —— root-commit `3e04ad0`，骨架 + IR 一起入库 ✅

### 2.2 M1 · OpenAI-compatible backend / Anthropic Messages 非流式闭环

- [x] `internal/protocol/openai/dto.go` —— `ChatCompletionRequest` 等结构 ✅
- [x] `internal/protocol/openai/codec.go` —— 请求 decode/encode + 非流式响应 decode ✅；流式仍待实现
- [x] `internal/protocol/anthropic/codec.go` —— Anthropic Messages 请求 decode + 非流式响应 encode ✅；流式仍待实现
- [x] `internal/backend/openai/` —— OpenAI-compatible HTTP 客户端，实现 `contract.Backend.Invoke` ✅
- [x] `cmd/eliis/main.go` 接入 `/v1/messages`（Anthropic Messages 非流式入站）✅
- [x] 配置 `upstreams.openai` + `routes.anthropic_messages`，支持 `${OPENAI_API_KEY}` env 展开 ✅
- [x] 首批单元/e2e 测试：config、codec、backend、HTTP route ✅
- [ ] `cmd/eliis/main.go` 接入 `/v1/chat/completions` OpenAI 入站透传路由
- [ ] **可选**：`raw passthrough` 旁路实现（同协议直通，先 codec/backend 跑通后再加）

### 2.2.1 M0.8 · 嵌入式转义入口（已做最小闭环）

- [x] `pkg/embed.TranslateJSON` —— 可直接 import 包调用协议转义 ✅
- [x] OpenAI Chat Completions 请求 JSON → OpenAI DTO → IR ✅
- [x] Lens 链：`OverrideModel` + `EnsureMaxTokens` ✅
- [x] IR → Anthropic Messages 请求 JSON ✅
- [x] OpenAI-compatible 真实上游调用 —— `backend/openai.Invoke` 已支持非流式 Chat Completions ✅
- [ ] 嵌入式 API 响应和流式转义 —— `pkg/embed` 仍只做请求 JSON 转义；HTTP 网关已有非流式响应转义

### 2.3 M2~M6

见 `DESIGN.md` 第 8 节路线图。要点：

- M2：Anthropic codec + `backend/anthropic`
- M3：第一次跨协议（OpenAI 入 → Anthropic 出），首批 lens 实装
- M4：Gemini codec + `backend/gemini`，复用已有 lens
- M5：`backend/bedrock` / `backend/vertex`（同 codec、不同 backend）
- M6：周边模块（ratelimit / cache / metrics / failover）

### 2.4 待与 owner 对齐的开放问题

- [ ] **DESIGN.md §1.3**："另一种项目"的精确定义还没填，owner 需口头补充
- [x] ~~**是否新增 `internal/backend/`** 子层~~ —— 已决：M0.7 三正交确立 backend 为独立一层 ✅
- [ ] **`internal/log/` 是否改名为 `internal/logging/`**（避免和标准库 `log` 冲突）
- [ ] **License**（MIT / Apache-2.0 / AGPL）
- [ ] **Web 框架**：当前共识 Gin，但未在 DESIGN.md 明文写死
- [ ] **Lens 是否需要 `ApplyChunk(*UnifiedChunk) error` 流式版本**（M3 实装时回看）

---

## 3. 关键决策记录（ADR-lite）

新决策**追加到末尾**，不要改老的。

| # | 日期 | 决策 | 理由 |
| - | ---- | ---- | ---- |
| 1 | 2026-05-09 | `new-api/` 仅作只读参考，写入隔离三件套 | 避免 AI 误改参考代码 |
| 2 | 2026-05-09 | IR 不直接用 OpenAI dto，单独造 `Unified*` | New API 的最大教训：用 OpenAI 当 IR 导致信息有损 |
| 3 | 2026-05-09 | 协议转换独立成 `protocol/converter/` 子包 | New API 把转换塞 channel 里导致复用难、N×N 必绕 OpenAI |
| 4 | 2026-05-10 | Web 框架倾向 Gin（vs Fiber） | LLM 网关瓶颈在上游推理，需要 HTTP/2 + 大生态；Fiber 不支持 HTTP/2 |
| 5 | 2026-05-10 | IR 设计参考 `cloudwego/eino/schema` | 不重复造轮子，Eino 已有 `Message`/`StreamReader` 等可借鉴 |
| 6 | 2026-05-10 | `eino/` 也加入只读参考隔离三件套 | 与 `new-api/` 同等待遇：仅供查阅 IR / Schema 设计 |
| 7 | 2026-05-10 | 参考代码隔离策略改为"可读不可写"：`.cursorignore` 不再屏蔽 `new-api/` `eino/`，改用 `.cursorindexingignore`（不索引）+ `CLAUDE.md` 红线（禁写） | Cursor 无原生只读机制；旧策略让 AI 想查参考都得绕 shell，效率低 |
| 8 | 2026-05-10 | M0.5 最小可运行 server：Gin + `/health` + 优雅关闭，端口 `:8090` | 让骨架可启动可调用，方便后续 M1 协议接入直接挂路由 |
| 9 | 2026-05-10 | IR 不带 `json` tag，由各 codec 自负协议字节流 | IR 是内部表达，不暴露到任何线上格式；避免被某一协议命名习惯（snake/camel）锁死 |
| 10 | 2026-05-10 | System prompt 升到 `UnifiedRequest.System` 顶层（非走 Messages） | OpenAI/Anthropic/Gemini 三种位置不同；放顶层让任何 codec 都能直接读，不用扫描 messages |
| 11 | 2026-05-10 | 采样参数（Temperature/TopP/MaxTokens 等）一律用指针 | 区分"未设置"与"零值"；Temperature=0 是合法语义，零值表达不充分会导致默认值错乱 |
| 12 | 2026-05-10 | `ToolUseBlock.Input` 用 `json.RawMessage` 而非 `map[string]any` | OpenAI tool_calls 的 arguments 是 string、Anthropic/Gemini 是 object；RawMessage 避免反复 marshal/unmarshal 失真 |
| 13 | 2026-05-10 | `Message` 同时维护 `Parts` 和 `ToolCalls`（后者是前者的镜像） | OpenAI codec 直接读 ToolCalls 更快；Anthropic/Gemini codec 关心顺序读 Parts；invariant 由 producer 保证（未来加工厂函数封装） |
| 14 | 2026-05-10 | `Extra` 强制用协议名前缀（`openai:` / `anthropic:` / `gemini:`） | 避免跨协议转换时键名冲突；出站时由 lens（如 `drop_extra` 链）按命名空间过滤 |
| 15 | 2026-05-10 | `UnifiedChunk.FinishReason` 用 `*string` | `nil` = 流未结束，`""` = 显式空（罕见但保留区分能力）；与 `Delta == nil` 配合识别终止 chunk |
| 16 | 2026-05-10 | 采纳 **Codec + Lens + Backend 三正交**架构，替代原 N×N converter | N×N converter 与 IR 中性化目标自相矛盾；三正交后新增协议 = 1 codec，跨协议差异由若干原子 lens 在路由配置里 compose 解决，扩展度从 N² 降到 N+ε |
| 17 | 2026-05-10 | 同协议入口 = 出口走 **raw passthrough 旁路**（byte-level 透传） | 网关场景独有：OpenAI→OpenAI 不需要解码到 IR 再编码回去，直通可省一次 marshal/unmarshal、保留所有 OpenAI 专属字段、零延迟开销 |
| 18 | 2026-05-10 | **Codec 与 Backend 解耦**：同一 codec 可服务多个 backend | Anthropic codec 可同时挂直连 / Bedrock / Vertex 三个 backend；切换上游服务商时只改 backend，codec 与 lens 完全复用 |
| 19 | 2026-05-10 | 先落地 `pkg/embed.TranslateJSON` 作为库调用最小入口 | owner 需要可直接 import 包、设置 `input=openai` / `output=a社` 的测试形态；先验证 request-side codec+lens+codec 链路，再接真实 backend |
| 20 | 2026-05-10 | M0.8 只支持请求 JSON 转义，不伪装成真实上游调用 | 当前还没有 `backend/*` HTTP 客户端和 response/stream codec；明确边界避免把“转义可用”误解为“代理调用可用” |
| 21 | 2026-06-08 | 首条真实 HTTP 闭环选择 Anthropic Messages 入站 → OpenAI-compatible Chat Completions 上游（非流式） | owner 想先“接入 OpenAI，看看 Claude 协议能不能用”；该路径最快验证 Claude-compatible client、IR、lens、backend、response codec 的端到端协作 |
| 22 | 2026-06-08 | `/v1/messages` 第一版遇到 `stream:true` 明确返回 Anthropic-style `invalid_request_error`，不静默降级为非流式 | Anthropic SSE 是多事件状态机，当前 stream codec 仍占位；静默降级会让流式客户端行为不可预期，明确报错更安全可测 |

---

## 4. 给下一个 AI 的接力指引

**你接手后请按以下顺序：**

1. **先读 `CLAUDE.md`**（红线）
2. **再读 `DESIGN.md`**（蓝图，重点看 §2 三大铁律 + §6 目录结构）
3. **然后读本文件 §1, §2**，知道当前进度和未完成项
4. **必要时进 `new-api/` 或 `eino/` 查参考实现**（可用 `Read`/`Grep`/`Glob`/`Shell` 任意读取；**禁用** `Edit`/`Write` —— 见 `CLAUDE.md §1`）
5. 开工前先和 owner 同步：
   - "我准备做 X，对应 STATUS.md §2.x，方向对吗？"
6. 完成后**回写本文件**：
   - §1 加新条目
   - §2 划掉对应 checkbox
   - §3 追加新决策（如果有）
   - 更新顶部"最后更新"日期

**禁止做的事**（出自 `CLAUDE.md`）：

- ❌ 修改 `new-api/` 或 `eino/` 下任何文件
- ❌ 跳过本文件直接动手干活
- ❌ 在没和 owner 确认前推翻已有决策（§3）
- ❌ 在 `internal/core/` 里 import 上层模块（破坏分层）

---

## 5. 快速命令参考

```powershell
# 启动服务（默认监听 :8090）
go run ./cmd/eliis -config configs/eliis.yaml

# 健康检查
Invoke-WebRequest -Uri http://127.0.0.1:8090/health -UseBasicParsing

# 编译（不运行）
go build ./...

# 测试
go test ./...

# 看依赖
go mod graph

# 看参考代码（可读不可写：IDE 与 shell 都能读，但 AI 不要 Edit/Write）
Get-ChildItem 'new-api/relay/channel' -Directory
Get-ChildItem 'eino/schema' -File

# 嵌入式 API 当前文档
Get-Content docs/EMBED.md
```

---

> 本文档结构稳定，但内容会随每次进展更新。改动范围尽量小、可追溯。
