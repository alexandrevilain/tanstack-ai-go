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
	"strings"
)

// Role represents a message role in the conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// MessagePartType represents the type of a message part.
type MessagePartType string

const (
	MessagePartTypeText       MessagePartType = "text"
	MessagePartTypeToolCall   MessagePartType = "tool-call"
	MessagePartTypeToolResult MessagePartType = "tool-result"
)

// ToolCallState represents the state of a tool call in the approval flow.
type ToolCallState string

const (
	ToolCallStateApprovalRequested ToolCallState = "approval-requested"
	ToolCallStateApprovalResponded ToolCallState = "approval-responded"
)

// ToolCallApproval holds approval metadata for a tool call.
type ToolCallApproval struct {
	ID            string `json:"id"`
	NeedsApproval bool   `json:"needsApproval"`
	Approved      *bool  `json:"approved,omitempty"`
}

// MessagePart represents a part of a message in the AG-UI protocol.
// This is a union type — fields are relevant depending on Type:
//   - "text": Content
//   - "tool-call": ID, Name, Arguments
//   - "tool-result": ToolCallID, Content
type MessagePart struct {
	Type MessagePartType `json:"type"`

	// Shared by text and tool-result parts.
	Content string `json:"content,omitempty"`

	// tool-call fields
	ID        string `json:"id,omitempty"`        // tool-call: unique call ID
	Name      string `json:"name,omitempty"`      // tool-call: tool name
	Arguments string `json:"arguments,omitempty"` // tool-call: JSON-encoded arguments

	// tool-result fields
	ToolCallID string `json:"toolCallId,omitempty"` // tool-result: references a tool-call ID
	Error      string `json:"error,omitempty"`      // tool-result: error message if failed

	// tool-call approval fields
	State    ToolCallState     `json:"state,omitempty"`    // tool-call: approval state
	Approval *ToolCallApproval `json:"approval,omitempty"` // tool-call: approval metadata
}

// ToolCallFunction holds the function name and JSON-encoded arguments.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a tool/function call from the model.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// Message represents a message in the AG-UI protocol.
// Messages use a parts-based format where content is structured as an array of typed parts.
type Message struct {
	ID        string        `json:"id,omitempty"`
	Role      Role          `json:"role"`
	Parts     []MessagePart `json:"parts,omitempty"`
	CreatedAt string        `json:"createdAt,omitempty"`
}

// TextContent returns the concatenated text content from all text parts.
func (m Message) TextContent() string {
	var sb strings.Builder
	for _, p := range m.Parts {
		if p.Type == MessagePartTypeText {
			sb.WriteString(p.Content)
		}
	}
	return sb.String()
}

// ToolCalls returns all tool call parts as ToolCall values.
func (m Message) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, p := range m.Parts {
		if p.Type == MessagePartTypeToolCall {
			calls = append(calls, ToolCall{
				ID:   p.ID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      p.Name,
					Arguments: p.Arguments,
				},
			})
		}
	}
	return calls
}

// NewTextPart creates a text message part.
func NewTextPart(content string) MessagePart {
	return MessagePart{Type: MessagePartTypeText, Content: content}
}

// NewToolCallPart creates a tool-call message part.
func NewToolCallPart(id, name, arguments string) MessagePart {
	return MessagePart{
		Type:      MessagePartTypeToolCall,
		ID:        id,
		Name:      name,
		Arguments: arguments,
	}
}

// NewToolResultPart creates a tool-result message part.
func NewToolResultPart(toolCallID, content string) MessagePart {
	return MessagePart{
		Type:       MessagePartTypeToolResult,
		ToolCallID: toolCallID,
		Content:    content,
	}
}

// ToolExecuteFunc is the function signature for server-side tool execution.
type ToolExecuteFunc func(ctx context.Context, args map[string]any) (any, error)

// Tool defines a tool/function that the model can call.
type Tool struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	InputSchema   map[string]any  `json:"inputSchema,omitempty"`
	NeedsApproval bool            `json:"needsApproval,omitempty"`
	Execute       ToolExecuteFunc `json:"-"`
}

// RunInput holds data from an incoming chat request.
type RunInput struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	ThreadID string    `json:"threadId,omitempty"`
	RunID    string    `json:"runId,omitempty"`
}
