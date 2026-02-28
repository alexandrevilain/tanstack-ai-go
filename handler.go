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
	"fmt"
	"net/http"
)

// RequestDecoder extracts a RunInput from an incoming HTTP request.
type RequestDecoder func(r *http.Request) (RunInput, error)

// NewHandler creates an http.Handler that serves AG-UI SSE streams.
func NewHandler(agent *Agent, decoder RequestDecoder) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		input, err := decoder(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		_ = agent.Run(r.Context(), input, func(event Event) error {
			data, err := json.Marshal(event)
			if err != nil {
				return fmt.Errorf("marshal event: %w", err)
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})

		// Send [DONE] marker to signal end of SSE stream
		_, err = fmt.Fprintf(w, "data: [DONE]\n\n")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		flusher.Flush()
	})
}

// DefaultRequestDecoder parses a JSON request body into a RunInput.
// It handles the AG-UI protocol wire format where messages use parts
// and configuration is nested under a "data" object.
func DefaultRequestDecoder(r *http.Request) (RunInput, error) {
	var req struct {
		Messages []Message `json:"messages"`
		Data     struct {
			Model          string `json:"model"`
			ConversationID string `json:"conversationId"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return RunInput{}, fmt.Errorf("decode request: %w", err)
	}

	return RunInput{
		ThreadID: req.Data.ConversationID,
		RunID:    req.Data.ConversationID,
		Messages: req.Messages,
		Model:    req.Data.Model,
	}, nil
}
