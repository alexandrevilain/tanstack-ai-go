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
	"encoding/json"
	"fmt"
)

// ToolCallManager accumulates streaming tool call events and tracks state.
type ToolCallManager struct {
	calls map[string]*trackedToolCall // toolCallID -> tracked state
	order []string                    // preserve insertion order
}

type trackedToolCall struct {
	id   string
	name string
	args string
}

// NewToolCallManager creates a new ToolCallManager.
func NewToolCallManager() *ToolCallManager {
	return &ToolCallManager{
		calls: make(map[string]*trackedToolCall),
	}
}

// AddStart begins tracking a new tool call.
func (m *ToolCallManager) AddStart(id, name string) {
	m.calls[id] = &trackedToolCall{id: id, name: name}
	m.order = append(m.order, id)
}

// AddArgs appends argument delta to the tool call identified by id.
func (m *ToolCallManager) AddArgs(id, delta string) {
	if tc, ok := m.calls[id]; ok {
		tc.args += delta
	}
}

// GetToolCalls returns all tracked tool calls in order.
func (m *ToolCallManager) GetToolCalls() []ToolCall {
	result := make([]ToolCall, 0, len(m.order))
	for _, id := range m.order {
		tc := m.calls[id]
		result = append(result, ToolCall{
			ID:   tc.id,
			Type: "function",
			Function: ToolCallFunction{
				Name:      tc.name,
				Arguments: tc.args,
			},
		})
	}
	return result
}

// HasToolCalls returns true if there are any tracked tool calls.
func (m *ToolCallManager) HasToolCalls() bool {
	return len(m.calls) > 0
}

// Clear resets the manager for the next iteration.
func (m *ToolCallManager) Clear() {
	m.calls = make(map[string]*trackedToolCall)
	m.order = nil
}

// ToolResult holds the result of a tool execution.
type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
}

// ApprovalRequest represents a tool call that needs user approval before execution.
type ApprovalRequest struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	Input      any    `json:"input"`
	ApprovalID string `json:"approvalId"`
}

// ToolExecutionResult holds the outcome of ExecuteToolCalls, including
// completed results and any tool calls that need user approval.
type ToolExecutionResult struct {
	Results       []ToolResult
	NeedsApproval []ApprovalRequest
}

// ExecuteToolCalls executes all tool calls against the registered tools.
// The approvals map contains approval decisions keyed by tool call ID (true = approved, false = denied).
// Tool calls requiring approval that have no decision in the map are returned in NeedsApproval.
func ExecuteToolCalls(ctx context.Context, toolCalls []ToolCall, tools []Tool, approvals map[string]bool, idGenerator IDGenerator) ToolExecutionResult {
	toolMap := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolMap[t.Name] = t
	}

	var result ToolExecutionResult
	for _, tc := range toolCalls {
		tool, ok := toolMap[tc.Function.Name]
		if !ok {
			result.Results = append(result.Results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(fmt.Sprintf("unknown tool: %s", tc.Function.Name)),
			})
			continue
		}
		if tool.Execute == nil {
			result.Results = append(result.Results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent("tool has no server-side execute function"),
			})
			continue
		}

		// Handle approval flow for tools that need approval.
		if tool.NeedsApproval {
			approved, hasDecision := approvals[tc.ID]
			if !hasDecision {
				// Parse input for the approval request.
				var input any
				argsStr := tc.Function.Arguments
				if argsStr == "" {
					argsStr = "{}"
				}
				_ = json.Unmarshal([]byte(argsStr), &input)

				result.NeedsApproval = append(result.NeedsApproval, ApprovalRequest{
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Input:      input,
					ApprovalID: idGenerator("approval"),
				})
				continue
			}
			if !approved {
				denied, _ := json.Marshal(map[string]any{
					"approved": false,
					"message":  "User denied this action",
				})
				result.Results = append(result.Results, ToolResult{
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
					Content:    string(denied),
				})
				continue
			}
			// approved == true: fall through to execute normally
		}

		var args map[string]any
		argsStr := tc.Function.Arguments
		if argsStr == "" {
			argsStr = "{}"
		}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			result.Results = append(result.Results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(fmt.Sprintf("invalid arguments: %s", err)),
			})
			continue
		}

		execResult, err := tool.Execute(ctx, args)
		if err != nil {
			result.Results = append(result.Results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(err.Error()),
			})
			continue
		}

		encoded, err := json.Marshal(execResult)
		if err != nil {
			encoded = []byte(toolErrorContent(fmt.Sprintf("marshal result: %s", err)))
		}
		result.Results = append(result.Results, ToolResult{
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    string(encoded),
		})
	}
	return result
}

func toolErrorContent(message string) string {
	encoded, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return `{"error":"internal tool error"}`
	}
	return string(encoded)
}
