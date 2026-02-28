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
	"strings"

	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

// Agent orchestrates the agentic loop with automatic tool execution.
// It is safe for concurrent use — all per-request state lives in Run().
type Agent struct {
	provider     Provider
	systemPrompt string
	tools        []Tool
	strategy     LoopStrategy
	hooks        Hooks
	idGenerator  IDGenerator
}

// NewAgent creates an Agent with the given provider and options.
func NewAgent(p Provider, opts ...Option) *Agent {
	a := &Agent{
		provider: p,
		strategy: CombineStrategies(
			MaxIterations(5),
			UntilFinishReason(
				provider.FinishReasonStop,
				provider.FinishReasonLength,
				provider.FinishReasonContentFilter,
				provider.FinishReasonUnknown,
			),
		),
		idGenerator: DefaultIDGenerator,
	}

	for _, o := range opts {
		o(a)
	}

	return a
}

// Run executes the full agentic loop, emitting all AG-UI events to the sink.
func (a *Agent) Run(ctx context.Context, input RunInput, sink EventSink) error {
	threadID := input.ThreadID
	if threadID == "" {
		threadID = a.idGenerator("thread")
	}
	runID := input.RunID
	if runID == "" {
		runID = a.idGenerator("run")
	}

	// Hook: OnRunStart
	if a.hooks.OnRunStart != nil {
		if err := a.hooks.OnRunStart(ctx, threadID, runID); err != nil {
			return err
		}
	}

	// Emit RUN_STARTED
	if err := sink(NewRunStartedEvent(threadID, runID)); err != nil {
		return err
	}

	// All per-request state is local
	messages := prependSystemPrompts(input.Messages, a.systemPrompt)
	toolMgr := NewToolCallManager()
	var iterationCount int
	var lastFinishReason provider.FinishReason
	totalUsage := &accumulatedUsage{}

	// Agentic loop
	var loopErr error
	for a.strategy(LoopState{
		IterationCount: iterationCount,
		Messages:       messages,
		FinishReason:   lastFinishReason,
	}) {
		if err := ctx.Err(); err != nil {
			loopErr = err
			break
		}

		// Phase 1: Process model response
		var (
			finishReason provider.FinishReason
			stepUsage    *provider.Usage
			newMessages  []Message
		)
		finishReason, stepUsage, newMessages, loopErr = a.processChat(ctx, input.Model, messages, a.tools, toolMgr, sink)
		if loopErr != nil {
			break
		}

		messages = append(messages, newMessages...)
		lastFinishReason = finishReason
		totalUsage.add(stepUsage)

		// Phase 2: Execute any tool calls
		var (
			toolResults  []ToolResult
			toolMessages []Message
		)
		toolMessages, toolResults, loopErr = a.executeToolCalls(ctx, toolMgr, a.tools, lastFinishReason, sink)
		if loopErr != nil {
			break
		}
		if len(toolMessages) > 0 {
			messages = append(messages, toolMessages...)
		} else if lastFinishReason == provider.FinishReasonToolCalls {
			// Tools were present but none were server-side; treat as stop.
			lastFinishReason = provider.FinishReasonStop
		}

		// Hook: OnStepFinish
		if a.hooks.OnStepFinish != nil {
			stepMessages := []Message{}
			stepMessages = append(stepMessages, newMessages...)
			stepMessages = append(stepMessages, toolMessages...)

			stepResult := StepResult{
				Messages:     stepMessages,
				Usage:        convertProviderUsage(stepUsage),
				FinishReason: finishReason,
				ToolCalls:    toolMgr.GetToolCalls(),
				ToolResults:  toolResults,
			}
			if err := a.hooks.OnStepFinish(ctx, stepResult); err != nil {
				loopErr = err
				break
			}
		}

		iterationCount++
	}

	if loopErr != nil {
		// Hook: OnError
		if a.hooks.OnError != nil {
			if hookErr := a.hooks.OnError(ctx, loopErr); hookErr != nil {
				return fmt.Errorf("run failed: %w; on error hook: %v", loopErr, hookErr)
			}
		}

		_ = sink(NewRunErrorEvent(loopErr.Error(), runErrorCode(loopErr), runID))
		return loopErr
	}

	// Emit RUN_FINISHED with accumulated usage
	finishedEvent := NewRunFinishedEvent(threadID, runID, lastFinishReason)
	finishedEvent.Usage = totalUsage.toUsage()
	if err := sink(finishedEvent); err != nil {
		return err
	}

	// Hook: OnFinish
	if a.hooks.OnFinish != nil {
		return a.hooks.OnFinish(ctx, RunResult{
			Messages:     messages,
			Usage:        totalUsage.toUsage(),
			FinishReason: lastFinishReason,
			ThreadID:     threadID,
			RunID:        runID,
		})
	}

	return nil
}

func runErrorCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "internal_error"
	}
}

// processChat streams the model response, translating provider events into AG-UI events.
// It returns the finish reason, usage, and any new messages to append to conversation history.
func (a *Agent) processChat(ctx context.Context, model string, messages []Message, tools []Tool, toolMgr *ToolCallManager, outerSink EventSink) (provider.FinishReason, *provider.Usage, []Message, error) {
	toolMgr.Clear()

	var contentBuilder strings.Builder
	messageID := a.idGenerator("msg")
	textStarted := false
	var finishReason provider.FinishReason
	var usage *provider.Usage

	err := a.provider.ChatStream(ctx, messages, tools, model, func(event provider.Event) error {
		switch ev := event.(type) {
		case provider.TextDeltaEvent:
			if !textStarted {
				textStarted = true
				if err := outerSink(NewTextMessageStartEvent(messageID, "assistant")); err != nil {
					return err
				}
			}
			contentBuilder.WriteString(ev.Delta)
			return outerSink(NewTextMessageContentEvent(messageID, ev.Delta))

		case provider.ToolCallStartEvent:
			toolMgr.AddStart(ev.ToolCallID, ev.ToolName)
			return outerSink(NewToolCallStartEvent(ev.ToolCallID, ev.ToolName))

		case provider.ToolCallDeltaEvent:
			toolMgr.AddArgs(ev.ToolCallID, ev.ArgsDelta)
			return outerSink(NewToolCallArgsEvent(ev.ToolCallID, ev.ArgsDelta))

		case provider.StreamEndEvent:
			finishReason = ev.FinishReason
			usage = ev.Usage
			if textStarted {
				return outerSink(NewTextMessageEndEvent(messageID))
			}
		}
		return nil
	})
	if err != nil {
		return "", nil, nil, fmt.Errorf("provider chat stream: %w", err)
	}

	// Build assistant message from accumulated content and tool calls
	var newMessages []Message
	accumulatedContent := contentBuilder.String()
	parts := []MessagePart{}
	if accumulatedContent != "" {
		parts = append(parts, NewTextPart(accumulatedContent))
	}
	toolCalls := toolMgr.GetToolCalls()
	for _, tc := range toolCalls {
		parts = append(parts, NewToolCallPart(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	if len(parts) > 0 {
		newMessages = append(newMessages, Message{
			Role:  RoleAssistant,
			Parts: parts,
		})
	}

	return finishReason, usage, newMessages, nil
}

// executeToolCalls handles tool execution after the model response.
// It returns new tool-result messages and the tool results.
func (a *Agent) executeToolCalls(ctx context.Context, toolMgr *ToolCallManager, tools []Tool, finishReason provider.FinishReason, sink EventSink) ([]Message, []ToolResult, error) {
	if !toolMgr.HasToolCalls() {
		return nil, nil, nil
	}

	toolCalls := toolMgr.GetToolCalls()

	// If no server-side tools registered or finish reason isn't tool_calls,
	// emit TOOL_CALL_END without result (client-side tools) and stop.
	if len(tools) == 0 || finishReason != provider.FinishReasonToolCalls {
		for _, tc := range toolCalls {
			if err := sink(NewToolCallEndEvent(tc.ID, tc.Function.Name)); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	}

	// Execute tools and emit TOOL_CALL_END with results
	var newMessages []Message
	results := ExecuteToolCalls(ctx, toolCalls, tools)
	for _, result := range results {
		endEvent := NewToolCallEndEvent(result.ToolCallID, result.ToolName)
		endEvent.Result = result.Content
		if err := sink(endEvent); err != nil {
			return nil, nil, err
		}

		newMessages = append(newMessages, Message{
			Role: RoleTool,
			Parts: []MessagePart{
				NewToolResultPart(result.ToolCallID, result.Content),
			},
		})
	}

	return newMessages, results, nil
}

func prependSystemPrompts(messages []Message, prompt string) []Message {
	if prompt == "" {
		return messages
	}
	return append([]Message{
		{
			Role:  RoleSystem,
			Parts: []MessagePart{NewTextPart(prompt)},
		},
	}, messages...)
}

// accumulatedUsage tracks total usage across multiple model calls.
type accumulatedUsage struct {
	promptTokens     int
	completionTokens int
	totalTokens      int
}

func (u *accumulatedUsage) add(other *provider.Usage) {
	if other == nil {
		return
	}
	u.promptTokens += other.PromptTokens
	u.completionTokens += other.CompletionTokens
	u.totalTokens += other.TotalTokens
}

func (u *accumulatedUsage) toUsage() *Usage {
	if u.promptTokens == 0 && u.completionTokens == 0 && u.totalTokens == 0 {
		return nil
	}
	return &Usage{
		PromptTokens:     u.promptTokens,
		CompletionTokens: u.completionTokens,
		TotalTokens:      u.totalTokens,
	}
}

// convertProviderUsage converts provider-level usage to the public Usage type.
func convertProviderUsage(u *provider.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
