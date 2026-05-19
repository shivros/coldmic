package ports

import "context"

// ConversationResponse is the result of a non-streaming backend call.
type ConversationResponse struct {
	Text      string `json:"text"`
	SessionID string `json:"sessionId"`
	Model     string `json:"model,omitempty"`
}

// StreamChunk is a single chunk from a streaming backend response.
type StreamChunk struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// ConversationBackend sends user text and receives assistant text.
// Implementations must not leak provider-specific details (no HTTP status codes, no raw JSON).
type ConversationBackend interface {
	// SendMessage sends a single user message and returns the assistant response.
	SendMessage(ctx context.Context, sessionID string, text string) (*ConversationResponse, error)

	// StreamMessage sends a user message and returns a channel of response chunks.
	// The channel is closed when the response is complete (final chunk has Done=true).
	StreamMessage(ctx context.Context, sessionID string, text string) (<-chan StreamChunk, error)

	// CloseSession ends a conversation session, releasing any in-memory history.
	CloseSession(ctx context.Context, sessionID string) error
}
