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

// EventSink is a function that receives AG-UI events as they are produced.
// Returning a non-nil error signals the stream to stop.
type EventSink func(event Event) error

// Provider is the interface that AI provider implementations must implement.
// It streams a single chat completion turn, emitting provider-level events
// (TextDeltaEvent, ToolCallStartEvent, etc.) to the sink.
//
// The agent translates these into AG-UI protocol events. Providers must NOT
// emit AG-UI events directly and must NOT manage run lifecycle.
type Provider interface {
	ChatStream(ctx context.Context,
		messages []Message,
		tools []Tool,
		model string,
		sink provider.EventSink) error
}
