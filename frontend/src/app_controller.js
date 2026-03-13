export const APP_TEMPLATE = `
  <main class="shell">
    <section class="panel">
      <header class="panel__header">
        <p class="eyebrow">Intentional Voice Interface</p>
        <h1>ColdMic</h1>
        <p class="subhead">Press-and-hold to transcribe with deterministic substitutions.</p>
      </header>

      <div class="status-row">
        <div class="status-pill" id="status-pill">MIC COLD</div>
        <div class="status-message" id="status-message">Mic cold</div>
      </div>

      <section class="live-block">
        <h2>Live Transcript</h2>
        <p id="live-transcript" class="live-text">Waiting for speech...</p>
      </section>

      <section class="final-block">
        <h2>Final Output</h2>
        <p id="final-transcript" class="final-text">No transcript yet.</p>
      </section>

      <section class="controls">
        <button id="ptt-button" class="ptt-button" type="button">Hold To Talk</button>
        <button id="abort-button" class="abort-button" type="button">Discard</button>
      </section>

      <section class="meta" id="meta"></section>
      <p id="error" class="error"></p>
    </section>

    <aside class="history-panel">
      <h2>Session History</h2>
      <ul id="history-list"></ul>
    </aside>
  </main>
`;

function normalizeError(err) {
  return err?.message || String(err);
}

function getDoc(elements) {
  return elements.historyList?.ownerDocument || document;
}

export function renderApp(rootEl) {
  if (!rootEl) {
    throw new Error('Missing root app element');
  }

  rootEl.innerHTML = APP_TEMPLATE;
  const doc = rootEl.ownerDocument;

  return {
    statusPill: doc.getElementById('status-pill'),
    statusMessage: doc.getElementById('status-message'),
    liveTranscript: doc.getElementById('live-transcript'),
    finalTranscript: doc.getElementById('final-transcript'),
    pttButton: doc.getElementById('ptt-button'),
    abortButton: doc.getElementById('abort-button'),
    errorEl: doc.getElementById('error'),
    historyList: doc.getElementById('history-list'),
    metaEl: doc.getElementById('meta'),
  };
}

export function createAppController({ elements, api, formatErrorMessage, historyLimit = 8 }) {
  let currentState = 'idle';
  let holdPointer = false;
  let holdSpace = false;
  let transitionLock = false;

  function updateStatus(state, message = '') {
    currentState = state;
    elements.statusPill.className = `status-pill state-${state}`;

    if (state === 'recording') {
      elements.statusPill.textContent = 'RECORDING';
    } else if (state === 'stopping') {
      elements.statusPill.textContent = 'TRANSCRIBING';
    } else if (state === 'error') {
      elements.statusPill.textContent = 'ERROR';
    } else {
      elements.statusPill.textContent = 'MIC COLD';
    }

    elements.statusMessage.textContent = message || elements.statusPill.textContent;
  }

  function showError(message) {
    elements.errorEl.textContent = message || '';
  }

  function addHistory(text) {
    const cleaned = String(text || '').trim();
    if (!cleaned) {
      return;
    }

    const item = getDoc(elements).createElement('li');
    item.textContent = cleaned;
    elements.historyList.prepend(item);

    while (elements.historyList.children.length > historyLimit) {
      elements.historyList.removeChild(elements.historyList.lastChild);
    }
  }

  async function startRecording() {
    if (transitionLock || currentState === 'recording' || currentState === 'stopping') {
      return;
    }

    transitionLock = true;
    showError('');
    elements.liveTranscript.textContent = 'Listening...';

    try {
      const status = await api.StartPTT();
      updateStatus(status?.state || 'recording');
    } catch (err) {
      showError(normalizeError(err));
      updateStatus('error', 'Could not start recording');
    } finally {
      transitionLock = false;
    }
  }

  async function stopRecording() {
    if (transitionLock || currentState !== 'recording') {
      return;
    }

    transitionLock = true;
    showError('');

    try {
      const result = await api.StopPTT();
      const transformed = result?.finalTranscript || '';
      if (transformed) {
        elements.finalTranscript.textContent = transformed;
        addHistory(transformed);
      }
    } catch (err) {
      showError(normalizeError(err));
      updateStatus('error', 'Stop failed');
    } finally {
      transitionLock = false;
    }
  }

  function beginHold() {
    holdPointer = true;
    void startRecording();
  }

  function endHold() {
    if (!holdPointer) {
      return;
    }

    holdPointer = false;
    void stopRecording();
  }

  async function abortRecording() {
    showError('');
    holdPointer = false;
    holdSpace = false;

    try {
      await api.AbortPTT();
      updateStatus('idle', 'Recording discarded');
      elements.liveTranscript.textContent = 'Waiting for speech...';
    } catch (err) {
      showError(normalizeError(err));
    }
  }

  function onSpaceKeyDown({ code, repeat, activeTagName }) {
    if (code !== 'Space' || repeat) {
      return false;
    }

    if (activeTagName === 'INPUT' || activeTagName === 'TEXTAREA') {
      return false;
    }

    holdSpace = true;
    void startRecording();
    return true;
  }

  function onSpaceKeyUp({ code }) {
    if (code !== 'Space') {
      return false;
    }

    if (!holdSpace) {
      return false;
    }

    holdSpace = false;
    void stopRecording();
    return true;
  }

  function onSession(payload) {
    const data = payload || {};
    updateStatus(data.state || 'idle', data.message || '');

    if ((data.state || '') === 'idle') {
      elements.liveTranscript.textContent = 'Waiting for speech...';
    }
  }

  function onPartial(payload) {
    const data = payload || {};
    const text = (data.text || '').trim();
    if (text) {
      elements.liveTranscript.textContent = text;
    }
  }

  function onFinal(payload) {
    const data = payload || {};
    const transformed = (data.transformed || '').trim();
    if (transformed) {
      elements.finalTranscript.textContent = transformed;
      addHistory(transformed);
    }
  }

  function onError(payload) {
    const data = payload || {};
    showError(formatErrorMessage(data));

    if (currentState !== 'recording') {
      updateStatus('error', data.message || 'Error');
    }
  }

  async function hydrate() {
    try {
      const [status, info] = await Promise.all([api.GetStatus(), api.GetRuntimeInfo()]);
      updateStatus(status?.state || 'idle', status?.message || 'Mic cold');

      const parts = [
        `Provider: ${info?.provider || 'n/a'}`,
        `Model: ${info?.model || 'n/a'}`,
        `Input: ${info?.audioInput || 'default'} (${info?.audioInputFormat || 'pulse'})`,
        `Rules: ${info?.rulesFile || 'none'}`,
      ];

      if (info?.error) {
        parts.unshift(`Startup error: ${info.error}`);
        showError(info.error);
      }

      elements.metaEl.innerHTML = parts.map((line) => `<p>${line}</p>`).join('');
    } catch (err) {
      showError(normalizeError(err));
    }
  }

  function getStateSnapshot() {
    return {
      currentState,
      holdPointer,
      holdSpace,
      transitionLock,
    };
  }

  return {
    updateStatus,
    showError,
    addHistory,
    startRecording,
    stopRecording,
    beginHold,
    endHold,
    abortRecording,
    onSpaceKeyDown,
    onSpaceKeyUp,
    onSession,
    onPartial,
    onFinal,
    onError,
    hydrate,
    getStateSnapshot,
  };
}
