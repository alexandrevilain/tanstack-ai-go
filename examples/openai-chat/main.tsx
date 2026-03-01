import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { useState } from "react";
import { useChat, fetchServerSentEvents } from "@tanstack/ai-react";

const Chat = () => {
  const [input, setInput] = useState("");

  const { messages, sendMessage, addToolApprovalResponse, isLoading } = useChat({
    connection: fetchServerSentEvents("/api/chat"),
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (input.trim() && !isLoading) {
      sendMessage(input);
      setInput("");
    }
  };

  return (
    <div className="flex flex-col h-screen">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4">
        {messages.map((message) => (
          <div
            key={message.id}
            className={`mb-4 ${
              message.role === "assistant" ? "text-blue-600" : "text-gray-800"
            }`}
          >
            <div className="font-semibold mb-1">
              {message.role === "assistant" ? "Assistant" : "You"}
            </div>
            <div>
              {message.parts.map((part, idx) => {
                if (part.type === "thinking") {
                  return (
                    <div
                      key={idx}
                      className="text-sm text-gray-500 italic mb-2"
                    >
                      💭 Thinking: {part.content}
                    </div>
                  );
                }
                if (part.type === "text") {
                  return <div key={idx}>{part.content}</div>;
                }
                if (
                  part.type === "tool-call" &&
                  part.state === "approval-requested" &&
                  part.approval
                ) {
                  return (
                    <div
                      key={idx}
                      className="my-2 p-3 border border-yellow-300 bg-yellow-50 rounded-lg"
                    >
                      <div className="text-sm text-gray-700 mb-2">
                        Tool <span className="font-semibold">{part.toolName}</span> requires approval
                      </div>
                      <div className="flex gap-2">
                        <button
                          onClick={() =>
                            addToolApprovalResponse({
                              id: part.approval!.id,
                              approved: true,
                            })
                          }
                          disabled={isLoading}
                          className="px-4 py-1 bg-green-600 text-white text-sm rounded disabled:opacity-50"
                        >
                          Approve
                        </button>
                        <button
                          onClick={() =>
                            addToolApprovalResponse({
                              id: part.approval!.id,
                              approved: false,
                            })
                          }
                          disabled={isLoading}
                          className="px-4 py-1 bg-red-600 text-white text-sm rounded disabled:opacity-50"
                        >
                          Deny
                        </button>
                      </div>
                    </div>
                  );
                }
                return null;
              })}
            </div>
          </div>
        ))}
      </div>

      {/* Input */}
      <form onSubmit={handleSubmit} className="p-4 border-t">
        <div className="flex gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Type a message..."
            className="flex-1 px-4 py-2 border rounded-lg"
            disabled={isLoading}
          />
          <button
            type="submit"
            disabled={!input.trim() || isLoading}
            className="px-6 py-2 bg-blue-600 text-white rounded-lg disabled:opacity-50"
          >
            Send
          </button>
        </div>
      </form>
    </div>
  );
}

const root = createRoot(document.getElementById('root')!)
root.render(<StrictMode><Chat /></StrictMode>)
