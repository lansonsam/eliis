# Eliis · Embed API

> 当前文档记录 `pkg/embed` 的最小库调用入口。
> 这不是 HTTP 网关，也不会真实请求上游模型；它只做协议请求 JSON 转义。

---

## 1. 当前能力

已支持：

- OpenAI Chat Completions 请求 JSON → Anthropic Messages 请求 JSON
- 同协议输入输出时 raw passthrough：原始 JSON 字节复制返回
- `output` 支持 `anthropic`、`claude`、`a`、`a社` 作为 Anthropic 协议别名
- 首批 request lens：`OverrideModel`、`EnsureMaxTokens`

尚未支持（仅指 `pkg/embed` 库入口）：

- 真实上游调用（HTTP 网关已支持 `/v1/messages` → OpenAI-compatible 非流式上游；`pkg/embed` 仍只做本地 JSON 转义）
- 响应 JSON 转义
- 流式 chunk 转义
- Gemini 路径

---

## 2. 最小调用

```go
package main

import (
    "fmt"

    "github.com/lansonsam/eliis/pkg/embed"
)

func main() {
    input := []byte(`{
      "model": "gpt-placeholder",
      "messages": [
        {"role": "system", "content": "You are concise."},
        {"role": "user", "content": "Explain Eliis in one sentence."}
      ],
      "max_completion_tokens": 1024
    }`)

    output, err := embed.TranslateJSON(embed.Route{
        Input:            embed.ProtocolOpenAI,
        Output:           "a社",
        OutputModel:      "claude-placeholder",
        DefaultMaxTokens: 1024,
    }, input)
    if err != nil {
        panic(err)
    }

    fmt.Println(string(output))
}
```

---

## 3. 当前内部链路

```text
pkg/embed.TranslateJSON
  -> openai.DecodeChatCompletionRequest
  -> LensChain(OverrideModel, EnsureMaxTokens)
  -> anthropic.EncodeMessagesRequest
  -> JSON bytes
```

这个链路对应 `DESIGN.md` 的三正交模型：

- `Codec`：OpenAI / Anthropic 协议 JSON 与 IR 互转
- `Lens`：对 IR 做目标协议所需的小变换
- `Backend`：尚未接入，后续负责真实 HTTP / SDK 调用

---

## 4. HTTP 网关 smoke test

`cmd/eliis` 当前还提供一个最小 Anthropic Messages 非流式入口：

```text
POST /v1/messages
  -> Anthropic Messages 请求
  -> OpenAI-compatible /v1/chat/completions 上游
  -> Anthropic Messages 响应
```

配置 `configs/eliis.yaml` 中的 `upstreams.openai` 与 `routes.anthropic_messages`，设置环境变量后启动：

```powershell
$env:OPENAI_API_KEY="sk-..."
go run ./cmd/eliis -config configs/eliis.yaml
```

然后可用 Anthropic Messages 形状请求本地网关：

```powershell
Invoke-RestMethod `
  -Uri http://127.0.0.1:8090/v1/messages `
  -Method POST `
  -Headers @{
    "content-type" = "application/json"
    "anthropic-version" = "2023-06-01"
    "x-api-key" = "dummy"
  } `
  -Body '{
    "model": "claude-opus-4-8",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Say hello."}]
  }'
```

当前限制：`stream:true` 会明确返回 `invalid_request_error`，流式 SSE 后续单独实现。

## 5. 验证

```powershell
go test ./...
```

如需临时手动验证 `pkg/embed`，可在仓库外或本地临时文件中 import `github.com/lansonsam/eliis/pkg/embed` 调用；不要把一次性验证文件长期留在仓库根目录。
