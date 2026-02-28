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
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"

	tanstackai "github.com/alexandrevilain/tanstack-ai-go"
)

// convertMessages converts Message slice to OpenAI message format.
func convertMessages(msgs []tanstackai.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case tanstackai.RoleSystem:
			result = append(result, openai.SystemMessage(msg.TextContent()))
		case tanstackai.RoleUser:
			result = append(result, openai.UserMessage(msg.TextContent()))
		case tanstackai.RoleAssistant:
			toolCalls := msg.ToolCalls()
			if len(toolCalls) > 0 {
				oaiToolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(toolCalls))
				for i, tc := range toolCalls {
					oaiToolCalls[i] = openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
				result = append(result, openai.ChatCompletionMessageParamUnion{
					OfAssistant: &openai.ChatCompletionAssistantMessageParam{
						Content: openai.ChatCompletionAssistantMessageParamContentUnion{
							OfString: param.NewOpt(msg.TextContent()),
						},
						ToolCalls: oaiToolCalls,
					},
				})
			} else {
				result = append(result, openai.AssistantMessage(msg.TextContent()))
			}
		case tanstackai.RoleTool:
			// Tool result messages have a single tool-result part
			for _, p := range msg.Parts {
				if p.Type == tanstackai.MessagePartTypeToolResult {
					result = append(result, openai.ToolMessage(p.Content, p.ToolCallID))
				}
			}
		}
	}
	return result
}

// convertTools converts Tool slice to OpenAI tool parameters.
func convertTools(tools []tanstackai.Tool) []openai.ChatCompletionToolParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolParam, len(tools))
	for i, t := range tools {
		result[i] = openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: param.NewOpt(t.Description),
				Parameters:  shared.FunctionParameters(t.InputSchema),
			},
		}
	}
	return result
}
