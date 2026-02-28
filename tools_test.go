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

func TestExecuteToolCalls(t *testing.T) {
	tests := map[string]struct {
		toolCalls []ToolCall
		tools     []Tool
		expected  []ToolResult
	}{
		"unknown tool": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "unknown", Arguments: "{}"}},
			},
			tools: nil,
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "unknown", Content: `{"error":"unknown tool: unknown"}`},
			},
		},
		"tool with nil execute": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "client_tool", Arguments: "{}"}},
			},
			tools: []Tool{
				{Name: "client_tool", Description: "client-side only"},
			},
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "client_tool", Content: `{"error":"tool has no server-side execute function"}`},
			},
		},
		"invalid json arguments": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "my_tool", Arguments: "not-json"}},
			},
			tools: []Tool{
				{
					Name: "my_tool",
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return nil, nil
					},
				},
			},
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "my_tool", Content: `{"error":"invalid arguments: invalid character 'o' in literal null (expecting 'u')"}`},
			},
		},
		"empty arguments defaults to empty object": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "my_tool", Arguments: ""}},
			},
			tools: []Tool{
				{
					Name: "my_tool",
					Execute: func(_ context.Context, args map[string]any) (any, error) {
						return map[string]int{"count": len(args)}, nil
					},
				},
			},
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "my_tool", Content: `{"count":0}`},
			},
		},
		"successful execution": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "greet", Arguments: `{"name":"Alice"}`}},
			},
			tools: []Tool{
				{
					Name: "greet",
					Execute: func(_ context.Context, args map[string]any) (any, error) {
						return map[string]string{"greeting": fmt.Sprintf("Hello, %s!", args["name"])}, nil
					},
				},
			},
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "greet", Content: `{"greeting":"Hello, Alice!"}`},
			},
		},
		"execute returns error": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "fail_tool", Arguments: "{}"}},
			},
			tools: []Tool{
				{
					Name: "fail_tool",
					Execute: func(_ context.Context, _ map[string]any) (any, error) {
						return nil, fmt.Errorf("something went wrong")
					},
				},
			},
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "fail_tool", Content: `{"error":"something went wrong"}`},
			},
		},
		"multiple tool calls executed in order": {
			toolCalls: []ToolCall{
				{ID: "tc-1", Type: "function", Function: ToolCallFunction{Name: "add", Arguments: `{"a":1,"b":2}`}},
				{ID: "tc-2", Type: "function", Function: ToolCallFunction{Name: "add", Arguments: `{"a":3,"b":4}`}},
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
			expected: []ToolResult{
				{ToolCallID: "tc-1", ToolName: "add", Content: `{"sum":3}`},
				{ToolCallID: "tc-2", ToolName: "add", Content: `{"sum":7}`},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			results := ExecuteToolCalls(context.Background(), test.toolCalls, test.tools)
			g.Expect(results).To(Equal(test.expected))
		})
	}
}
