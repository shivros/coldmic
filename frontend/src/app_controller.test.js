import { beforeEach, describe, expect, it, vi } from 'vitest';

import { createAppController, renderApp } from './app_controller';

function flushPromises() {
  return new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
}

function createHarness(overrides = {}) {
  document.body.innerHTML = '<div id="app"></div>';
  const root = document.getElementById('app');
  const elements = renderApp(root);

  const api = {
    AbortPTT: vi.fn().mockResolvedValue(undefined),
    GetRuntimeInfo: vi.fn().mockResolvedValue({
      provider: 'deepgram',
      model: 'nova-2',
      audioInput: 'default',
      audioInputFormat: 'pulse',
      rulesFile: 'rules.txt',
      error: '',
    }),
    GetStatus: vi.fn().mockResolvedValue({
      state: 'idle',
      message: 'Mic cold',
    }),
    StartPTT: vi.fn().mockResolvedValue({ state: 'recording' }),
    StopPTT: vi.fn().mockResolvedValue({ finalTranscript: 'done text' }),
    ...overrides.api,
  };

  const formatErrorMessage = vi
    .fn()
    .mockImplementation((payload) => payload?.message || 'Unknown error');

  const controller = createAppController({
    elements,
    api,
    formatErrorMessage,
    historyLimit: 3,
  });

  return { api, controller, elements, formatErrorMessage };
}

describe('app controller', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.body.innerHTML = '';
  });

  it('throws when app root element is missing', () => {
    expect(() => renderApp(null)).toThrow('Missing root app element');
  });

  it('updates status text and class for each state', () => {
    const { controller, elements } = createHarness();

    controller.updateStatus('recording');
    expect(elements.statusPill.className).toContain('state-recording');
    expect(elements.statusPill.textContent).toBe('RECORDING');

    controller.updateStatus('stopping');
    expect(elements.statusPill.textContent).toBe('TRANSCRIBING');

    controller.updateStatus('error', 'boom');
    expect(elements.statusPill.textContent).toBe('ERROR');
    expect(elements.statusMessage.textContent).toBe('boom');

    controller.updateStatus('idle');
    expect(elements.statusPill.textContent).toBe('MIC COLD');
  });

  it('adds history entries and enforces history limit', () => {
    const { controller, elements } = createHarness();

    controller.addHistory('first');
    controller.addHistory('second');
    controller.addHistory('third');
    controller.addHistory('fourth');
    controller.addHistory('   ');

    expect(elements.historyList.children).toHaveLength(3);
    expect(elements.historyList.children[0].textContent).toBe('fourth');
    expect(elements.historyList.children[2].textContent).toBe('second');
  });

  it('starts recording and transitions to recording state', async () => {
    const { controller, elements, api } = createHarness();

    await controller.startRecording();

    expect(api.StartPTT).toHaveBeenCalledTimes(1);
    expect(elements.liveTranscript.textContent).toBe('Listening...');
    expect(elements.statusPill.className).toContain('state-recording');
  });

  it('does not start recording when already recording', async () => {
    const { controller, api } = createHarness();

    controller.updateStatus('recording');
    await controller.startRecording();

    expect(api.StartPTT).not.toHaveBeenCalled();
  });

  it('shows an error when starting recording fails', async () => {
    const { controller, elements } = createHarness({
      api: {
        StartPTT: vi.fn().mockRejectedValue(new Error('start failed')),
      },
    });

    await controller.startRecording();

    expect(elements.errorEl.textContent).toBe('start failed');
    expect(elements.statusMessage.textContent).toBe('Could not start recording');
    expect(elements.statusPill.className).toContain('state-error');
  });

  it('stops recording and commits final transcript to history', async () => {
    const { controller, elements, api } = createHarness({
      api: {
        StopPTT: vi.fn().mockResolvedValue({
          finalTranscript: '  final line  ',
          sessionId: 'session-1',
        }),
      },
    });

    controller.updateStatus('recording');
    await controller.stopRecording();

    expect(api.StopPTT).toHaveBeenCalledTimes(1);
    expect(elements.finalTranscript.textContent).toBe('final line');
    expect(elements.historyList.children).toHaveLength(1);
    expect(elements.historyList.children[0].textContent).toBe('final line');
  });

  it('adds one history entry for a completed session when stop result and final event both arrive', async () => {
    const { controller, elements } = createHarness({
      api: {
        StopPTT: vi.fn().mockResolvedValue({
          finalTranscript: 'final line',
          sessionId: 'session-1',
        }),
      },
    });

    controller.updateStatus('recording');
    await controller.stopRecording();
    controller.onFinal({ transformed: ' final line ', sessionId: 'session-1' });

    expect(elements.historyList.children).toHaveLength(1);
    expect(elements.historyList.children[0].textContent).toBe('final line');
  });

  it('deduplicates repeated final events for the same session id', () => {
    const { controller, elements } = createHarness();

    controller.onFinal({ transformed: 'same words', sessionId: 'session-7' });
    controller.onFinal({ transformed: 'same words', sessionId: 'session-7' });

    expect(elements.historyList.children).toHaveLength(1);
    expect(elements.historyList.children[0].textContent).toBe('same words');
  });

  it('keeps separate history entries for different sessions with identical transcript text', () => {
    const { controller, elements } = createHarness();

    controller.onFinal({ transformed: 'same words', sessionId: 'session-8' });
    controller.onFinal({ transformed: 'same words', sessionId: 'session-9' });

    expect(elements.historyList.children).toHaveLength(2);
    expect(elements.historyList.children[0].textContent).toBe('same words');
    expect(elements.historyList.children[1].textContent).toBe('same words');
  });

  it('shows stop error when stopping fails', async () => {
    const { controller, elements } = createHarness({
      api: {
        StopPTT: vi.fn().mockRejectedValue(new Error('stop failed')),
      },
    });

    controller.updateStatus('recording');
    await controller.stopRecording();

    expect(elements.errorEl.textContent).toBe('stop failed');
    expect(elements.statusMessage.textContent).toBe('Stop failed');
  });

  it('does not stop recording when not in recording state', async () => {
    const { controller, api } = createHarness();

    await controller.stopRecording();

    expect(api.StopPTT).not.toHaveBeenCalled();
  });

  it('prevents and handles spacebar press outside text inputs', async () => {
    const { controller, api } = createHarness();

    const shouldPrevent = controller.onSpaceKeyDown({
      code: 'Space',
      repeat: false,
      activeTagName: 'DIV',
    });

    await flushPromises();
    expect(shouldPrevent).toBe(true);
    expect(api.StartPTT).toHaveBeenCalledTimes(1);

    controller.updateStatus('recording');
    const shouldPreventUp = controller.onSpaceKeyUp({ code: 'Space' });
    await flushPromises();

    expect(shouldPreventUp).toBe(true);
    expect(api.StopPTT).toHaveBeenCalledTimes(1);
  });

  it('ignores spacebar interactions in input fields', async () => {
    const { controller, api } = createHarness();

    const shouldPrevent = controller.onSpaceKeyDown({
      code: 'Space',
      repeat: false,
      activeTagName: 'INPUT',
    });

    await flushPromises();
    expect(shouldPrevent).toBe(false);
    expect(api.StartPTT).not.toHaveBeenCalled();

    const shouldPreventUp = controller.onSpaceKeyUp({ code: 'Space' });
    expect(shouldPreventUp).toBe(false);
    expect(api.StopPTT).not.toHaveBeenCalled();
  });

  it('ignores non-space keyboard interactions', async () => {
    const { controller, api } = createHarness();

    const shouldPreventDown = controller.onSpaceKeyDown({
      code: 'Enter',
      repeat: false,
      activeTagName: 'DIV',
    });
    const shouldPreventUp = controller.onSpaceKeyUp({ code: 'Enter' });

    await flushPromises();
    expect(shouldPreventDown).toBe(false);
    expect(shouldPreventUp).toBe(false);
    expect(api.StartPTT).not.toHaveBeenCalled();
    expect(api.StopPTT).not.toHaveBeenCalled();
  });

  it('does not commit empty final transcript payloads', () => {
    const { controller, elements } = createHarness();

    controller.onFinal({ transformed: '   ', sessionId: 'session-empty' });

    expect(elements.historyList.children).toHaveLength(0);
    expect(elements.finalTranscript.textContent).toBe('No transcript yet.');
  });

  it('handles session, partial, final, and error events', () => {
    const { controller, elements, formatErrorMessage } = createHarness({
      api: {
        StartPTT: vi.fn().mockResolvedValue({ state: 'recording' }),
      },
    });

    controller.onSession({ state: 'idle', message: 'Idle now' });
    expect(elements.statusMessage.textContent).toBe('Idle now');
    expect(elements.liveTranscript.textContent).toBe('Waiting for speech...');

    controller.onPartial({ text: ' partial words ' });
    expect(elements.liveTranscript.textContent).toBe('partial words');

    controller.onFinal({ transformed: ' final words ', sessionId: 'session-3' });
    expect(elements.finalTranscript.textContent).toBe('final words');

    controller.onError({ message: 'Network down' });
    expect(formatErrorMessage).toHaveBeenCalledTimes(1);
    expect(elements.errorEl.textContent).toBe('Network down');
    expect(elements.statusPill.className).toContain('state-error');
  });

  it('does not force error status while actively recording', () => {
    const { controller, elements } = createHarness();

    controller.updateStatus('recording');
    controller.onError({ message: 'network blip' });

    expect(elements.statusPill.className).toContain('state-recording');
    expect(elements.errorEl.textContent).toBe('network blip');
  });

  it('hydrates runtime metadata and startup errors', async () => {
    const { controller, elements } = createHarness({
      api: {
        GetStatus: vi.fn().mockResolvedValue({ state: 'idle', message: 'Ready' }),
        GetRuntimeInfo: vi.fn().mockResolvedValue({
          provider: 'deepgram',
          model: 'nova-2',
          audioInput: 'default',
          audioInputFormat: 'pulse',
          rulesFile: 'config.rules',
          error: 'Missing microphone',
        }),
      },
    });

    await controller.hydrate();

    expect(elements.statusMessage.textContent).toBe('Ready');
    expect(elements.metaEl.innerHTML).toContain('Provider: deepgram');
    expect(elements.metaEl.innerHTML).toContain('Rules: config.rules');
    expect(elements.metaEl.innerHTML).toContain('Startup error: Missing microphone');
    expect(elements.errorEl.textContent).toBe('Missing microphone');
  });

  it('surfaces hydrate failures', async () => {
    const { controller, elements } = createHarness({
      api: {
        GetStatus: vi.fn().mockRejectedValue(new Error('status unavailable')),
      },
    });

    await controller.hydrate();

    expect(elements.errorEl.textContent).toBe('status unavailable');
  });

  it('aborts recording and resets state', async () => {
    const { controller, elements, api } = createHarness();

    controller.onSpaceKeyDown({
      code: 'Space',
      repeat: false,
      activeTagName: 'DIV',
    });

    await flushPromises();
    await controller.abortRecording();

    expect(api.AbortPTT).toHaveBeenCalledTimes(1);
    expect(elements.statusMessage.textContent).toBe('Recording discarded');
    expect(elements.liveTranscript.textContent).toBe('Waiting for speech...');
  });

  it('endHold does nothing when hold is not active', async () => {
    const { controller, api } = createHarness();

    controller.endHold();
    await flushPromises();

    expect(api.StopPTT).not.toHaveBeenCalled();
  });

  it('returns controller state snapshot', () => {
    const { controller } = createHarness();

    const snapshot = controller.getStateSnapshot();

    expect(snapshot).toEqual({
      currentState: 'idle',
      holdPointer: false,
      holdSpace: false,
      transitionLock: false,
    });
  });
});
