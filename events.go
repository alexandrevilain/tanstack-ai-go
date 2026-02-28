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
	"encoding/json"
	"time"

	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

// EventType represents an AG-UI protocol event type.
type EventType string

const (
	EventTypeRunStarted         EventType = "RUN_STARTED"
	EventTypeRunFinished        EventType = "RUN_FINISHED"
	EventTypeRunError           EventType = "RUN_ERROR"
	EventTypeTextMessageStart   EventType = "TEXT_MESSAGE_START"
	EventTypeTextMessageContent EventType = "TEXT_MESSAGE_CONTENT"
	EventTypeTextMessageEnd     EventType = "TEXT_MESSAGE_END"
	EventTypeToolCallStart      EventType = "TOOL_CALL_START"
	EventTypeToolCallArgs       EventType = "TOOL_CALL_ARGS"
	EventTypeToolCallEnd        EventType = "TOOL_CALL_END"
	EventTypeStepStarted        EventType = "STEP_STARTED"
	EventTypeStepFinished       EventType = "STEP_FINISHED"
	EventTypeStateSnapshot      EventType = "STATE_SNAPSHOT"
	EventTypeStateDelta         EventType = "STATE_DELTA"
	EventTypeCustom             EventType = "CUSTOM"
)

// Event is the interface that all AG-UI events implement.
type Event interface {
	EventType() EventType
}

// Usage represents token usage information for a run.
type Usage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// RunError represents an error that occurred during a run.
type RunError struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// RunStartedEvent signals the start of an agent run.
type RunStartedEvent struct {
	Type      EventType `json:"type"`
	ThreadID  string    `json:"threadId"`
	RunID     string    `json:"runId"`
	Timestamp int64     `json:"timestamp"`
}

func (e RunStartedEvent) EventType() EventType { return e.Type }

// RunFinishedEvent signals the successful completion of an agent run.
type RunFinishedEvent struct {
	Type         EventType             `json:"type"`
	ThreadID     string                `json:"threadId"`
	RunID        string                `json:"runId"`
	FinishReason provider.FinishReason `json:"finishReason"`
	Usage        *Usage                `json:"usage,omitempty"`
	Timestamp    int64                 `json:"timestamp"`
}

func (e RunFinishedEvent) EventType() EventType { return e.Type }

// RunErrorEvent signals an error during an agent run.
type RunErrorEvent struct {
	Type      EventType `json:"type"`
	Error     RunError  `json:"error"`
	RunID     string    `json:"runId,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

func (e RunErrorEvent) EventType() EventType { return e.Type }

// TextMessageStartEvent signals the start of a text message.
type TextMessageStartEvent struct {
	Type      EventType `json:"type"`
	MessageID string    `json:"messageId"`
	Role      string    `json:"role,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

func (e TextMessageStartEvent) EventType() EventType { return e.Type }

// TextMessageContentEvent contains a piece of streaming text content.
type TextMessageContentEvent struct {
	Type      EventType `json:"type"`
	MessageID string    `json:"messageId"`
	Delta     string    `json:"delta"`
	Timestamp int64     `json:"timestamp"`
}

func (e TextMessageContentEvent) EventType() EventType { return e.Type }

// TextMessageEndEvent signals the end of a text message.
type TextMessageEndEvent struct {
	Type      EventType `json:"type"`
	MessageID string    `json:"messageId"`
	Timestamp int64     `json:"timestamp"`
}

func (e TextMessageEndEvent) EventType() EventType { return e.Type }

// ToolCallStartEvent signals the start of a tool call.
type ToolCallStartEvent struct {
	Type       EventType `json:"type"`
	ToolCallID string    `json:"toolCallId"`
	ToolName   string    `json:"toolName"`
	Timestamp  int64     `json:"timestamp"`
}

func (e ToolCallStartEvent) EventType() EventType { return e.Type }

// ToolCallArgsEvent contains streaming tool call arguments.
type ToolCallArgsEvent struct {
	Type       EventType `json:"type"`
	ToolCallID string    `json:"toolCallId"`
	Delta      string    `json:"delta"`
	Timestamp  int64     `json:"timestamp"`
}

func (e ToolCallArgsEvent) EventType() EventType { return e.Type }

// ToolCallEndEvent signals the end of a tool call.
type ToolCallEndEvent struct {
	Type       EventType       `json:"type"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Input      json.RawMessage `json:"input,omitempty"`
	Result     string          `json:"result,omitempty"`
	Timestamp  int64           `json:"timestamp"`
}

func (e ToolCallEndEvent) EventType() EventType { return e.Type }

// now returns the current time in Unix milliseconds.
func now() int64 {
	return time.Now().UnixMilli()
}

// NewRunStartedEvent creates a new RunStartedEvent.
func NewRunStartedEvent(threadID, runID string) RunStartedEvent {
	return RunStartedEvent{
		Type:      EventTypeRunStarted,
		ThreadID:  threadID,
		RunID:     runID,
		Timestamp: now(),
	}
}

// NewRunFinishedEvent creates a new RunFinishedEvent.
func NewRunFinishedEvent(threadID, runID string, finishReason provider.FinishReason) RunFinishedEvent {
	return RunFinishedEvent{
		Type:         EventTypeRunFinished,
		ThreadID:     threadID,
		RunID:        runID,
		FinishReason: finishReason,
		Timestamp:    now(),
	}
}

// NewRunErrorEvent creates a new RunErrorEvent.
func NewRunErrorEvent(message, code, runID string) RunErrorEvent {
	return RunErrorEvent{
		Type: EventTypeRunError,
		Error: RunError{
			Message: message,
			Code:    code,
		},
		RunID:     runID,
		Timestamp: now(),
	}
}

// NewTextMessageStartEvent creates a new TextMessageStartEvent.
func NewTextMessageStartEvent(messageID, role string) TextMessageStartEvent {
	return TextMessageStartEvent{
		Type:      EventTypeTextMessageStart,
		MessageID: messageID,
		Role:      role,
		Timestamp: now(),
	}
}

// NewTextMessageContentEvent creates a new TextMessageContentEvent.
func NewTextMessageContentEvent(messageID, delta string) TextMessageContentEvent {
	return TextMessageContentEvent{
		Type:      EventTypeTextMessageContent,
		MessageID: messageID,
		Delta:     delta,
		Timestamp: now(),
	}
}

// NewTextMessageEndEvent creates a new TextMessageEndEvent.
func NewTextMessageEndEvent(messageID string) TextMessageEndEvent {
	return TextMessageEndEvent{
		Type:      EventTypeTextMessageEnd,
		MessageID: messageID,
		Timestamp: now(),
	}
}

// NewToolCallStartEvent creates a new ToolCallStartEvent.
func NewToolCallStartEvent(toolCallID, toolName string) ToolCallStartEvent {
	return ToolCallStartEvent{
		Type:       EventTypeToolCallStart,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Timestamp:  now(),
	}
}

// NewToolCallArgsEvent creates a new ToolCallArgsEvent.
func NewToolCallArgsEvent(toolCallID, delta string) ToolCallArgsEvent {
	return ToolCallArgsEvent{
		Type:       EventTypeToolCallArgs,
		ToolCallID: toolCallID,
		Delta:      delta,
		Timestamp:  now(),
	}
}

// NewToolCallEndEvent creates a new ToolCallEndEvent.
func NewToolCallEndEvent(toolCallID, toolName string) ToolCallEndEvent {
	return ToolCallEndEvent{
		Type:       EventTypeToolCallEnd,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Timestamp:  now(),
	}
}
