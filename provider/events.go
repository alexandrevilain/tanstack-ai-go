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

package provider

// EventSink is a function that receives provider-level events.
// Returning a non-nil error signals the provider to stop streaming.
type EventSink func(event Event) error

// Event is the type for events emitted by providers.
// Providers emit these provider-agnostic events, which the agent
// translates into AG-UI protocol events using a type switch.
type Event interface {
	isProviderEvent()
}

// TextDeltaEvent represents a chunk of text content from the model.
type TextDeltaEvent struct {
	Delta string
}

func (TextDeltaEvent) isProviderEvent() {}

// ToolCallStartEvent signals the start of a tool call from the model.
type ToolCallStartEvent struct {
	ToolCallID string
	ToolName   string
}

func (ToolCallStartEvent) isProviderEvent() {}

// ToolCallDeltaEvent contains a chunk of tool call arguments.
type ToolCallDeltaEvent struct {
	ToolCallID string
	ArgsDelta  string
}

func (ToolCallDeltaEvent) isProviderEvent() {}

// FinishReason represents why the model stopped generating.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonUnknown       FinishReason = "unknown"
)

// Usage represents token usage information from a single model call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// StreamEndEvent signals the end of the model's response stream.
type StreamEndEvent struct {
	FinishReason FinishReason
	Usage        *Usage
}

func (StreamEndEvent) isProviderEvent() {}
