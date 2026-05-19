# Spec: Conversation Backend

## Purpose

Define the `ConversationBackend` port and implement an OpenAI-compatible HTTP bridge. The bridge pattern ensures new providers can be added without touching core logic.

## ADDED Requirements

### Requirement: ConversationBackend Interface

The `ConversationBackend` interface must be defined in `internal/ports/conversation.go`:

```go
type ConversationBackend interface {
    SendMessage(ctx context.Context, sessionID string, text string) (*ConversationResponse, error)
    StreamMessage(ctx context.Context, sessionID string, text string) (<-chan StreamChunk, error)
    CloseSession(ctx context.Context, sessionID string) error
}
```

Where:
- `sessionID` ties multiple messages into a conversation (maps to provider-specific session/thread if supported)
- `ConversationResponse` contains `Text string`, `SessionID string`, `Model string`
- `StreamChunk` contains `Text string`, `Done bool`

The interface must not leak provider-specific details (no HTTP status codes, no raw JSON).

### Requirement: OpenAI-Compatible Bridge Provider

`internal/providers/openai_conversation.go` implements `ConversationBackend` for any OpenAI-compatible API.

Configuration via environment:
- `COLDMIC_CONVERSATION_BACKEND` — provider name, default `openai`
- `COLDMIC_BACKEND_BASE_URL` — API base URL (e.g. `https://api.openai.com/v1`)
- `COLDMIC_BACKEND_API_KEY` — API key
- `COLDMIC_BACKEND_MODEL` — model name (e.g. `gpt-4o`)
- `COLDMIC_BACKEND_SYSTEM_PROMPT` — system prompt for voice assistant persona
- `COLDMIC_BACKEND_STREAM` — enable streaming responses, default `true`
- `COLDMIC_BACKEND_TIMEOUT` — request timeout, default `30s`

The bridge must:
- Support both streaming (SSE) and non-streaming OpenAI Chat Completion responses
- Map `sessionID` to conversation history (in-memory, per daemon lifecycle)
- Handle rate limits and transient errors with retries (max 3, exponential backoff)
- Strip markdown/formatting from responses before returning (voice assistant context)

### Requirement: Provider Registration

The bootstrap/wire layer (`internal/bootstrap/wire.go`) must select the `ConversationBackend` implementation based on `COLDMIC_CONVERSATION_BACKEND`. Unknown providers fail fast at startup with a clear error message.

### Requirement: Conversation History Management

Each session maintains an in-memory message history (user + assistant turns). History is bounded:
- `COLDMIC_BACKEND_MAX_HISTORY` — max conversation turns retained, default `20`
- Oldest turns are evicted when the limit is reached
- History is lost on daemon restart (acceptable for v1)

#### Scenario: Streaming response with OpenAI-compatible endpoint

Given `COLDMIC_BACKEND_STREAM=true` and a valid base URL + API key
When `StreamMessage` is called with text "What's the weather?"
Then chunks are yielded on the channel as SSE `data:` lines arrive
And the final chunk has `Done=true`
And the full response text is appended to session history

#### Scenario: Non-streaming fallback

Given `COLDMIC_BACKEND_STREAM=false`
When `SendMessage` is called with text "Hello"
Then a single `ConversationResponse` is returned after the full response completes
And the response is appended to session history

#### Scenario: Provider auth failure

Given an invalid API key
When any backend method is called
Then the error is returned without retry (auth errors are not transient)
And the error message includes the HTTP status code
