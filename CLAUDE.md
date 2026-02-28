# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go library implementing the [AG-UI protocol](https://docs.ag-ui.com) (Agent User Interaction protocol) for building AI agent backends that stream SSE events to TanStack AI frontends. The module path is `github.com/alexandrevilain/tanstack-ai-go`.

## Common Commands

```bash
go build ./...        # Build all packages
go test ./...         # Run all tests
go test ./openai/     # Run tests for a specific package
go vet ./...          # Run static analysis
make lint             # Run golangci-lint
make generate         # Generate mocks (mockery) and go generate
make ensure-license   # Add Apache 2.0 license headers to all files
make check-license    # Verify license headers are present
```

## Architecture

The library follows a provider pattern with an agentic loop agent:

- **`Provider` interface** (`provider.go`): Provider-agnostic interface with a single `ChatStream` method. Providers convert provider-specific streaming responses into provider-level events. Providers must NOT emit run lifecycle events (`RUN_STARTED`/`RUN_FINISHED`).
- **`Agent`** (`agent.go`): Orchestrates the agentic loop with two alternating phases: `processChat` (stream model response) and `executeToolCalls` (execute tool calls and append results to conversation history). Manages run lifecycle events, usage accumulation, and hooks invocation.
- **`EventSink`** (`provider.go`): `func(Event) error` callback pattern used throughout for streaming events. Returning an error stops the stream.
- **`Option`** (`options.go`): Functional options pattern (`func(*Agent)`) for configuring `Agent` at creation time or overriding defaults per `Run()` call. Options directly modify agent fields. Includes `WithSystemPrompt`, `WithTools`, `WithStrategy`, `WithHooks`, `WithIDGenerator`.
- **`IDGenerator`** (`options.go`): `func(prefix string) string` for generating thread, run, and message IDs. `DefaultIDGenerator` produces `prefix-<short-uuid>`. Customizable via `WithIDGenerator`.
- **`Hooks`** (`hooks.go`): Lifecycle callbacks (`OnRunStart`, `OnStepFinish`, `OnFinish`, `OnError`) with `StepResult` and `RunResult` types providing accumulated messages and usage data.
- **Events** (`events.go`): Typed AG-UI protocol events (run, text message, tool call) with JSON serialization and constructor functions (`New*Event`).
- **`LoopStrategy`** (`strategy.go`): Composable functions controlling when the agentic loop stops (e.g., `MaxIterations`, `UntilFinishReason`, `CombineStrategies`).
- **`ToolCallManager`** (`tools.go`): Accumulates streaming tool call deltas, then executes tools against registered `Tool` definitions.
- **HTTP handler** (`handler.go`): `NewHandler` creates an `http.Handler` that wires up SSE streaming. Uses a `RequestDecoder` function to extract `RunInput` from requests.

### Provider Implementations

- **`openai/`**: OpenAI provider using `github.com/openai/openai-go`. Converts between `Message`/`Tool` and OpenAI SDK types (`messages.go`), streams completions with usage tracking and maps finish reasons (`provider.go`).

### Provider Events Package

- **`provider/`**: Defines provider-level event types (`TextDeltaEvent`, `ToolCallStartEvent`, `ToolCallDeltaEvent`, `StreamEndEvent`), `EventSink`, `FinishReason`, and `Usage`. This is the contract between the agent and provider implementations.

### Example

- **`examples/openai-chat/`**: Full example with Go backend and TypeScript/Vite frontend using TanStack AI client (separate npm project, not part of the Go module).

## Testing Conventions

- Use `github.com/onsi/gomega` for assertions, imported with dot (`. "github.com/onsi/gomega"`).
- Use table-driven tests with `map[string]struct{...}` for test cases.
- Each subtest should call `t.Parallel()` and use `g := NewWithT(t)` for assertions.
- Follow this pattern:

```go
tests := map[string]struct {
    input  string
    result string
}{
    "case name": {
        input:  "value",
        result: "expected",
    },
}

for name, test := range tests {
    t.Run(name, func(t *testing.T) {
        t.Parallel()
        g := NewWithT(t)
        result := doSomething(test.input)
        g.Expect(result).To(Equal(test.result))
    })
}
```

## Key Conventions

- All Go files must have Apache 2.0 license headers (enforced by CI). Run `make ensure-license` after creating new files.

- The package name is `tanstackai` (imported as `tanstackai "github.com/alexandrevilain/tanstack-ai-go"`).
- Provider implementations live in sub-packages (e.g., `openai/`) and are imported with aliases (e.g., `tsopenai`).
- Tools have an optional `Execute` field — when set, the agent executes them server-side; when nil, they are client-side only.
- `Agent` is created with `NewAgent(provider, ...Option)` using functional options. The same `Option` type works for per-request overrides in `Run()`.
- `RunInput` carries request data (model, messages, threadID, runID).
- Hooks (`OnFinish`, `OnStepFinish`) provide accumulated messages and usage, enabling persistence and monitoring without coupling to storage.
