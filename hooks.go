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

	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

// Hooks contains optional callbacks for agent lifecycle events.
// All fields default to nil (no-op). Safe to use as zero value.
type Hooks struct {
	// OnRunStart is called when a run begins, before the first model call.
	OnRunStart func(ctx context.Context, threadID, runID string) error

	// OnStepFinish is called after each iteration of the agentic loop
	// (model response + tool execution). Useful for logging, metrics, or
	// intermediate persistence.
	OnStepFinish func(ctx context.Context, step StepResult) error

	// OnFinish is called after the run completes successfully.
	// The RunResult contains the full accumulated messages and usage,
	// making this the natural place for persistence.
	OnFinish func(ctx context.Context, result RunResult) error

	// OnError is called when the run encounters an error.
	// The error has already been emitted as a RUN_ERROR event.
	OnError func(ctx context.Context, err error) error
}

// StepResult holds information about a single iteration of the agentic loop.
type StepResult struct {
	// Messages produced in this step (assistant message + any tool results).
	Messages     []Message
	Usage        *Usage
	FinishReason provider.FinishReason
	ToolCalls    []ToolCall
	ToolResults  []ToolResult
}

// RunResult holds accumulated information about the entire run.
type RunResult struct {
	// Messages is the complete conversation history including the system prompt
	// and all messages produced during the run.
	Messages     []Message
	Usage        *Usage
	FinishReason provider.FinishReason
	ThreadID     string
	RunID        string
}
