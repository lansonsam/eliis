# Eliis · 项目状态（接力交接）

> **给下一个 AI / 下一个会话看的"我们走到哪了"。**
> 与 `DESIGN.md`（蓝图）、`CLAUDE.md`（红线）配套使用。
> 进度有变化时**先更新本文件**，再继续干活。

- **最后更新**：2026-05-10（v0.1 骨架阶段 · IR 字段冻结）
- **当前阶段**：M0.6 · IR 已定型，准备进 M1（OpenAI passthrough）
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
2. **协议转换独立成 N×N converter**，不绑后端实现
3. **协议层（codec）≠ 后端层（backend）**，让 Bedrock / Vertex 能复用 Anthropic / Gemini codec
4. **零强依赖**：默认无 DB / 无 Redis / 单二进制可跑

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
cmd/eliis/                      内容：仅 .gitkeep
pkg/embed/                      内容：仅 .gitkeep
internal/core/{contract,bus,config,types,pipeline}/    部分已写
internal/protocol/{openai,anthropic,gemini,converter}/ 仅 .gitkeep
internal/{router,auth,ratelimit,cache,log,metrics,failover,storage}/  仅 .gitkeep
configs/                        仅 .gitkeep
docs/                           仅 .gitkeep
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
| `internal/core/contract/converter.go` | 11 | ✅ **接口已冻结** | `Converter` 接口：From()/To()/Convert/ConvertChunk |
| `internal/core/contract/middleware.go` | 12 | ✅ **接口已冻结** | `Middleware` + `Handler` 类型 |
| `internal/core/bus/bus.go` | 10 | 🟡 占位 | 只有 `Bus struct{}` + `New()` |
| `internal/core/config/config.go` | 35 | ✅ 可用 | YAML 加载实现，含 `Server.Addr` 默认值 |
| `internal/core/pipeline/pipeline.go` | 17 | 🟡 占位 | `Pipeline.Handle()` 直接返回 nil |
| `cmd/eliis/main.go` | ~110 | ✅ 可用 | Gin server，监听配置 addr，含 `/health` + `/`，SIGINT/SIGTERM 优雅关闭 |
| `configs/eliis.yaml` | 9 | ✅ 可用 | 默认监听 `:8090` |
| `docs/IR.md` | ~230 | ✅ 完整 | 三协议字段对照表 + Extra 约定 + 未决问题清单 |

> 标记说明：✅ 可用 · 🟡 占位（需要扩展） · ❌ 未开始

---

## 2. 未完成 ❌（按优先级）

### 2.1 阻塞下一步的（必做）

- [x] **`cmd/eliis/main.go`** —— 已建，Gin server 可启动 ✅
- [x] **填充 `UnifiedRequest/Response/Chunk` 字段** —— IR 已定型，覆盖三协议字段 ✅
- [x] **`docs/IR.md`** —— 三协议字段对照表 + Extra 约定 + Parts/ToolCalls 不变量 ✅
- [x] **首次 git commit** —— root-commit `3e04ad0`，骨架 + IR 一起入库 ✅

### 2.2 M1 · OpenAI passthrough（最小闭环）

- [ ] `internal/protocol/openai/codec.go` —— 实现 `contract.Codec`
- [ ] `internal/protocol/openai/dto.go` —— `ChatCompletionRequest` 等结构
- [ ] `internal/backend/`（**目录还没建**，需要新增）—— OpenAI HTTP 客户端
- [ ] `cmd/eliis/main.go` 接入 `/v1/chat/completions` 透传路由

### 2.3 M2~M4

见 `DESIGN.md` 第 8 节路线图。

### 2.4 待与 owner 对齐的开放问题

- [ ] **DESIGN.md §1.3**："另一种项目"的精确定义还没填，owner 需口头补充
- [ ] **是否新增 `internal/backend/`** 子层（codec 与 HTTP 客户端解耦，DESIGN 漏写，AI 已建议加）
- [ ] **`internal/log/` 是否改名为 `internal/logging/`**（避免和标准库 `log` 冲突）
- [ ] **License**（MIT / Apache-2.0 / AGPL）
- [ ] **Web 框架**：当前共识 Gin，但未在 DESIGN.md 明文写死

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
| 14 | 2026-05-10 | `Extra` 强制用协议名前缀（`openai:` / `anthropic:` / `gemini:`） | 避免跨协议转换时键名冲突；converter 默认丢弃异协议的 Extra 键 |
| 15 | 2026-05-10 | `UnifiedChunk.FinishReason` 用 `*string` | `nil` = 流未结束，`""` = 显式空（罕见但保留区分能力）；与 `Delta == nil` 配合识别终止 chunk |

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
```

---

> 本文档结构稳定，但内容会随每次进展更新。改动范围尽量小、可追溯。
