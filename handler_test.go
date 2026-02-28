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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

// fakeProvider implements Provider for testing.
type fakeProvider struct {
	events []provider.Event
	err    error
}

func (f *fakeProvider) ChatStream(_ context.Context, _ []Message, _ []Tool, _ string, sink provider.EventSink) error {
	for _, ev := range f.events {
		if err := sink(ev); err != nil {
			return err
		}
	}
	return f.err
}

// assertSSEResponse validates that the response has correct SSE headers, well-formed
// SSE framing with a [DONE] terminator, and that each event's JSON content matches the
// corresponding entry in expectedEvents. Only keys present in the expected map are checked,
// so dynamic fields like "timestamp" can be omitted.
func assertSSEResponse(g Gomega, rec *httptest.ResponseRecorder, expectedEvents []map[string]any) {
	// Verify SSE headers
	g.Expect(rec.Header().Get("Content-Type")).To(Equal("text/event-stream"))
	g.Expect(rec.Header().Get("Cache-Control")).To(Equal("no-cache"))
	g.Expect(rec.Header().Get("Connection")).To(Equal("keep-alive"))
	g.Expect(rec.Header().Get("X-Accel-Buffering")).To(Equal("no"))

	body := rec.Body.String()
	lines := strings.Split(body, "\n")

	// Parse all SSE event payloads
	var events []map[string]any
	for i, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		// Each data line must be followed by a blank line (SSE double newline)
		if i+1 < len(lines) {
			g.Expect(lines[i+1]).To(Equal(""), "SSE data line not followed by blank line: %s", line)
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}

		var parsed map[string]any
		g.Expect(json.Unmarshal([]byte(data), &parsed)).To(Succeed(), "invalid JSON in SSE data: %s", data)
		events = append(events, parsed)
	}

	// Stream must end with [DONE] marker
	g.Expect(body).To(HaveSuffix("data: [DONE]\n\n"))

	// Match each parsed event against the expected events
	g.Expect(events).To(HaveLen(len(expectedEvents)), "expected %d SSE events, got %d", len(expectedEvents), len(events))

	for i, expected := range expectedEvents {
		for key, expectedVal := range expected {
			g.Expect(events[i]).To(HaveKeyWithValue(key, expectedVal),
				"event %d: expected %s=%v", i, key, expectedVal)
		}
	}
}

func TestNewHandler(t *testing.T) {
	tests := map[string]struct {
		provider         *fakeProvider
		decoder          RequestDecoder
		expectedStatus   int
		validateResponse func(g Gomega, rec *httptest.ResponseRecorder)
	}{
		"successful SSE stream": {
			provider: &fakeProvider{
				events: []provider.Event{
					provider.TextDeltaEvent{Delta: "Hello"},
					provider.StreamEndEvent{FinishReason: provider.FinishReasonStop},
				},
			},
			decoder: func(r *http.Request) (RunInput, error) {
				return RunInput{
					ThreadID: "thread-1",
					RunID:    "run-1",
					Model:    "gpt-4",
					Messages: []Message{
						{Role: RoleUser, Parts: []MessagePart{NewTextPart("hi")}},
					},
				}, nil
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(g Gomega, rec *httptest.ResponseRecorder) {
				assertSSEResponse(g, rec, []map[string]any{
					{"type": "RUN_STARTED", "threadId": "thread-1", "runId": "run-1"},
					{"type": "TEXT_MESSAGE_START", "role": "assistant"},
					{"type": "TEXT_MESSAGE_CONTENT", "delta": "Hello"},
					{"type": "TEXT_MESSAGE_END"},
					{"type": "RUN_FINISHED", "threadId": "thread-1", "runId": "run-1", "finishReason": "stop"},
				})
			},
		},
		"stream without text content": {
			provider: &fakeProvider{
				events: []provider.Event{
					provider.StreamEndEvent{FinishReason: provider.FinishReasonStop},
				},
			},
			decoder: func(r *http.Request) (RunInput, error) {
				return RunInput{ThreadID: "t", RunID: "r"}, nil
			},
			expectedStatus: http.StatusOK,
			validateResponse: func(g Gomega, rec *httptest.ResponseRecorder) {
				assertSSEResponse(g, rec, []map[string]any{
					{"type": "RUN_STARTED", "threadId": "t", "runId": "r"},
					{"type": "RUN_FINISHED", "threadId": "t", "runId": "r", "finishReason": "stop"},
				})
			},
		},
		"decoder error returns 400": {
			provider: &fakeProvider{},
			decoder: func(r *http.Request) (RunInput, error) {
				return RunInput{}, fmt.Errorf("invalid request body")
			},
			expectedStatus: http.StatusBadRequest,
			validateResponse: func(g Gomega, rec *httptest.ResponseRecorder) {
				g.Expect(rec.Body.String()).To(ContainSubstring("invalid request body"))
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			agent := NewAgent(test.provider)
			handler := NewHandler(agent, test.decoder)

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			g.Expect(rec.Code).To(Equal(test.expectedStatus))
			test.validateResponse(g, rec)
		})
	}
}

func TestDefaultRequestDecoder(t *testing.T) {
	tests := map[string]struct {
		body        string
		expected    RunInput
		expectError bool
	}{
		"valid request": {
			body: `{
				"messages": [
					{"role": "user", "parts": [{"type": "text", "content": "hello"}]}
				],
				"data": {
					"model": "gpt-4",
					"conversationId": "conv-123"
				}
			}`,
			expected: RunInput{
				ThreadID: "conv-123",
				RunID:    "conv-123",
				Model:    "gpt-4",
				Messages: []Message{
					{Role: RoleUser, Parts: []MessagePart{NewTextPart("hello")}},
				},
			},
		},
		"empty messages": {
			body: `{
				"messages": [],
				"data": {
					"model": "gpt-4",
					"conversationId": "conv-1"
				}
			}`,
			expected: RunInput{
				ThreadID: "conv-1",
				RunID:    "conv-1",
				Model:    "gpt-4",
				Messages: []Message{},
			},
		},
		"missing data fields": {
			body: `{
				"messages": [],
				"data": {}
			}`,
			expected: RunInput{
				Messages: []Message{},
			},
		},
		"invalid JSON": {
			body:        `{invalid`,
			expectError: true,
		},
		"unknown fields": {
			body:        `{"messages": [], "data": {}, "unknown": true}`,
			expectError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")

			result, err := DefaultRequestDecoder(req)
			if test.expectError {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(Equal(test.expected))
		})
	}
}
