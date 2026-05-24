package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"coldmic/internal/debuglog"
	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

// ConversationControllerConfig controls conversation controller behavior.
type ConversationControllerConfig struct {
	StopPhrases    []string
	SilenceTimeout time.Duration
}

// DefaultConversationConfig returns sensible defaults.
func DefaultConversationConfig() ConversationControllerConfig {
	return ConversationControllerConfig{
		StopPhrases:    []string{"thanks alice", "that's all", "goodbye", "bye alice", "stop"},
		SilenceTimeout: 30 * time.Second,
	}
}

// ConversationController orchestrates the full voice assistant conversation loop
// using a four-state machine: IDLE → LISTENING → PROCESSING → SPEAKING → LISTENING (loop).
//
// It subscribes to the ContinuousListener event channel and coordinates with
// the ConversationBackend and TextToSpeech to complete the conversation cycle.
type ConversationController struct {
	listener *ContinuousListener
	backend  ports.ConversationBackend
	tts      ports.TextToSpeech
	events   ports.EventSink
	cfg      ConversationControllerConfig

	mu         sync.Mutex
	state      domain.ConversationState
	sessionID  string
	running    bool
	cancel     context.CancelFunc
	generation uint64 // incremented on each Start(); deferred cleanup only acts if generation matches

	// silenceTimer tracks the inactivity timeout while in LISTENING state.
	silenceTimer *time.Timer
	silenceCh    <-chan time.Time
}

// NewConversationController creates a new conversation controller.
func NewConversationController(
	listener *ContinuousListener,
	backend ports.ConversationBackend,
	tts ports.TextToSpeech,
	events ports.EventSink,
	cfg ConversationControllerConfig,
) *ConversationController {
	if cfg.SilenceTimeout <= 0 {
		cfg.SilenceTimeout = 30 * time.Second
	}
	if len(cfg.StopPhrases) == 0 {
		cfg.StopPhrases = []string{"thanks alice", "that's all", "goodbye", "bye alice", "stop"}
	}
	return &ConversationController{
		listener: listener,
		backend:  backend,
		tts:      tts,
		events:   events,
		cfg:      cfg,
		state:    domain.ConvStateIdle,
	}
}

// State returns the current conversation state.
func (c *ConversationController) State() domain.ConversationState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Running reports whether the conversation controller loop is active.
func (c *ConversationController) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Status returns the current conversation status.
func (c *ConversationController) Status() domain.ConversationStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return domain.ConversationStatus{
		State:     c.state,
		SessionID: c.sessionID,
		Active:    c.state != domain.ConvStateIdle,
	}
}

// Start begins the conversation controller loop. It subscribes to the
// ContinuousListener's event channel and processes events until the context
// is cancelled or a fatal error occurs.
func (c *ConversationController) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return domain.ErrConversationActive
	}
	ctx, c.cancel = context.WithCancel(ctx)
	c.running = true
	c.state = domain.ConvStateIdle
	c.generation++
	gen := c.generation
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.generation == gen {
			c.running = false
			c.state = domain.ConvStateIdle
			c.sessionID = ""
		}
		c.mu.Unlock()
	}()

	debuglog.Printf("conversation controller starting stop_phrases=%v timeout=%v", c.cfg.StopPhrases, c.cfg.SilenceTimeout)

	events := c.listener.Events()
	for {
		// Capture silenceCh under lock to avoid data race with Stop().
		c.mu.Lock()
		silenceCh := c.silenceCh
		c.mu.Unlock()

		select {
		case <-ctx.Done():
			debuglog.Printf("conversation controller stopped")
			c.emitState(domain.ConvStateIdle, domain.ConvReasonManualStop)
			return ctx.Err()

		case <-silenceCh:
			// Silence timeout fires while in LISTENING state.
			c.mu.Lock()
			if c.state != domain.ConvStateListening {
				c.mu.Unlock()
				continue
			}
			c.silenceTimer = nil
			c.silenceCh = nil
			c.mu.Unlock()

			debuglog.Printf("conversation controller: silence timeout")
			c.transitionToIdle(domain.ConvReasonSilenceTimeout)

		case evt, ok := <-events:
			if !ok {
				debuglog.Printf("conversation controller: listener event channel closed")
				c.transitionToIdle(domain.ConvReasonError)
				return nil
			}
			c.handleEvent(ctx, evt)
		}
	}
}

// Stop terminates the conversation controller loop and returns to IDLE.
func (c *ConversationController) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.running = false
	c.state = domain.ConvStateIdle
	sid := c.sessionID
	c.sessionID = ""
	if c.silenceTimer != nil {
		c.silenceTimer.Stop()
		c.silenceTimer = nil
		c.silenceCh = nil
	}
	c.mu.Unlock()

	// Close backend session to avoid resource leak.
	if sid != "" && c.backend != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = c.backend.CloseSession(ctx, sid)
		cancel()
	}

	// Note: do NOT emit idle event here. The Start() loop's ctx.Done() handler
	// will emit it. Emitting here causes a duplicate event.
}

// handleEvent dispatches a single ListenerEvent based on the current state.
func (c *ConversationController) handleEvent(ctx context.Context, evt ListenerEvent) {
	switch evt.Kind {
	case ListenerEventWakePhrase:
		c.handleWakePhrase(ctx, evt)
	case ListenerEventTranscript:
		c.handleTranscript(ctx, evt)
	case ListenerEventVADSpeechEnd:
		c.handleSpeechEnd(ctx, evt)
	case ListenerEventVADSpeechStart:
		// Reset silence timer on new speech activity.
		c.resetSilenceTimer()
	}
}

// handleWakePhrase processes a wake phrase event: IDLE → LISTENING.
func (c *ConversationController) handleWakePhrase(ctx context.Context, evt ListenerEvent) {
	c.mu.Lock()
	currentState := c.state
	c.mu.Unlock()

	if currentState != domain.ConvStateIdle {
		debuglog.Printf("conversation controller: ignoring wake phrase in state=%s", currentState)
		return
	}

	c.mu.Lock()
	c.sessionID = evt.SessionID
	c.state = domain.ConvStateListening
	c.mu.Unlock()

	c.emitState(domain.ConvStateListening, domain.ConvReasonWakeDetected)
	debuglog.Printf("conversation controller: wake phrase detected session=%s text=%q", evt.SessionID, evt.Text)

	// If the wake phrase includes text (e.g. "Hey Alice, what time is it?"),
	// process it immediately as a user utterance.
	if strings.TrimSpace(evt.Text) != "" {
		c.processUtterance(ctx, evt.Text)
	}

	// Start silence timer.
	c.resetSilenceTimer()
}

// handleTranscript processes a transcript event during an active conversation.
func (c *ConversationController) handleTranscript(ctx context.Context, evt ListenerEvent) {
	c.mu.Lock()
	currentState := c.state
	c.mu.Unlock()

	if currentState != domain.ConvStateListening {
		return
	}

	text := strings.TrimSpace(evt.Text)
	if text == "" {
		return
	}

	// Reset silence timer on new transcript activity.
	c.resetSilenceTimer()

	// Check for stop phrases before sending to backend.
	if c.isStopPhrase(text) {
		debuglog.Printf("conversation controller: stop phrase detected text=%q", text)
		c.transitionToIdle(domain.ConvReasonStopPhrase)
		return
	}

	c.processUtterance(ctx, text)
}

// handleSpeechEnd is called when VAD detects the end of a speech segment.
func (c *ConversationController) handleSpeechEnd(ctx context.Context, evt ListenerEvent) {
	c.mu.Lock()
	currentState := c.state
	c.mu.Unlock()

	// If we're in LISTENING and haven't already transitioned, start silence timer.
	if currentState == domain.ConvStateListening {
		c.resetSilenceTimer()
	}
}

// processUtterance sends user text to the backend and then speaks the response.
// State transitions: LISTENING → PROCESSING → SPEAKING → LISTENING
func (c *ConversationController) processUtterance(ctx context.Context, text string) {
	// LISTENING → PROCESSING
	c.mu.Lock()
	if c.state != domain.ConvStateListening {
		c.mu.Unlock()
		return
	}
	c.state = domain.ConvStateProcessing
	sessionID := c.sessionID
	c.mu.Unlock()

	c.emitState(domain.ConvStateProcessing, domain.ConvReasonSpeechReceived)
	debuglog.Printf("conversation controller: sending to backend session=%s text=%q", sessionID, text)

	// Stop silence timer during processing.
	c.stopSilenceTimer()

	// Guard against nil backend.
	if c.backend == nil {
		debuglog.Printf("conversation controller: no backend configured, returning to listening")
		c.handleError(ctx, "Sorry, I'm not connected right now.")
		return
	}

	// Call the backend.
	resp, err := c.backend.SendMessage(ctx, sessionID, text)
	if err != nil {
		debuglog.Printf("conversation controller: backend error: %v", err)
		// Error recovery: speak error message and return to LISTENING.
		c.handleError(ctx, "Sorry, I had trouble with that. Could you try again?")
		return
	}

	// PROCESSING → SPEAKING
	c.mu.Lock()
	if c.state != domain.ConvStateProcessing {
		c.mu.Unlock()
		return
	}
	c.state = domain.ConvStateSpeaking
	c.mu.Unlock()

	c.emitState(domain.ConvStateSpeaking, domain.ConvReasonBackendResponse)
	debuglog.Printf("conversation controller: speaking response session=%s text=%q", sessionID, resp.Text)

	// Speak the response.
	if c.tts != nil {
		if ttsErr := c.tts.Play(ctx, resp.Text); ttsErr != nil {
			debuglog.Printf("conversation controller: TTS error: %v", ttsErr)
			// Log but continue — skip playback and return to LISTENING.
		}
	}

	// SPEAKING → LISTENING
	c.mu.Lock()
	if c.state != domain.ConvStateSpeaking {
		c.mu.Unlock()
		return
	}
	c.state = domain.ConvStateListening
	c.mu.Unlock()

	c.emitState(domain.ConvStateListening, domain.ConvReasonPlaybackDone)
	debuglog.Printf("conversation controller: playback done, listening for next utterance session=%s", sessionID)

	// Restart silence timer.
	c.resetSilenceTimer()
}

// handleError handles backend errors by speaking an error message and returning to LISTENING.
func (c *ConversationController) handleError(ctx context.Context, message string) {
	c.mu.Lock()
	if c.state != domain.ConvStateProcessing {
		c.mu.Unlock()
		return
	}
	c.state = domain.ConvStateSpeaking
	c.mu.Unlock()

	c.emitState(domain.ConvStateSpeaking, domain.ConvReasonError)

	if c.tts != nil {
		_ = c.tts.Play(ctx, message)
	}

	// SPEAKING → LISTENING (recoverable error)
	c.mu.Lock()
	if c.state != domain.ConvStateSpeaking {
		c.mu.Unlock()
		return
	}
	c.state = domain.ConvStateListening
	c.mu.Unlock()

	c.emitState(domain.ConvStateListening, domain.ConvReasonPlaybackDone)
	c.resetSilenceTimer()
}

// transitionToIdle moves from any state back to IDLE.
func (c *ConversationController) transitionToIdle(reason domain.ConversationStateReason) {
	c.stopSilenceTimer()

	c.mu.Lock()
	c.state = domain.ConvStateIdle
	sid := c.sessionID
	c.sessionID = ""
	c.mu.Unlock()

	// Close backend session if we have one.
	if sid != "" && c.backend != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = c.backend.CloseSession(ctx, sid)
		cancel()
	}

	c.emitState(domain.ConvStateIdle, reason)
	debuglog.Printf("conversation controller: transitioned to idle reason=%s", reason)
}

// isStopPhrase checks if text contains any configured stop phrase (case-insensitive).
func (c *ConversationController) isStopPhrase(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range c.cfg.StopPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// resetSilenceTimer resets the silence timeout timer.
func (c *ConversationController) resetSilenceTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.silenceTimer != nil {
		c.silenceTimer.Stop()
	}
	c.silenceTimer = time.NewTimer(c.cfg.SilenceTimeout)
	c.silenceCh = c.silenceTimer.C
}

// stopSilenceTimer stops the silence timeout timer.
func (c *ConversationController) stopSilenceTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.silenceTimer != nil {
		c.silenceTimer.Stop()
		c.silenceTimer = nil
		c.silenceCh = nil
	}
}

// emitState emits a conversation state change event.
func (c *ConversationController) emitState(state domain.ConversationState, reason domain.ConversationStateReason) {
	c.events.ConversationStateChanged(state, reason)
}

// IsStopPhrase is exported for testing purposes. It checks if the given text
// contains a stop phrase using the controller's configured stop phrases.
func (c *ConversationController) IsStopPhrase(text string) bool {
	return c.isStopPhrase(text)
}

// SetState sets the conversation state directly (for testing).
// Returns an error if the transition is invalid.
func (c *ConversationController) SetState(state domain.ConversationState) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !isValidTransition(c.state, state) {
		return fmt.Errorf("%w: %s → %s", domain.ErrInvalidTransition, c.state, state)
	}
	c.state = state
	return nil
}

// isValidTransition checks if a state transition is allowed.
func isValidTransition(from, to domain.ConversationState) bool {
	switch from {
	case domain.ConvStateIdle:
		return to == domain.ConvStateListening
	case domain.ConvStateListening:
		return to == domain.ConvStateProcessing || to == domain.ConvStateIdle
	case domain.ConvStateProcessing:
		return to == domain.ConvStateSpeaking || to == domain.ConvStateIdle
	case domain.ConvStateSpeaking:
		return to == domain.ConvStateListening || to == domain.ConvStateIdle
	default:
		return false
	}
}

// Transcript returns a formatted description of valid transitions for debugging.
func validTransitions() map[domain.ConversationState][]domain.ConversationState {
	return map[domain.ConversationState][]domain.ConversationState{
		domain.ConvStateIdle:       {domain.ConvStateListening},
		domain.ConvStateListening:  {domain.ConvStateProcessing, domain.ConvStateIdle},
		domain.ConvStateProcessing: {domain.ConvStateSpeaking, domain.ConvStateIdle},
		domain.ConvStateSpeaking:   {domain.ConvStateListening, domain.ConvStateIdle},
	}
}

// Ensure ConversationController satisfies expected interfaces at compile time.
var (
	_ fmt.Stringer = (*conversationStateStringer)(nil)
)

type conversationStateStringer struct{}

func (conversationStateStringer) String() string { return "ConversationController" }

// String returns a human-readable description of the valid state transitions.
func init() {
	// Validate all states and transitions are defined.
	for state, targets := range validTransitions() {
		for _, target := range targets {
			if !isValidTransition(state, target) {
				panic(fmt.Sprintf("BUG: validTransitions claims %s → %s but isValidTransition disagrees", state, target))
			}
		}
	}
}
