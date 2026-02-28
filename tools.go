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

// ExecuteToolCalls executes all tool calls against the registered tools.
func ExecuteToolCalls(ctx context.Context, toolCalls []ToolCall, tools []Tool) []ToolResult {
	toolMap := make(map[string]Tool, len(tools))
	for _, t := range tools {
		toolMap[t.Name] = t
	}

	results := make([]ToolResult, 0, len(toolCalls))
	for _, tc := range toolCalls {
		tool, ok := toolMap[tc.Function.Name]
		if !ok {
			results = append(results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(fmt.Sprintf("unknown tool: %s", tc.Function.Name)),
			})
			continue
		}
		if tool.Execute == nil {
			results = append(results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent("tool has no server-side execute function"),
			})
			continue
		}

		var args map[string]any
		argsStr := tc.Function.Arguments
		if argsStr == "" {
			argsStr = "{}"
		}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			results = append(results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(fmt.Sprintf("invalid arguments: %s", err)),
			})
			continue
		}

		result, err := tool.Execute(ctx, args)
		if err != nil {
			results = append(results, ToolResult{
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
				Content:    toolErrorContent(err.Error()),
			})
			continue
		}

		encoded, err := json.Marshal(result)
		if err != nil {
			encoded = []byte(toolErrorContent(fmt.Sprintf("marshal result: %s", err)))
		}
		results = append(results, ToolResult{
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
			Content:    string(encoded),
		})
	}
	return results
}

func toolErrorContent(message string) string {
	encoded, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		return `{"error":"internal tool error"}`
	}
	return string(encoded)
}
