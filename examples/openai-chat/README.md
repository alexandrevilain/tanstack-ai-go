# OpenAI Chat Example

A chat application with a Go backend streaming AG-UI events and a React frontend using `@tanstack/ai-react`.

The backend exposes a `POST /api/chat` endpoint with a server-side `get_weather` tool. The frontend connects via SSE and renders messages in real time.

## Prerequisites

- Go 1.23+
- Node.js 20+ (or Bun)
- An OpenAI API key

## Quick Start

### 1. Start the backend

```bash
export OPENAI_API_KEY=sk-...
go run main.go
```

The server starts on `http://localhost:8080`.

### 2. Start the frontend

In a separate terminal, from this directory:

```bash
# Install dependencies
npm install   # or: bun install

# Start the dev server
npm run dev   # or: bun dev
```

Vite starts on `http://localhost:5173` and proxies `/api` requests to the Go backend.

### 3. Chat

Open `http://localhost:5173` in your browser and start chatting. Try asking about the weather to see the server-side tool in action.
