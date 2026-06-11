# Eliis · IR（Intermediate Representation）字段表

> 中间表示 = Eliis 内部统一的请求/响应/流式结构。
> 三种协议（OpenAI / Anthropic / Gemini）进入 Eliis 后**必须先翻译成 IR**，
> 出去时再从 IR 翻译成目标协议。所有 `codec` 与 `lens` 都只认 IR。
>
> 源代码：[`internal/core/types/unified.go`](../internal/core/types/unified.go)
> 配套接口：[`internal/core/contract/codec.go`](../internal/core/contract/codec.go)、
> [`internal/core/contract/lens.go`](../internal/core/contract/lens.go)
> 设计决策：见 `STATUS.md §3` 决策 #2、#5、#16

---

## 当前实现状态（M0.8）

当前已落地一条 request-side 验证链路：

```text
OpenAI Chat Completions JSON
  -> openai codec decode
  -> UnifiedRequest
  -> LensChain(OverrideModel, EnsureMaxTokens)
  -> anthropic codec encode
  -> Anthropic Messages JSON
```

公开调用入口见 [`docs/EMBED.md`](./EMBED.md) 与 [`pkg/embed`](../pkg/embed)。
这条链路只验证请求 JSON 转义；响应、流式 chunk、真实 backend 调用仍未完成。

---

## 0. 设计原则

1. **能无损表达三协议的并集**——核心字段进 IR；冷门 / 协议专属字段进 `Extra`
2. **IR 不直接序列化到线上**——没有 `json` tag，每个 codec 自己负责协议字节流
3. **保留 "未设置" 与 "零值" 的差别**——采样参数（temperature/top_p 等）一律用指针
4. **OpenAI 不是默认协议**——任何协议都可以做入口或出口，IR 是真正中性的

---

## 1. 顶层类型一览

| 类型 | 用途 | 备注 |
| --- | --- | --- |
| `UnifiedRequest`  | 入站请求 IR | codec 的 `DecodeRequest` 输出 |
| `UnifiedResponse` | 非流式响应 IR | codec 的 `EncodeResponse` 输入 |
| `UnifiedChunk`    | 流式增量 IR | codec 的 `Decode/EncodeStreamChunk` 输入输出 |
| `Message`         | 一条对话轮次 | 内含 `[]ContentPart` |
| `ContentPart`     | 消息里的一个内容块 | text / 媒体 / tool_use / tool_result / reasoning |
| `ToolUseBlock`    | 工具调用 | 三协议工具调用的统一形态 |
| `ToolResultBlock` | 工具返回结果 | 配对 `ToolUseBlock.ID` |
| `MediaData`       | 媒体载荷 | 远程 URL 或 inline 字节 |
| `ToolDef`         | 工具声明 | 含 JSON Schema |
| `TokenUsage`      | 用量统计 | 三套字段名归一 |
| `ThinkingConfig`  | 思考模式配置 | reasoning_effort / thinking budget |
| `ResponseFormat`  | 输出结构约束 | json_object / json_schema |

---

## 2. 三协议差异对照（核心字段）

### 2.1 System Prompt 的位置

| 协议 | 位置 | 说明 |
| --- | --- | --- |
| OpenAI    | `messages[0].role == "system"` | 在消息列表里 |
| Anthropic | 顶层 `system` 字段 | 消息列表只能 user/assistant 交替 |
| Gemini    | 顶层 `systemInstruction` | 单独的 Content 对象 |

**IR 决策**：统一升到顶层 `UnifiedRequest.System string`。

- 入站 OpenAI 时，codec 把 `messages[0]`（如果是 system）拆出来放到 `System`，剩下的进 `Messages`
- 出站 OpenAI 时，OpenAI codec 把 `System` 重新插回 `messages[0]`
- 出站 Anthropic / Gemini 时各自 codec 直接对应原生字段

### 2.2 Tool 调用 / 结果

| 协议 | 调用 | 结果 |
| --- | --- | --- |
| OpenAI    | `message.tool_calls[].{id, function.name, function.arguments(string)}` | `role="tool"` 消息 + `tool_call_id` |
| Anthropic | content block `type=tool_use` `{id, name, input(object)}` | content block `type=tool_result` `{tool_use_id, content, is_error}` |
| Gemini    | `parts[].functionCall.{name, args}` | `parts[].functionResponse.{name, response}` |

**IR 决策**：

- 调用 → `ToolUseBlock`（含 `ID`、`Name`、`Input json.RawMessage`）
  - Gemini 没有 per-call ID，codec **必须合成**一个稳定的 ID（建议 `gemini-tc-{index}`）
  - `Input` 用 `json.RawMessage` 而非 `map[string]any`：避免 OpenAI 的 string-encoded JSON 与 Anthropic/Gemini 的 object 之间反复 marshal/unmarshal 失真
- 结果 → `ToolResultBlock`（含 `ToolUseID`、`Name`、`Content []ContentPart`、`IsError`）
- 在 `Message` 上同时维护 `ToolCalls []ToolUseBlock`（assistant 消息）作为便利访问入口；它必须与 `Parts[i].ToolUse` 保持一致（producer 的责任）

### 2.3 Usage / 用量

| 协议 | 输入 token | 输出 token | 缓存读 | 缓存写 | 思考 |
| --- | --- | --- | --- | --- | --- |
| OpenAI    | `prompt_tokens` | `completion_tokens` | (无原生) | (无原生) | `completion_tokens_details.reasoning_tokens` |
| Anthropic | `input_tokens` | `output_tokens` | `cache_read_input_tokens` | `cache_creation_input_tokens` | (无独立) |
| Gemini    | `promptTokenCount` | `candidatesTokenCount` | `cachedContentTokenCount` | (无原生) | `thoughtsTokenCount` |

**IR 决策**：归一到 `TokenUsage.{InputTokens, OutputTokens, TotalTokens, CacheReadInputTokens, CacheCreationInputTokens, ReasoningTokens}`。

- Gemini 的 `cachedContentTokenCount` 也填进 `CacheReadInputTokens`（语义最接近）
- `TotalTokens` 在 OpenAI / Gemini 上游会给，Anthropic 不给，consumer 需要时自己算

### 2.4 流式 Chunk 形态

| 协议 | chunk 形态 |
| --- | --- |
| OpenAI    | 单一 SSE 事件 `chat.completion.chunk`，`choices[].delta` 增量 |
| Anthropic | **多种** SSE 事件：`message_start` / `content_block_start` / `content_block_delta` / `content_block_stop` / `message_delta` / `message_stop` |
| Gemini    | 复用 `GenerateContentResponse`，整段 chunk 直接是一个 candidate 增量 |

**IR 决策**：`UnifiedChunk.Delta *Message` 承载所有增量内容。

- Anthropic 的多事件需要 codec **聚合**：例如 `content_block_start(tool_use)` + 多个 `content_block_delta(input_json_delta)` 应聚合成一个 `ContentPart{Type: tool_use, ToolUse: ...}` 的增量
- `FinishReason *string` 用指针：`nil` = 流未结束，`""` = 显式空（罕见，但保留区分能力）
- 终止 chunk 通常 `Delta == nil` + `FinishReason != nil`，可能附带 `Usage`

### 2.5 思考 / Reasoning

| 协议 | 形态 |
| --- | --- |
| OpenAI    | 请求 `reasoning_effort: low/medium/high`；响应 `message.reasoning_content` |
| Anthropic | 请求 `thinking: {type:"enabled", budget_tokens}`；响应 content block `type=thinking` |
| Gemini    | 请求 `generationConfig.thinkingConfig`；响应 `parts[].thought = true` |

**IR 决策**：

- 请求侧：`UnifiedRequest.Thinking *ThinkingConfig`，含 `Enabled / BudgetTokens / Effort` 三字段，codec 按能力翻译
- 响应侧：`ContentPart{Type: ContentTypeReasoning, Reasoning: "..."}`，与正文 text part 顺序混排

### 2.6 Role 命名

| 协议 | assistant 角色名 |
| --- | --- |
| OpenAI    | `"assistant"` |
| Anthropic | `"assistant"` |
| Gemini    | `"model"` |

**IR 决策**：IR 统一用 `"assistant"`，Gemini codec 在 decode/encode 时双向翻译 `"model"` ↔ `"assistant"`。

---

## 3. 字段对照速查表

### `UnifiedRequest`

| IR 字段 | OpenAI | Anthropic | Gemini |
| --- | --- | --- | --- |
| `Model`         | `model`         | `model`             | URL path `models/{model}` |
| `Messages`      | `messages` (剔除 system) | `messages` | `contents` |
| `System`        | `messages[0]` (role=system) | `system`     | `systemInstruction` |
| `MaxTokens`     | `max_tokens` / `max_completion_tokens` | `max_tokens` (必填) | `generationConfig.maxOutputTokens` |
| `Temperature`   | `temperature`   | `temperature`       | `generationConfig.temperature` |
| `TopP`          | `top_p`         | `top_p`             | `generationConfig.topP` |
| `TopK`          | (无)            | `top_k`             | `generationConfig.topK` |
| `Stream`        | `stream`        | `stream`            | URL `:streamGenerateContent` |
| `StopSequences` | `stop`          | `stop_sequences`    | `generationConfig.stopSequences` |
| `N`             | `n`             | (无)                | `generationConfig.candidateCount` |
| `Tools`         | `tools[].function` | `tools`          | `tools[].functionDeclarations` |
| `ToolChoice`    | `tool_choice`   | `tool_choice`       | `toolConfig.functionCallingConfig` |
| `Thinking`      | `reasoning_effort` | `thinking`       | `generationConfig.thinkingConfig` |
| `ResponseFormat`| `response_format` | (无；codec 转 system) | `generationConfig.responseMimeType` + `responseSchema` |
| `Extra`         | logprobs / seed / penalties / user / ... | metadata / mcp_servers / ... | safetySettings / cachedContent / ... |

### `UnifiedResponse`

| IR 字段 | OpenAI | Anthropic | Gemini |
| --- | --- | --- | --- |
| `ID`           | `id`            | `id`              | (无；codec 合成) |
| `Model`        | `model`         | `model`           | (响应不带；从请求传递) |
| `Message`      | `choices[0].message` | `content` (拆成 Parts) | `candidates[0].content` |
| `FinishReason` | `choices[0].finish_reason` | `stop_reason` | `candidates[0].finishReason` |
| `Usage`        | `usage`         | `usage`           | `usageMetadata` |

`FinishReason` 归一值（建议）：

| IR 值 | OpenAI | Anthropic | Gemini |
| --- | --- | --- | --- |
| `"stop"`           | `stop`          | `end_turn`       | `STOP` |
| `"length"`         | `length`        | `max_tokens`     | `MAX_TOKENS` |
| `"tool_use"`       | `tool_calls`    | `tool_use`       | (隐式：parts 含 functionCall) |
| `"content_filter"` | `content_filter`| `refusal`（出站 Anthropic 时） | `SAFETY` |
| `"error"`          | (无)            | (无；走 error envelope) | `OTHER` 等异常 |

### `UnifiedChunk`

| IR 字段 | OpenAI | Anthropic | Gemini |
| --- | --- | --- | --- |
| `ID`           | `id`            | `message_start.message.id` | (无；codec 合成) |
| `Index`        | `choices[].index` | `index` (各事件) | `candidates[0].index` |
| `Delta`        | `choices[0].delta` | 由多事件聚合 | `candidates[0].content` |
| `FinishReason` | `choices[0].finish_reason` | `message_delta.delta.stop_reason` | `candidates[0].finishReason` |
| `Usage`        | 末尾 chunk 的 `usage` | `message_delta.usage` / `message_start.message.usage` | `usageMetadata` |

---

## 4. `Extra` 使用约定

`Extra map[string]any` 出现在 `UnifiedRequest`、`UnifiedResponse`、`UnifiedChunk`、`Message`、`ContentPart` 上，
用来承载**没有进 IR 的协议专属字段**。

### 4.1 何时用 Extra

放进 Extra 的字段需满足以下任一条：

1. 仅一个协议存在，其它协议没有对应概念（如 OpenAI `logprobs`、Gemini `safetySettings`）
2. 罕用 / 实验性字段，提升到 IR 不划算（如 `seed`、`logit_bias`、`web_search_options`）
3. 厂商扩展、非标准字段（如 OpenRouter 的 `extra_body`、各家的 `prompt_cache_*`）

### 4.2 键名规范

**强制**用协议名作为前缀，避免冲突：

```text
openai:logprobs               -> *bool
openai:top_logprobs           -> *int
openai:seed                   -> *int
openai:frequency_penalty      -> *float64
openai:presence_penalty       -> *float64
openai:user                   -> string
openai:logit_bias             -> map[string]int
anthropic:metadata            -> map[string]any
anthropic:mcp_servers         -> json.RawMessage
anthropic:cache_control       -> json.RawMessage   (在 ContentPart.Extra)
anthropic:service_tier        -> string
gemini:safety_settings        -> []SafetySetting   (codec 自定义类型)
gemini:cached_content         -> string
gemini:tool_config            -> json.RawMessage
gemini:response_logprobs      -> *bool
```

### 4.3 跨协议传递规则

- **同协议透传**（OpenAI → OpenAI）：走 raw passthrough 旁路，`Extra` 原样保留（且根本不进 IR）
- **跨协议转换**（OpenAI → Anthropic）：由 lens 链按命名空间过滤，例如挂一个 `drop_extra(prefix_not="anthropic:")` lens 默认丢弃异协议键
- 如果某个 `Extra` 在多个协议都有同义概念，应考虑**升进 IR**而不是用 Extra

### 4.4 流式 chunk 的 Extra

流式场景下 `Extra` 主要用于：

- 标记 chunk 类型（如 `anthropic:event_type` = `"message_start" | "content_block_delta" | ...`）
- 透传计费 / 监控元数据

---

## 5. `Parts` vs `ToolCalls` 不变量

`Message.ToolCalls` 是 `Message.Parts` 中所有 `Type == ContentTypeToolUse` 项的便利镜像。

**Producer 的责任**（codec / lens）：

- 写入 assistant 消息时，**必须**同时维护 Parts 和 ToolCalls 两份，且内容一致
- 工具调用顺序：以 Parts 中的顺序为准（ToolCalls 仅作快速访问）

**Consumer 的便利**：

- 只关心"有没有工具调用"或"工具调用列表" → 读 `ToolCalls`
- 关心"内容块的精确顺序"（text → tool → text 这种交错） → 读 `Parts`

> 未来可能在 `internal/core/types` 下加 `BuildAssistantMessage(...)` 工厂函数把这个 invariant 封起来。

---

## 6. 未决问题

- [ ] `ToolChoice any` 是否进一步抽象为强类型？目前三协议形态分歧太大，先用 `any`，等多 codec 实现后回看
- [ ] Image `detail` 字段（OpenAI low/high/auto）：当前进 `MediaData` 还是 `Extra`？倾向 Extra，但用得多就提升
- [ ] 多 candidate（`N > 1`）的响应：当前 `UnifiedResponse.Message` 只能放一条，是否需要 `Messages []Message`？M1 暂不支持，等 OpenAI N>1 真正用到再扩
- [ ] Anthropic `signature`（thinking block 的签名）：进 `ContentPart.Extra` 还是单独字段？

> 上述问题随实现推进逐项收敛，每个决议追加到 `STATUS.md §3`。
