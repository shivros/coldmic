import './style.css';
import './app.css';

import { EventsOn } from '../wailsjs/runtime/runtime';
import { AbortPTT, GetRuntimeInfo, GetStatus, StartPTT, StopPTT } from '../wailsjs/go/main/App';
import { formatErrorMessage } from './ui_errors';

const app = document.querySelector('#app');

app.innerHTML = `
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

const statusPill = document.getElementById('status-pill');
const statusMessage = document.getElementById('status-message');
const liveTranscript = document.getElementById('live-transcript');
const finalTranscript = document.getElementById('final-transcript');
const pttButton = document.getElementById('ptt-button');
const abortButton = document.getElementById('abort-button');
const errorEl = document.getElementById('error');
const historyList = document.getElementById('history-list');
const metaEl = document.getElementById('meta');

let currentState = 'idle';
let holdPointer = false;
let holdSpace = false;
let transitionLock = false;

function updateStatus(state, message = '') {
  currentState = state;
  statusPill.className = `status-pill state-${state}`;

  if (state === 'recording') {
    statusPill.textContent = 'RECORDING';
  } else if (state === 'stopping') {
    statusPill.textContent = 'TRANSCRIBING';
  } else if (state === 'error') {
    statusPill.textContent = 'ERROR';
  } else {
    statusPill.textContent = 'MIC COLD';
  }

  statusMessage.textContent = message || statusPill.textContent;
}

function showError(message) {
  errorEl.textContent = message || '';
}

function addHistory(text) {
  const cleaned = text.trim();
  if (!cleaned) {
    return;
  }
  const item = document.createElement('li');
  item.textContent = cleaned;
  historyList.prepend(item);

  while (historyList.children.length > 8) {
    historyList.removeChild(historyList.lastChild);
  }
}

async function startRecording() {
  if (transitionLock || currentState === 'recording' || currentState === 'stopping') {
    return;
  }

  transitionLock = true;
  showError('');
  liveTranscript.textContent = 'Listening...';

  try {
    const status = await StartPTT();
    updateStatus(status.state || 'recording');
  } catch (err) {
    showError(err?.message || String(err));
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
    const result = await StopPTT();
    const transformed = result?.finalTranscript || '';
    if (transformed) {
      finalTranscript.textContent = transformed;
      addHistory(transformed);
    }
  } catch (err) {
    showError(err?.message || String(err));
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

pttButton.addEventListener('pointerdown', (event) => {
  event.preventDefault();
  beginHold();
});
pttButton.addEventListener('pointerup', endHold);
pttButton.addEventListener('pointercancel', endHold);
pttButton.addEventListener('pointerleave', endHold);

abortButton.addEventListener('click', async () => {
  showError('');
  holdPointer = false;
  holdSpace = false;
  try {
    await AbortPTT();
    updateStatus('idle', 'Recording discarded');
    liveTranscript.textContent = 'Waiting for speech...';
  } catch (err) {
    showError(err?.message || String(err));
  }
});

window.addEventListener('keydown', (event) => {
  if (event.code !== 'Space' || event.repeat) {
    return;
  }
  const tag = document.activeElement?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA') {
    return;
  }
  event.preventDefault();
  holdSpace = true;
  void startRecording();
});

window.addEventListener('keyup', (event) => {
  if (event.code !== 'Space') {
    return;
  }
  if (!holdSpace) {
    return;
  }
  holdSpace = false;
  void stopRecording();
});

EventsOn('coldmic:session', (payload) => {
  const data = payload || {};
  updateStatus(data.state || 'idle', data.message || '');
  if ((data.state || '') === 'idle') {
    liveTranscript.textContent = 'Waiting for speech...';
  }
});

EventsOn('coldmic:partial', (payload) => {
  const data = payload || {};
  const text = (data.text || '').trim();
  if (text) {
    liveTranscript.textContent = text;
  }
});

EventsOn('coldmic:final', (payload) => {
  const data = payload || {};
  const transformed = (data.transformed || '').trim();
  if (transformed) {
    finalTranscript.textContent = transformed;
    addHistory(transformed);
  }
});

EventsOn('coldmic:error', (payload) => {
  const data = payload || {};
  const message = formatErrorMessage(data);
  showError(message);
  if (currentState !== 'recording') {
    updateStatus('error', data.message || 'Error');
  }
});

async function hydrate() {
  try {
    const [status, info] = await Promise.all([GetStatus(), GetRuntimeInfo()]);
    updateStatus(status.state || 'idle', status.message || 'Mic cold');

    const parts = [
      `Provider: ${info.provider || 'n/a'}`,
      `Model: ${info.model || 'n/a'}`,
      `Input: ${info.audioInput || 'default'} (${info.audioInputFormat || 'pulse'})`,
      `Rules: ${info.rulesFile || 'none'}`,
    ];
    if (info.error) {
      parts.unshift(`Startup error: ${info.error}`);
      showError(info.error);
    }

    metaEl.innerHTML = parts.map((line) => `<p>${line}</p>`).join('');
  } catch (err) {
    showError(err?.message || String(err));
  }
}

void hydrate();
