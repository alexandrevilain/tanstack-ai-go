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

import "github.com/alexandrevilain/tanstack-ai-go/provider"

// LoopState holds state for the agent loop strategy decision.
type LoopState struct {
	IterationCount int
	Messages       []Message
	FinishReason   provider.FinishReason
}

// LoopStrategy determines whether the agent loop should continue.
// Returns true to continue, false to stop.
type LoopStrategy func(state LoopState) bool

// MaxIterations returns a strategy that allows up to n iterations.
func MaxIterations(n int) LoopStrategy {
	return func(state LoopState) bool {
		return state.IterationCount < n
	}
}

// UntilFinishReason returns a strategy that stops when a specific
// finish reason is encountered.
func UntilFinishReason(stopReasons ...provider.FinishReason) LoopStrategy {
	set := make(map[provider.FinishReason]bool, len(stopReasons))
	for _, r := range stopReasons {
		set[r] = true
	}
	return func(state LoopState) bool {
		if state.IterationCount == 0 {
			return true
		}
		return !set[state.FinishReason]
	}
}

// CombineStrategies returns a strategy that continues only if ALL
// sub-strategies return true.
func CombineStrategies(strategies ...LoopStrategy) LoopStrategy {
	return func(state LoopState) bool {
		for _, s := range strategies {
			if !s(state) {
				return false
			}
		}
		return true
	}
}
