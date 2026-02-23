package usecase

import (
	"context"

	"coldmic/internal/domain"
	"coldmic/internal/ports"
)

type transcriptFinalizer struct {
	rules     ports.RulesEngine
	clipboard ports.Clipboard
	events    ports.EventSink
}

func newTranscriptFinalizer(rules ports.RulesEngine, clipboard ports.Clipboard, events ports.EventSink) transcriptFinalizer {
	return transcriptFinalizer{rules: rules, clipboard: clipboard, events: events}
}

func (f transcriptFinalizer) Finalize(ctx context.Context, raw string) (domain.StopResult, domain.SessionStateReason, error) {
	transformed, err := f.rules.Apply(raw)
	if err != nil {
		f.events.SessionError(domain.ErrorCodeRules, err.Error())
		return domain.StopResult{}, domain.SessionReasonRulesFailed, err
	}

	result := domain.StopResult{
		RawTranscript:   raw,
		FinalTranscript: transformed,
		Copied:          true,
	}
	reason := domain.SessionReasonTranscriptCopied

	if err := f.clipboard.SetText(ctx, transformed); err != nil {
		result.Copied = false
		reason = domain.SessionReasonTranscriptReadyClipboardFailed
		f.events.SessionError(domain.ErrorCodeClipboard, "transcript ready but clipboard write failed")
	}

	return result, reason, nil
}
