import './style.css';
import './app.css';

import { EventsOn } from '../wailsjs/runtime/runtime';
import { AbortPTT, GetRuntimeInfo, GetStatus, StartPTT, StopPTT } from '../wailsjs/go/main/App';
import { createAppController, renderApp } from './app_controller';
import { formatErrorMessage } from './ui_errors';

const app = document.querySelector('#app');
const elements = renderApp(app);

const controller = createAppController({
  elements,
  formatErrorMessage,
  api: {
    AbortPTT,
    GetRuntimeInfo,
    GetStatus,
    StartPTT,
    StopPTT,
  },
});

elements.pttButton.addEventListener('pointerdown', (event) => {
  event.preventDefault();
  controller.beginHold();
});

elements.pttButton.addEventListener('pointerup', () => {
  controller.endHold();
});

elements.pttButton.addEventListener('pointercancel', () => {
  controller.endHold();
});

elements.pttButton.addEventListener('pointerleave', () => {
  controller.endHold();
});

elements.abortButton.addEventListener('click', async () => {
  await controller.abortRecording();
});

window.addEventListener('keydown', (event) => {
  const shouldPreventDefault = controller.onSpaceKeyDown({
    code: event.code,
    repeat: event.repeat,
    activeTagName: document.activeElement?.tagName,
  });

  if (shouldPreventDefault) {
    event.preventDefault();
  }
});

window.addEventListener('keyup', (event) => {
  const shouldPreventDefault = controller.onSpaceKeyUp({
    code: event.code,
  });

  if (shouldPreventDefault) {
    event.preventDefault();
  }
});

EventsOn('coldmic:session', controller.onSession);
EventsOn('coldmic:partial', controller.onPartial);
EventsOn('coldmic:final', controller.onFinal);
EventsOn('coldmic:error', controller.onError);

void controller.hydrate();
