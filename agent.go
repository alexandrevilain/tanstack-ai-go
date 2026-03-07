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
	r := &agentRun{
		agent:      a,
		ctx:        ctx,
		sink:       sink,
		model:      input.Model,
		messages:   prependSystemPrompts(input.Messages, a.systemPrompt),
		toolMgr:    NewToolCallManager(),
		totalUsage: &accumulatedUsage{},
	}

	r.threadID = input.ThreadID
	if r.threadID == "" {
		r.threadID = a.idGenerator("thread")
	}
	r.runID = input.RunID
	if r.runID == "" {
		r.runID = a.idGenerator("run")
	}

	return r.execute()
}

// agentRun holds all per-request state for a single Run() invocation.
type agentRun struct {
	agent *Agent
	ctx   context.Context
	sink  EventSink

	model        string
	threadID     string
	runID        string
	messages     []Message
	toolMgr      *ToolCallManager
	totalUsage   *accumulatedUsage
	finishReason provider.FinishReason
	iteration    int
}

// execute runs the full agentic lifecycle: hooks, events, loop, and cleanup.
func (r *agentRun) execute() error {
	// Hook: OnRunStart
	if r.agent.hooks.OnRunStart != nil {
		if err := r.agent.hooks.OnRunStart(r.ctx, r.threadID, r.runID); err != nil {
			return err
		}
	}

	// Emit RUN_STARTED
	if err := r.sink(NewRunStartedEvent(r.threadID, r.runID)); err != nil {
		return err
	}

	loopErr := r.loop()

	if loopErr != nil {
		return r.handleError(loopErr)
	}

	return r.handleSuccess()
}

// loop runs the agentic loop: handle pending approvals, then alternate
// between processChat and executeToolCalls until the strategy says stop.
func (r *agentRun) loop() error {
	// Handle pending approvals before starting the model loop.
	approvals := extractApprovals(r.messages)
	if len(approvals) > 0 {
		if err := r.handlePendingApprovals(approvals); err != nil {
			return err
		}
	}

	for r.agent.strategy(LoopState{
		IterationCount: r.iteration,
		Messages:       r.messages,
		FinishReason:   r.finishReason,
	}) {
		if err := r.ctx.Err(); err != nil {
			return err
		}

		if err := r.step(); err != nil {
			return err
		}

		r.iteration++
	}

	return nil
}

// handlePendingApprovals processes tool calls that have approval responses
// from the client, executing or denying them before entering the model loop.
func (r *agentRun) handlePendingApprovals(approvals map[string]bool) error {
	pendingToolCalls := extractPendingToolCalls(r.messages)
	if len(pendingToolCalls) == 0 {
		return nil
	}

	r.toolMgr.Clear()
	for _, tc := range pendingToolCalls {
		r.toolMgr.AddStart(tc.ID, tc.Function.Name)
		r.toolMgr.AddArgs(tc.ID, tc.Function.Arguments)
	}

	r.finishReason = provider.FinishReasonToolCalls
	toolMessages, _, err := r.executeToolCalls(approvals)
	if err != nil {
		return err
	}

	if len(toolMessages) > 0 {
		r.messages = append(r.messages, toolMessages...)
		r.finishReason = provider.FinishReasonToolCalls
		r.iteration++
	}

	return nil
}

// step runs one iteration of the agentic loop: model response + tool execution + hooks.
func (r *agentRun) step() error {
	// Phase 1: Process model response
	stepUsage, newMessages, err := r.processChat()
	if err != nil {
		return err
	}

	r.messages = append(r.messages, newMessages...)
	r.totalUsage.add(stepUsage)

	// Phase 2: Execute any tool calls
	toolMessages, toolResults, err := r.executeToolCalls(nil)
	if err != nil {
		return err
	}

	if len(toolMessages) > 0 {
		r.messages = append(r.messages, toolMessages...)
	} else if r.finishReason == provider.FinishReasonToolCalls {
		// Tools were present but none were server-side; treat as stop.
		r.finishReason = provider.FinishReasonStop
	}

	// Hook: OnStepFinish
	if r.agent.hooks.OnStepFinish != nil {
		stepMessages := make([]Message, 0, len(newMessages)+len(toolMessages))
		stepMessages = append(stepMessages, newMessages...)
		stepMessages = append(stepMessages, toolMessages...)

		stepResult := StepResult{
			Messages:     stepMessages,
			Usage:        convertProviderUsage(stepUsage),
			FinishReason: r.finishReason,
			ToolCalls:    r.toolMgr.GetToolCalls(),
			ToolResults:  toolResults,
		}
		if err := r.agent.hooks.OnStepFinish(r.ctx, stepResult); err != nil {
			return err
		}
	}

	return nil
}

// processChat streams the model response, translating provider events into AG-UI events.
// It returns the step usage and new messages to append to conversation history.
func (r *agentRun) processChat() (*provider.Usage, []Message, error) {
	r.toolMgr.Clear()

	var contentBuilder strings.Builder
	messageID := r.agent.idGenerator("msg")
	textStarted := false
	var finishReason provider.FinishReason
	var usage *provider.Usage

	err := r.agent.provider.ChatStream(r.ctx, r.messages, r.agent.tools, r.model, func(event provider.Event) error {
		switch ev := event.(type) {
		case provider.TextDeltaEvent:
			if !textStarted {
				textStarted = true
				if err := r.sink(NewTextMessageStartEvent(messageID, "assistant")); err != nil {
					return err
				}
			}
			contentBuilder.WriteString(ev.Delta)
			return r.sink(NewTextMessageContentEvent(messageID, ev.Delta))

		case provider.ToolCallStartEvent:
			r.toolMgr.AddStart(ev.ToolCallID, ev.ToolName)
			return r.sink(NewToolCallStartEvent(ev.ToolCallID, ev.ToolName))

		case provider.ToolCallDeltaEvent:
			r.toolMgr.AddArgs(ev.ToolCallID, ev.ArgsDelta)
			return r.sink(NewToolCallArgsEvent(ev.ToolCallID, ev.ArgsDelta))

		case provider.StreamEndEvent:
			finishReason = ev.FinishReason
			usage = ev.Usage
			if textStarted {
				return r.sink(NewTextMessageEndEvent(messageID))
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("provider chat stream: %w", err)
	}

	r.finishReason = finishReason

	// Build assistant message from accumulated content and tool calls
	var newMessages []Message
	accumulatedContent := contentBuilder.String()
	parts := []MessagePart{}
	if accumulatedContent != "" {
		parts = append(parts, NewTextPart(accumulatedContent))
	}
	toolCalls := r.toolMgr.GetToolCalls()
	for _, tc := range toolCalls {
		parts = append(parts, NewToolCallPart(tc.ID, tc.Function.Name, tc.Function.Arguments))
	}
	if len(parts) > 0 {
		newMessages = append(newMessages, Message{
			Role:  RoleAssistant,
			Parts: parts,
		})
	}

	return usage, newMessages, nil
}

// executeToolCalls handles tool execution after the model response.
// It returns new tool-result messages and the tool results.
func (r *agentRun) executeToolCalls(approvals map[string]bool) ([]Message, []ToolResult, error) {
	if !r.toolMgr.HasToolCalls() {
		return nil, nil, nil
	}

	// If no server-side tools registered or finish reason isn't tool_calls,
	// emit TOOL_CALL_END without result (client-side tools) and stop.
	if len(r.agent.tools) == 0 || r.finishReason != provider.FinishReasonToolCalls {
		for _, tc := range r.toolMgr.GetToolCalls() {
			if err := r.sink(NewToolCallEndEvent(tc.ID, tc.Function.Name)); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	}

	// Execute tools and emit TOOL_CALL_END with results
	execResult := r.toolMgr.ExecuteToolCalls(r.ctx, r.agent.tools, approvals, r.agent.idGenerator)

	// Emit CUSTOM events for tool calls that need approval.
	for _, req := range execResult.NeedsApproval {
		endEvent := NewToolCallEndEvent(req.ToolCallID, req.ToolName)
		if err := r.sink(endEvent); err != nil {
			return nil, nil, err
		}

		if err := r.sink(NewCustomEvent("approval-requested", map[string]any{
			"toolCallId": req.ToolCallID,
			"toolName":   req.ToolName,
			"input":      req.Input,
			"approval": map[string]any{
				"id":            req.ApprovalID,
				"needsApproval": true,
			},
		})); err != nil {
			return nil, nil, err
		}
	}

	// Emit results for executed/denied tool calls.
	var newMessages []Message
	for _, result := range execResult.Results {
		endEvent := NewToolCallEndEvent(result.ToolCallID, result.ToolName)
		endEvent.Result = result.Content
		if err := r.sink(endEvent); err != nil {
			return nil, nil, err
		}

		newMessages = append(newMessages, Message{
			Role: RoleTool,
			Parts: []MessagePart{
				NewToolResultPart(result.ToolCallID, result.Content),
			},
		})
	}

	// If there are pending approvals, stop the loop by returning no tool messages.
	// The run will finish and the client will re-call with approval decisions.
	if len(execResult.NeedsApproval) > 0 {
		return nil, execResult.Results, nil
	}

	return newMessages, execResult.Results, nil
}

// handleError emits a RUN_ERROR event and calls the OnError hook.
func (r *agentRun) handleError(err error) error {
	if r.agent.hooks.OnError != nil {
		if hookErr := r.agent.hooks.OnError(r.ctx, err); hookErr != nil {
			return fmt.Errorf("run failed: %w; on error hook: %v", err, hookErr)
		}
	}

	_ = r.sink(NewRunErrorEvent(err.Error(), runErrorCode(err), r.runID))
	return err
}

// handleSuccess emits a RUN_FINISHED event and calls the OnFinish hook.
func (r *agentRun) handleSuccess() error {
	finishedEvent := NewRunFinishedEvent(r.threadID, r.runID, r.finishReason)
	finishedEvent.Usage = r.totalUsage.toUsage()
	if err := r.sink(finishedEvent); err != nil {
		return err
	}

	if r.agent.hooks.OnFinish != nil {
		return r.agent.hooks.OnFinish(r.ctx, RunResult{
			Messages:     r.messages,
			Usage:        r.totalUsage.toUsage(),
			FinishReason: r.finishReason,
			ThreadID:     r.threadID,
			RunID:        r.runID,
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

// extractApprovals scans messages for tool-call parts with approval responses
// and returns a map of tool call ID → approved (true/false).
func extractApprovals(messages []Message) map[string]bool {
	approvals := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role != RoleAssistant {
			continue
		}
		for _, part := range msg.Parts {
			if part.Type != MessagePartTypeToolCall {
				continue
			}
			if part.State != ToolCallStateApprovalResponded {
				continue
			}
			if part.Approval == nil || part.Approval.Approved == nil {
				continue
			}
			approvals[part.ID] = *part.Approval.Approved
		}
	}
	return approvals
}

// extractPendingToolCalls extracts tool calls from assistant messages that have
// approval responses, so they can be re-executed with the approval decisions.
func extractPendingToolCalls(messages []Message) []ToolCall {
	var calls []ToolCall
	for _, msg := range messages {
		if msg.Role != RoleAssistant {
			continue
		}
		for _, part := range msg.Parts {
			if part.Type != MessagePartTypeToolCall {
				continue
			}
			if part.State != ToolCallStateApprovalResponded {
				continue
			}
			calls = append(calls, ToolCall{
				ID:   part.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      part.Name,
					Arguments: part.Arguments,
				},
			})
		}
	}
	return calls
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
