// Licensed to Alexandre VILAIN under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Alexandre VILAIN licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package tanstackai

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

// collectEvents returns a sink that appends events to the given slice.
func collectEvents(events *[]Event) EventSink {
	return func(event Event) error {
		*events = append(*events, event)
		return nil
	}
}

// sequentialIDGenerator returns a deterministic ID generator for testing.
func sequentialIDGenerator() IDGenerator {
	counters := map[string]int{}
	return func(prefix string) string {
		counters[prefix]++
		return fmt.Sprintf("%s-%d", prefix, counters[prefix])
	}
}

func TestAgent_Run_SimpleTextResponse(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.TextDeltaEvent{Delta: "Hello"})
			_ = sink(provider.TextDeltaEvent{Delta: " world"})
			_ = sink(provider.StreamEndEvent{
				FinishReason: provider.FinishReasonStop,
				Usage:        &provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			})
			return nil
		})

	agent := NewAgent(p, WithIDGenerator(sequentialIDGenerator()))
	var events []Event
	err := agent.Run(context.Background(), RunInput{
		Model:    "test-model",
		Messages: []Message{{Role: RoleUser, Parts: []MessagePart{NewTextPart("hi")}}},
	}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(eventTypes(events)).To(Equal([]EventType{
		EventTypeRunStarted,
		EventTypeTextMessageStart,
		EventTypeTextMessageContent,
		EventTypeTextMessageContent,
		EventTypeTextMessageEnd,
		EventTypeRunFinished,
	}))

	// Verify run started/finished events
	started := events[0].(RunStartedEvent)
	g.Expect(started.ThreadID).To(Equal("thread-1"))
	g.Expect(started.RunID).To(Equal("run-1"))

	finished := events[len(events)-1].(RunFinishedEvent)
	g.Expect(finished.ThreadID).To(Equal("thread-1"))
	g.Expect(finished.RunID).To(Equal("run-1"))
	g.Expect(finished.FinishReason).To(Equal(provider.FinishReasonStop))
	g.Expect(finished.Usage).ToNot(BeNil())
	g.Expect(finished.Usage.PromptTokens).To(Equal(10))
	g.Expect(finished.Usage.CompletionTokens).To(Equal(5))
	g.Expect(finished.Usage.TotalTokens).To(Equal(15))

	// Verify text content events
	content1 := events[2].(TextMessageContentEvent)
	g.Expect(content1.Delta).To(Equal("Hello"))
	content2 := events[3].(TextMessageContentEvent)
	g.Expect(content2.Delta).To(Equal(" world"))
}

func TestAgent_Run_UsesInputThreadAndRunIDs(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop})
			return nil
		})

	agent := NewAgent(p)
	var events []Event
	err := agent.Run(context.Background(), RunInput{
		ThreadID: "my-thread",
		RunID:    "my-run",
	}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())

	started := events[0].(RunStartedEvent)
	g.Expect(started.ThreadID).To(Equal("my-thread"))
	g.Expect(started.RunID).To(Equal("my-run"))

	finished := events[len(events)-1].(RunFinishedEvent)
	g.Expect(finished.ThreadID).To(Equal("my-thread"))
	g.Expect(finished.RunID).To(Equal("my-run"))
}

func TestAgent_Run_SystemPromptPrepended(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	var receivedMessages []Message
	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, messages []Message, _ []Tool, _ string, sink provider.EventSink) error {
			receivedMessages = messages
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop})
			return nil
		})

	agent := NewAgent(p, WithSystemPrompt("You are helpful."))
	err := agent.Run(context.Background(), RunInput{
		Messages: []Message{{Role: RoleUser, Parts: []MessagePart{NewTextPart("hi")}}},
	}, func(_ Event) error { return nil })

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(receivedMessages).To(HaveLen(2))
	g.Expect(receivedMessages[0].Role).To(Equal(RoleSystem))
	g.Expect(receivedMessages[0].TextContent()).To(Equal("You are helpful."))
	g.Expect(receivedMessages[1].Role).To(Equal(RoleUser))
	g.Expect(receivedMessages[1].TextContent()).To(Equal("hi"))
}

func TestAgent_Run_ToolCallExecution(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	callIndex := 0
	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			defer func() { callIndex++ }()
			if callIndex == 0 {
				_ = sink(provider.ToolCallStartEvent{ToolCallID: "tc-1", ToolName: "greet"})
				_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `{"name":`})
				_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `"Alice"}`})
				_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonToolCalls})
				return nil
			}
			_ = sink(provider.TextDeltaEvent{Delta: "Done"})
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop})
			return nil
		}).Times(2)

	greetTool := Tool{
		Name: "greet",
		Execute: func(_ context.Context, args map[string]any) (any, error) {
			return map[string]string{"greeting": fmt.Sprintf("Hello, %s!", args["name"])}, nil
		},
	}

	agent := NewAgent(p,
		WithTools(greetTool),
		WithIDGenerator(sequentialIDGenerator()),
	)
	var events []Event
	err := agent.Run(context.Background(), RunInput{Model: "test"}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())
	p.AssertNumberOfCalls(t, "ChatStream", 2)

	types := eventTypes(events)
	g.Expect(types).To(Equal([]EventType{
		EventTypeRunStarted,
		// Iteration 1: tool call
		EventTypeToolCallStart,
		EventTypeToolCallArgs,
		EventTypeToolCallArgs,
		EventTypeToolCallEnd,
		// Iteration 2: text response
		EventTypeTextMessageStart,
		EventTypeTextMessageContent,
		EventTypeTextMessageEnd,
		EventTypeRunFinished,
	}))

	// Verify tool call end has result
	toolEnd := events[4].(ToolCallEndEvent)
	g.Expect(toolEnd.ToolCallID).To(Equal("tc-1"))
	g.Expect(toolEnd.ToolName).To(Equal("greet"))
	g.Expect(toolEnd.Result).To(Equal(`{"greeting":"Hello, Alice!"}`))
}

func TestAgent_Run_NoServerSideToolsStopsLoop(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.ToolCallStartEvent{ToolCallID: "tc-1", ToolName: "client_tool"})
			_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `{}`})
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonToolCalls})
			return nil
		})

	// No tools registered — model emits tool calls but no server-side execution is available
	agent := NewAgent(p, WithIDGenerator(sequentialIDGenerator()))
	var events []Event
	err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())
	// Should only call provider once — loop stops because finish reason becomes "stop"
	p.AssertNumberOfCalls(t, "ChatStream", 1)

	types := eventTypes(events)
	g.Expect(types).To(Equal([]EventType{
		EventTypeRunStarted,
		EventTypeToolCallStart,
		EventTypeToolCallArgs,
		EventTypeToolCallEnd,
		EventTypeRunFinished,
	}))

	// ToolCallEnd should have no result when no server-side tools are registered
	toolEnd := events[3].(ToolCallEndEvent)
	g.Expect(toolEnd.Result).To(BeEmpty())
}

func TestAgent_Run_MaxIterations(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.ToolCallStartEvent{ToolCallID: "tc-1", ToolName: "loop_tool"})
			_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `{}`})
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonToolCalls})
			return nil
		}).Times(3)

	loopTool := Tool{
		Name: "loop_tool",
		Execute: func(_ context.Context, _ map[string]any) (any, error) {
			return "ok", nil
		},
	}

	agent := NewAgent(p,
		WithTools(loopTool),
		WithStrategy(MaxIterations(3)),
		WithIDGenerator(sequentialIDGenerator()),
	)
	var events []Event
	err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())
	p.AssertNumberOfCalls(t, "ChatStream", 3)
}

func TestAgent_Run_ProviderError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	providerErr := errors.New("provider failure")
	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(providerErr)

	agent := NewAgent(p, WithIDGenerator(sequentialIDGenerator()))
	var events []Event
	err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("provider failure"))

	// Should have RUN_STARTED and RUN_ERROR
	types := eventTypes(events)
	g.Expect(types).To(Equal([]EventType{
		EventTypeRunStarted,
		EventTypeRunError,
	}))

	runErr := events[1].(RunErrorEvent)
	g.Expect(runErr.Error.Code).To(Equal("internal_error"))
}

func TestAgent_Run_ContextCancellation(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	ctx, cancel := context.WithCancel(context.Background())
	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.ToolCallStartEvent{ToolCallID: "tc-1", ToolName: "tool"})
			_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `{}`})
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonToolCalls})
			// Cancel after first iteration
			cancel()
			return nil
		})

	tool := Tool{
		Name: "tool",
		Execute: func(_ context.Context, _ map[string]any) (any, error) {
			return "ok", nil
		},
	}

	agent := NewAgent(p,
		WithTools(tool),
		WithIDGenerator(sequentialIDGenerator()),
	)
	var events []Event
	err := agent.Run(ctx, RunInput{}, collectEvents(&events))

	g.Expect(err).To(MatchError(context.Canceled))

	// Should have RUN_ERROR with canceled code
	lastEvent := events[len(events)-1]
	runErr, ok := lastEvent.(RunErrorEvent)
	g.Expect(ok).To(BeTrue())
	g.Expect(runErr.Error.Code).To(Equal("canceled"))
}

func TestAgent_Run_Hooks(t *testing.T) {
	t.Parallel()

	t.Run("OnRunStart is called", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var hookThreadID, hookRunID string
		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
				_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop})
				return nil
			})

		agent := NewAgent(p,
			WithIDGenerator(sequentialIDGenerator()),
			WithHooks(Hooks{
				OnRunStart: func(_ context.Context, threadID, runID string) error {
					hookThreadID = threadID
					hookRunID = runID
					return nil
				},
			}),
		)
		err := agent.Run(context.Background(), RunInput{}, func(_ Event) error { return nil })

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(hookThreadID).To(Equal("thread-1"))
		g.Expect(hookRunID).To(Equal("run-1"))
	})

	t.Run("OnRunStart error stops run", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		hookErr := errors.New("hook failed")
		p := NewMockProvider(t)

		agent := NewAgent(p, WithHooks(Hooks{
			OnRunStart: func(_ context.Context, _, _ string) error { return hookErr },
		}))
		err := agent.Run(context.Background(), RunInput{}, func(_ Event) error { return nil })

		g.Expect(err).To(MatchError(hookErr))
	})

	t.Run("OnStepFinish receives step data", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var stepResults []StepResult
		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
				_ = sink(provider.TextDeltaEvent{Delta: "Hi"})
				_ = sink(provider.StreamEndEvent{
					FinishReason: provider.FinishReasonStop,
					Usage:        &provider.Usage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6},
				})
				return nil
			})

		agent := NewAgent(p,
			WithIDGenerator(sequentialIDGenerator()),
			WithHooks(Hooks{
				OnStepFinish: func(_ context.Context, step StepResult) error {
					stepResults = append(stepResults, step)
					return nil
				},
			}),
		)
		err := agent.Run(context.Background(), RunInput{}, func(_ Event) error { return nil })

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stepResults).To(HaveLen(1))
		g.Expect(stepResults[0].FinishReason).To(Equal(provider.FinishReasonStop))
		g.Expect(stepResults[0].Usage).ToNot(BeNil())
		g.Expect(stepResults[0].Usage.PromptTokens).To(Equal(5))
		g.Expect(stepResults[0].Messages).To(HaveLen(1))
		g.Expect(stepResults[0].Messages[0].Role).To(Equal(RoleAssistant))
	})

	t.Run("OnStepFinish error stops run", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		hookErr := errors.New("step hook failed")
		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
				_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop})
				return nil
			})

		agent := NewAgent(p, WithHooks(Hooks{
			OnStepFinish: func(_ context.Context, _ StepResult) error { return hookErr },
		}))
		var events []Event
		err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

		g.Expect(err).To(MatchError(hookErr))
		// Should emit RUN_ERROR
		g.Expect(eventTypes(events)).To(ContainElement(EventTypeRunError))
	})

	t.Run("OnFinish receives run result", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var runResult RunResult
		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
				_ = sink(provider.TextDeltaEvent{Delta: "result"})
				_ = sink(provider.StreamEndEvent{
					FinishReason: provider.FinishReasonStop,
					Usage:        &provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				})
				return nil
			})

		agent := NewAgent(p,
			WithIDGenerator(sequentialIDGenerator()),
			WithHooks(Hooks{
				OnFinish: func(_ context.Context, result RunResult) error {
					runResult = result
					return nil
				},
			}),
		)
		err := agent.Run(context.Background(), RunInput{
			Messages: []Message{{Role: RoleUser, Parts: []MessagePart{NewTextPart("hi")}}},
		}, func(_ Event) error { return nil })

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(runResult.ThreadID).To(Equal("thread-1"))
		g.Expect(runResult.RunID).To(Equal("run-1"))
		g.Expect(runResult.FinishReason).To(Equal(provider.FinishReasonStop))
		g.Expect(runResult.Usage).ToNot(BeNil())
		g.Expect(runResult.Usage.TotalTokens).To(Equal(15))
		// Messages should include user message + assistant message
		g.Expect(len(runResult.Messages)).To(BeNumerically(">=", 2))
	})

	t.Run("OnError is called on provider error", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		var hookErr error
		providerErr := errors.New("provider broke")
		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(providerErr)

		agent := NewAgent(p, WithHooks(Hooks{
			OnError: func(_ context.Context, err error) error {
				hookErr = err
				return nil
			},
		}))
		err := agent.Run(context.Background(), RunInput{}, func(_ Event) error { return nil })

		g.Expect(err).To(HaveOccurred())
		g.Expect(hookErr).To(HaveOccurred())
		g.Expect(hookErr.Error()).To(ContainSubstring("provider broke"))
	})

	t.Run("OnError hook error wraps both errors", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		p := NewMockProvider(t)
		p.EXPECT().
			ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(errors.New("original error"))

		agent := NewAgent(p, WithHooks(Hooks{
			OnError: func(_ context.Context, _ error) error {
				return errors.New("hook error")
			},
		}))
		err := agent.Run(context.Background(), RunInput{}, func(_ Event) error { return nil })

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("original error"))
		g.Expect(err.Error()).To(ContainSubstring("hook error"))
	})
}

func TestAgent_Run_SinkError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)

	sinkErr := errors.New("sink broken")
	agent := NewAgent(p)
	err := agent.Run(context.Background(), RunInput{}, func(_ Event) error {
		return sinkErr
	})

	// RUN_STARTED sink error should propagate
	g.Expect(err).To(MatchError(sinkErr))
}

func TestAgent_Run_UsageAccumulation(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	callIndex := 0
	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			defer func() { callIndex++ }()
			if callIndex == 0 {
				_ = sink(provider.ToolCallStartEvent{ToolCallID: "tc-1", ToolName: "tool"})
				_ = sink(provider.ToolCallDeltaEvent{ToolCallID: "tc-1", ArgsDelta: `{}`})
				_ = sink(provider.StreamEndEvent{
					FinishReason: provider.FinishReasonToolCalls,
					Usage:        &provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
				})
				return nil
			}
			_ = sink(provider.TextDeltaEvent{Delta: "done"})
			_ = sink(provider.StreamEndEvent{
				FinishReason: provider.FinishReasonStop,
				Usage:        &provider.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
			})
			return nil
		}).Times(2)

	tool := Tool{
		Name: "tool",
		Execute: func(_ context.Context, _ map[string]any) (any, error) {
			return "ok", nil
		},
	}

	agent := NewAgent(p,
		WithTools(tool),
		WithIDGenerator(sequentialIDGenerator()),
	)
	var events []Event
	err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())

	finished := events[len(events)-1].(RunFinishedEvent)
	g.Expect(finished.Usage).ToNot(BeNil())
	g.Expect(finished.Usage.PromptTokens).To(Equal(30))
	g.Expect(finished.Usage.CompletionTokens).To(Equal(15))
	g.Expect(finished.Usage.TotalTokens).To(Equal(45))
}

func TestAgent_Run_NoUsageWhenNilFromProvider(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	p := NewMockProvider(t)
	p.EXPECT().
		ChatStream(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
			_ = sink(provider.StreamEndEvent{FinishReason: provider.FinishReasonStop, Usage: nil})
			return nil
		})

	agent := NewAgent(p, WithIDGenerator(sequentialIDGenerator()))
	var events []Event
	err := agent.Run(context.Background(), RunInput{}, collectEvents(&events))

	g.Expect(err).ToNot(HaveOccurred())
	finished := events[len(events)-1].(RunFinishedEvent)
	g.Expect(finished.Usage).To(BeNil())
}

func TestRunErrorCode(t *testing.T) {
	tests := map[string]struct {
		err      error
		expected string
	}{
		"context canceled": {
			err:      context.Canceled,
			expected: "canceled",
		},
		"deadline exceeded": {
			err:      context.DeadlineExceeded,
			expected: "deadline_exceeded",
		},
		"wrapped canceled": {
			err:      fmt.Errorf("operation failed: %w", context.Canceled),
			expected: "canceled",
		},
		"generic error": {
			err:      errors.New("something went wrong"),
			expected: "internal_error",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			g.Expect(runErrorCode(test.err)).To(Equal(test.expected))
		})
	}
}

func TestPrependSystemPrompts(t *testing.T) {
	tests := map[string]struct {
		messages  []Message
		prompt    string
		expected  int // expected message count
		hasSystem bool
	}{
		"empty prompt returns original": {
			messages:  []Message{{Role: RoleUser}},
			prompt:    "",
			expected:  1,
			hasSystem: false,
		},
		"non-empty prompt prepends system message": {
			messages:  []Message{{Role: RoleUser}},
			prompt:    "Be helpful",
			expected:  2,
			hasSystem: true,
		},
		"empty messages with prompt": {
			messages:  nil,
			prompt:    "Be helpful",
			expected:  1,
			hasSystem: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			result := prependSystemPrompts(test.messages, test.prompt)
			g.Expect(result).To(HaveLen(test.expected))
			if test.hasSystem {
				g.Expect(result[0].Role).To(Equal(RoleSystem))
				g.Expect(result[0].TextContent()).To(Equal(test.prompt))
			}
		})
	}
}

func TestAccumulatedUsage(t *testing.T) {
	t.Parallel()

	t.Run("add nil usage is no-op", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		u := &accumulatedUsage{}
		u.add(nil)
		g.Expect(u.toUsage()).To(BeNil())
	})

	t.Run("accumulates multiple usages", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		u := &accumulatedUsage{}
		u.add(&provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
		u.add(&provider.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30})

		result := u.toUsage()
		g.Expect(result).ToNot(BeNil())
		g.Expect(result.PromptTokens).To(Equal(30))
		g.Expect(result.CompletionTokens).To(Equal(15))
		g.Expect(result.TotalTokens).To(Equal(45))
	})

	t.Run("zero usage returns nil", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)

		u := &accumulatedUsage{}
		g.Expect(u.toUsage()).To(BeNil())
	})
}

func TestConvertProviderUsage(t *testing.T) {
	tests := map[string]struct {
		input    *provider.Usage
		expected *Usage
	}{
		"nil returns nil": {
			input:    nil,
			expected: nil,
		},
		"converts usage fields": {
			input:    &provider.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			expected: &Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			result := convertProviderUsage(test.input)
			g.Expect(result).To(Equal(test.expected))
		})
	}
}

// eventTypes extracts EventType from a slice of events.
func eventTypes(events []Event) []EventType {
	types := make([]EventType, len(events))
	for i, e := range events {
		types[i] = e.EventType()
	}
	return types
}
