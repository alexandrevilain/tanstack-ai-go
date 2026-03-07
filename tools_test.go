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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
)

func TestToolCallManager_AddStart(t *testing.T) {
	tests := map[string]struct {
		starts        []struct{ id, name string }
		expectedCalls []ToolCall
		expectedHas   bool
	}{
		"no tool calls": {
			starts:        nil,
			expectedCalls: []ToolCall{},
			expectedHas:   false,
		},
		"single tool call": {
			starts: []struct{ id, name string }{
				{id: "tc-1", name: "get_weather"},
			},
			expectedCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "get_weather"}},
			},
			expectedHas: true,
		},
		"multiple tool calls preserve order": {
			starts: []struct{ id, name string }{
				{id: "tc-1", name: "get_weather"},
				{id: "tc-2", name: "search"},
				{id: "tc-3", name: "calculate"},
			},
			expectedCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "get_weather"}},
				{ID: "tc-2", Type: "function", Function: ToolCallFunction{Name: "search"}},
				{ID: "tc-3", Type: "function", Function: ToolCallFunction{Name: "calculate"}},
			},
			expectedHas: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			m := NewToolCallManager()
			for _, s := range test.starts {
				m.AddStart(s.id, s.name)
			}

			g.Expect(m.HasToolCalls()).To(Equal(test.expectedHas))
			g.Expect(m.GetToolCalls()).To(Equal(test.expectedCalls))
		})
	}
}

func TestToolCallManager_AddArgs(t *testing.T) {
	tests := map[string]struct {
		starts []struct{ id, name string }
		deltas []struct{ id, delta string }
		expect map[string]string // toolCallID -> expected args
	}{
		"single delta": {
			starts: []struct{ id, name string }{
				{id: "tc-1", name: "get_weather"},
			},
			deltas: []struct{ id, delta string }{
				{id: "tc-1", delta: `{"city":"NYC"}`},
			},
			expect: map[string]string{
				"tc-1": `{"city":"NYC"}`,
			},
		},
		"multiple deltas concatenated": {
			starts: []struct{ id, name string }{
				{id: "tc-1", name: "get_weather"},
			},
			deltas: []struct{ id, delta string }{
				{id: "tc-1", delta: `{"cit`},
				{id: "tc-1", delta: `y":"NYC`},
				{id: "tc-1", delta: `"}`},
			},
			expect: map[string]string{
				"tc-1": `{"city":"NYC"}`,
			},
		},
		"delta for unknown id is ignored": {
			starts: []struct{ id, name string }{
				{id: "tc-1", name: "get_weather"},
			},
			deltas: []struct{ id, delta string }{
				{id: "tc-unknown", delta: `{"x":1}`},
			},
			expect: map[string]string{
				"tc-1": "",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			m := NewToolCallManager()
			for _, s := range test.starts {
				m.AddStart(s.id, s.name)
			}
			for _, d := range test.deltas {
				m.AddArgs(d.id, d.delta)
			}

			calls := m.GetToolCalls()
			for _, tc := range calls {
				if expected, ok := test.expect[tc.ID]; ok {
					g.Expect(tc.Function.Arguments).To(Equal(expected))
				}
			}
		})
	}
}

func TestToolCallManager_Clear(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	m := NewToolCallManager()
	m.AddStart("tc-1", "get_weather")
	m.AddArgs("tc-1", `{"city":"NYC"}`)

	g.Expect(m.HasToolCalls()).To(BeTrue())

	m.Clear()

	g.Expect(m.HasToolCalls()).To(BeFalse())
	g.Expect(m.GetToolCalls()).To(BeEmpty())
}

func TestToolCallManager_ExecuteToolCalls(t *testing.T) {
	idGen := func(prefix string) string { return prefix + "-test" }

	tests := map[string]struct {
		calls     []struct{ id, name, args string }
		tools     []Tool
		approvals map[string]bool
		expected  ToolExecutionResult
	}{
		"unknown tool": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "unknown", args: "{}"},
			},
			tools: nil,
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "unknown", Content: `{"error":"unknown tool: unknown"}`},
				},
			},
		},
		"tool with nil execute": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "client_tool", args: "{}"},
			},
			tools: []Tool{
				{Name: "client_tool", Description: "client-side only"},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "client_tool", Content: `{"error":"tool has no server-side execute function"}`},
				},
			},
		},
		"invalid json arguments": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "my_tool", args: "not-json"},
			},
			tools: []Tool{
				{
					Name: "my_tool",
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return nil, nil
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "my_tool", Content: `{"error":"invalid arguments: invalid character 'o' in literal null (expecting 'u')"}`},
				},
			},
		},
		"empty arguments defaults to empty object": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "my_tool", args: ""},
			},
			tools: []Tool{
				{
					Name: "my_tool",
					Execute: func(_ context.Context, args map[string]any) (any, error) {
						return map[string]int{"count": len(args)}, nil
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "my_tool", Content: `{"count":0}`},
				},
			},
		},
		"successful execution": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "greet", args: `{"name":"Alice"}`},
			},
			tools: []Tool{
				{
					Name: "greet",
					Execute: func(_ context.Context, args map[string]any) (any, error) {
						return map[string]string{"greeting": fmt.Sprintf("Hello, %s!", args["name"])}, nil
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "greet", Content: `{"greeting":"Hello, Alice!"}`},
				},
			},
		},
		"execute returns error": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "fail_tool", args: "{}"},
			},
			tools: []Tool{
				{
					Name: "fail_tool",
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return nil, fmt.Errorf("something went wrong")
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "fail_tool", Content: `{"error":"something went wrong"}`},
				},
			},
		},
		"multiple tool calls executed in order": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "add", args: `{"a":1,"b":2}`},
				{id: "tc-2", name: "add", args: `{"a":3,"b":4}`},
			},
			tools: []Tool{
				{
					Name: "add",
					Execute: func(_ context.Context, args map[string]any) (any, error) {
						a, _ := args["a"].(float64)
						b, _ := args["b"].(float64)
						return map[string]float64{"sum": a + b}, nil
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "add", Content: `{"sum":3}`},
					{ToolCallID: "tc-2", ToolName: "add", Content: `{"sum":7}`},
				},
			},
		},
		"needs approval with no prior decision": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "danger", args: `{"action":"delete"}`},
			},
			tools: []Tool{
				{
					Name:          "danger",
					NeedsApproval: true,
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return "done", nil
					},
				},
			},
			expected: ToolExecutionResult{
				NeedsApproval: []ApprovalRequest{
					{ToolCallID: "tc-1", ToolName: "danger", Input: map[string]any{"action": "delete"}, ApprovalID: "approval-test"},
				},
			},
		},
		"needs approval with approved decision": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "danger", args: `{"action":"delete"}`},
			},
			tools: []Tool{
				{
					Name:          "danger",
					NeedsApproval: true,
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return map[string]string{"status": "deleted"}, nil
					},
				},
			},
			approvals: map[string]bool{"tc-1": true},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "danger", Content: `{"status":"deleted"}`},
				},
			},
		},
		"needs approval with denied decision": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "danger", args: `{"action":"delete"}`},
			},
			tools: []Tool{
				{
					Name:          "danger",
					NeedsApproval: true,
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return "done", nil
					},
				},
			},
			approvals: map[string]bool{"tc-1": false},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "danger", Content: `{"approved":false,"message":"User denied this action"}`},
				},
			},
		},
		"mix of approval and non-approval tools": {
			calls: []struct{ id, name, args string }{
				{id: "tc-1", name: "safe", args: `{}`},
				{id: "tc-2", name: "danger", args: `{"x":1}`},
			},
			tools: []Tool{
				{
					Name: "safe",
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return "ok", nil
					},
				},
				{
					Name:          "danger",
					NeedsApproval: true,
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return "done", nil
					},
				},
			},
			expected: ToolExecutionResult{
				Results: []ToolResult{
					{ToolCallID: "tc-1", ToolName: "safe", Content: `"ok"`},
				},
				NeedsApproval: []ApprovalRequest{
					{ToolCallID: "tc-2", ToolName: "danger", Input: map[string]any{"x": float64(1)}, ApprovalID: "approval-test"},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			m := NewToolCallManager()
			for _, c := range test.calls {
				m.AddStart(c.id, c.name)
				if c.args != "" {
					m.AddArgs(c.id, c.args)
				}
			}

			result := m.ExecuteToolCalls(context.Background(), test.tools, test.approvals, idGen)
			g.Expect(result).To(Equal(test.expected))
		})
	}
}
