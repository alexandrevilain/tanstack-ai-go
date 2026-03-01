# tanstack-ai-go

Go library for building AI agent backends that stream to TanStack AI frontends using the [AG-UI protocol](https://docs.ag-ui.com).

## Installation

```bash
go get github.com/alexandrevilain/tanstack-ai-go
```

## Quick Start

```go
package main

import (
    "context"
    "net/http"

    tanstackai "github.com/alexandrevilain/tanstack-ai-go"
    tsopenai "github.com/alexandrevilain/tanstack-ai-go/openai"
)

func main() {
    provider := tsopenai.NewProvider()

    agent := tanstackai.NewAgent(provider,
        tanstackai.WithTools(tanstackai.Tool{
            Name:        "get_weather",
            Description: "Get the current weather for a location",
            InputSchema: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "location": map[string]any{
                        "type":        "string",
                        "description": "City and state, e.g. San Francisco, CA",
                    },
                },
                "required": []string{"location"},
            },
            Execute: func(ctx context.Context, args map[string]any) (any, error) {
                location := args["location"].(string)
                return map[string]any{
                    "location":    location,
                    "temperature": 72,
                    "conditions":  "sunny",
                }, nil
            },
        }),
    )

    handler := tanstackai.NewHandler(agent, func(r *http.Request) (tanstackai.RunInput, error) {
        input, _ := tanstackai.DefaultRequestDecoder(r)
        input.Model = "gpt-4o"
        return input, nil
    })

    http.Handle("POST /api/chat", handler)
    http.ListenAndServe(":8080", nil)
}
```

## How It Works

The library implements an agentic loop that alternates between streaming model responses and executing tool calls. It handles the AG-UI protocol details so you can focus on defining tools and business logic.

**Agent**: Orchestrates the conversation loop, manages tool execution, and streams SSE events to clients.

**Provider**: Adapts LLM provider SDKs (OpenAI, etc.) to the library's event interface. Currently supports OpenAI via `github.com/alexandrevilain/tanstack-ai-go/openai`.

**Tools**: Server-side functions the model can invoke. Set the `Execute` field to handle tool calls automatically, or omit it for client-side execution.

**Options**: Configure behavior with functional options like `WithTools`, `WithSystemPrompt`, `WithMaxIterations`, `WithStrategy`, and `WithHooks`.

## Configuration

```go
agent := tanstackai.NewAgent(provider,
    tanstackai.WithSystemPrompt("You are a helpful assistant."),
    tanstackai.WithStrategy(tanstackai.CombineStrategies(
        tanstackai.MaxIterations(10),
        tanstackai.UntilFinishReason(provider.FinishReasonStop),
    )),
    tanstackai.WithHooks(tanstackai.Hooks{
        OnStepFinish: func(result tanstackai.StepResult) error {
            // Save messages, log usage, etc.
            return nil
        },
    }),
)
```

## Providers

### OpenAI

```go
import tsopenai "github.com/alexandrevilain/tanstack-ai-go/openai"

provider := tsopenai.NewProvider()  // Uses OPENAI_API_KEY env var
```

Additional providers can be implemented by satisfying the `Provider` interface in `provider.go`.

## HTTP Handler

The `NewHandler` function creates an HTTP handler that wires up SSE streaming:

```go
handler := tanstackai.NewHandler(agent, func(r *http.Request) (tanstackai.RunInput, error) {
    input, err := tanstackai.DefaultRequestDecoder(r)
    if err != nil {
        return input, err
    }
    // Customize input (set default model, inject context, etc.)
    if input.Model == "" {
        input.Model = "gpt-4o"
    }
    return input, nil
})
```

## Hooks

Hooks provide lifecycle callbacks for persistence and monitoring:

```go
tanstackai.WithHooks(tanstackai.Hooks{
    OnRunStart: func(input tanstackai.RunInput) error {
        // Initialize run
        return nil
    },
    OnStepFinish: func(result tanstackai.StepResult) error {
        // Save messages after each agent step
        return nil
    },
    OnFinish: func(result tanstackai.RunResult) error {
        // Finalize run, save total usage
        return nil
    },
    OnError: func(err error) {
        // Log errors
    },
})
```

## Examples

See `examples/openai-chat/` for a complete working example with a frontend.

## License

Apache 2.0
