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

package openai

import (
	"context"
	"fmt"

	"github.com/alexandrevilain/tanstack-ai-go/provider"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"

	tanstackai "github.com/alexandrevilain/tanstack-ai-go"
)

// Provider implements tanstackai.Provider for OpenAI.
type Provider struct {
	client openai.Client
}

// NewProvider creates a new OpenAI provider.
func NewProvider(opts ...option.RequestOption) *Provider {
	client := openai.NewClient(opts...)
	return &Provider{client: client}
}

// ChatStream implements tanstackai.Provider.
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []tanstackai.Message,
	tools []tanstackai.Tool,
	model string,
	sink provider.EventSink,
) error {
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: convertMessages(messages),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}
	if len(tools) > 0 {
		params.Tools = convertTools(tools)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() {
		_ = stream.Close()
	}()

	toolCallsState := make(map[int]*toolCallState)
	var usage *provider.Usage
	var finishReason provider.FinishReason

	for stream.Next() {
		chunk := stream.Current()

		// Capture usage from the final chunk (arrives after finish reason, with empty choices)
		if chunk.Usage.TotalTokens > 0 {
			usage = &provider.Usage{
				PromptTokens:     int(chunk.Usage.PromptTokens),
				CompletionTokens: int(chunk.Usage.CompletionTokens),
				TotalTokens:      int(chunk.Usage.TotalTokens),
			}
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		delta := choice.Delta

		// Handle text content
		if delta.Content != "" {
			if err := sink(provider.TextDeltaEvent{Delta: delta.Content}); err != nil {
				return err
			}
		}

		// Handle tool calls
		for _, tc := range delta.ToolCalls {
			idx := int(tc.Index)
			state, exists := toolCallsState[idx]
			if !exists {
				state = &toolCallState{}
				toolCallsState[idx] = state
			}
			if tc.ID != "" {
				state.id = tc.ID
			}
			if tc.Function.Name != "" {
				state.name = tc.Function.Name
			}

			// Emit start once we have both ID and name
			if !state.started && state.id != "" && state.name != "" {
				state.started = true
				if err := sink(provider.ToolCallStartEvent{
					ToolCallID: state.id, ToolName: state.name,
				}); err != nil {
					return err
				}
			}

			// Emit argument deltas
			if tc.Function.Arguments != "" {
				if err := sink(provider.ToolCallDeltaEvent{
					ToolCallID: state.id, ArgsDelta: tc.Function.Arguments,
				}); err != nil {
					return err
				}
			}
		}

		// Capture finish reason (usage arrives in a later chunk)
		if choice.FinishReason != "" {
			finishReason = mapFinishReason(choice.FinishReason)
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("openai stream: %w", err)
	}

	// Emit StreamEndEvent after the loop so usage from the final chunk is included
	if err := sink(provider.StreamEndEvent{
		FinishReason: finishReason,
		Usage:        usage,
	}); err != nil {
		return err
	}

	return nil
}

type toolCallState struct {
	id      string
	name    string
	started bool
}

func mapFinishReason(reason string) provider.FinishReason {
	switch reason {
	case "stop":
		return provider.FinishReasonStop
	case "tool_calls":
		return provider.FinishReasonToolCalls
	case "length":
		return provider.FinishReasonLength
	case "content_filter":
		return provider.FinishReasonContentFilter
	default:
		return provider.FinishReasonUnknown
	}
}
