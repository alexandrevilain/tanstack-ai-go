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
	"github.com/google/uuid"
)

// Option configures an Agent at creation time or overrides defaults per Run call.
type Option func(*Agent)

// IDGenerator generates unique identifiers given a prefix (e.g. "thread", "run", "msg").
type IDGenerator func(prefix string) string

// DefaultIDGenerator returns a default ID generator that produces IDs
// in the format "prefix-<short-uuid>".
func DefaultIDGenerator(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) {
		a.systemPrompt = prompt
	}
}

// WithTools sets the available tools.
func WithTools(tools ...Tool) Option {
	return func(a *Agent) {
		a.tools = tools
	}
}

// WithStrategy sets a custom loop strategy, overriding MaxIterations.
func WithStrategy(s LoopStrategy) Option {
	return func(a *Agent) {
		a.strategy = s
	}
}

// WithHooks sets lifecycle hooks.
func WithHooks(h Hooks) Option {
	return func(a *Agent) {
		a.hooks = h
	}
}

// WithIDGenerator sets a custom ID generator for thread, run, and message IDs.
func WithIDGenerator(gen IDGenerator) Option {
	return func(a *Agent) {
		a.idGenerator = gen
	}
}
