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

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	tanstackai "github.com/alexandrevilain/tanstack-ai-go"
	tsopenai "github.com/alexandrevilain/tanstack-ai-go/openai"
	"github.com/alexandrevilain/tanstack-ai-go/provider"
)

func main() {
	// Create OpenAI provider (uses OPENAI_API_KEY env var by default)
	openaiProvider := tsopenai.NewProvider()

	// Define a server-side tool with approval required
	weatherTool := tanstackai.Tool{
		Name:          "get_weather",
		Description:   "Get the current weather for a location",
		NeedsApproval: true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "The city and state, e.g. San Francisco, CA",
				},
			},
			"required": []string{"location"},
		},
		Execute: func(ctx context.Context, args map[string]any) (any, error) {
			location, _ := args["location"].(string)
			return map[string]any{
				"location":    location,
				"temperature": 72,
				"unit":        "fahrenheit",
				"conditions":  "sunny",
			}, nil
		},
	}

	agent := tanstackai.NewAgent(openaiProvider,
		tanstackai.WithTools(weatherTool),
		tanstackai.WithStrategy(tanstackai.CombineStrategies(
			tanstackai.MaxIterations(5),
			tanstackai.UntilFinishReason(provider.FinishReasonStop),
		)),
	)

	handler := tanstackai.NewHandler(agent, func(r *http.Request) (tanstackai.RunInput, error) {
		input, err := tanstackai.DefaultRequestDecoder(r)
		if err != nil {
			return input, err
		}
		// Set default model if not specified
		if input.Model == "" {
			input.Model = "gpt-4o"
		}
		return input, nil
	})

	mux := http.NewServeMux()
	mux.Handle("POST /api/chat", handler)
	mux.HandleFunc("OPTIONS /api/chat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	addr := ":8080"
	fmt.Printf("Server listening on %s\n", addr)

	if err := http.ListenAndServe(addr, corsMiddleware(mux)); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		next.ServeHTTP(w, r)
	})
}
