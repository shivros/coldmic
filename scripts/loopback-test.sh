#!/usr/bin/env bash
set -euo pipefail

# ColdMic Loopback Test Harness
# Creates a virtual audio sink, pipes TTS audio through it, and runs
# ColdMic conversation mode against it — fully automated, no mic needed.
#
# Usage:
#   ./scripts/loopback-test.sh [speech_text]
#
# Requires:
#   - PulseAudio/PipeWire (for module-null-sink)
#   - ffmpeg (for flite speech synthesis + ffplay playback)
#   - ColdMic built or buildable from source
#
# Env vars (all optional — sensible defaults provided):
#   COLDMIC_BACKEND_BASE_URL   - LLM backend URL
#   COLDMIC_BACKEND_API_KEY    - LLM API key
#   COLDMIC_BACKEND_MODEL      - LLM model name
#   COLDMIC_TEST_WAIT          - seconds to wait for pipeline (default: 30)
#   COLDMIC_SKIP_BUILD         - if "1", skip building coldmicd

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SINK_NAME="coldmic_loopback"
MONITOR_SOURCE="${SINK_NAME}.monitor"
DAEMON_ADDR="127.0.0.1:4317"
SCRIPT_DIR="${REPO_ROOT}/scripts"

export COLDMIC_DEBUG=1

DAEMON_PID=""
cleanup() {
  local exit_code=$?
  echo "[harness] Cleaning up..."
  if [ -n "${DAEMON_PID}" ] && kill -0 "${DAEMON_PID}" 2>/dev/null; then
    kill "${DAEMON_PID}" 2>/dev/null || true
    wait "${DAEMON_PID}" 2>/dev/null || true
  fi
  exit $exit_code
}
trap cleanup EXIT

# Ensure virtual audio device exists
ensure_sink() {
  if ! pactl list short sinks | grep -q "${SINK_NAME}"; then
    pactl load-module module-null-sink \
      sink_name="${SINK_NAME}" \
      sink_properties=device.description="ColdMic_Test_Loopback" \
      rate=48000 channels=1 2>/dev/null
    echo "[harness] Created virtual sink ${SINK_NAME}"
  else
    echo "[harness] Virtual sink ${SINK_NAME} already exists"
  fi
}

# Generate test audio using KittenTTS (available on endver)
# Falls back to a sine wave if KittenTTS is unavailable
KITTENTTS_PY="${HOME}/Workspaces/KittenTTS/.venv/bin/python"
KITTENTTS_SCRIPT="${HOME}/.hermes/scripts/kittentts_generate.py"

generate_test_audio() {
  local output="$1"
  local text="${2:-Hello Alice, what time is it?}"

  if [ -x "${KITTENTTS_PY}" ] && [ -f "${KITTENTTS_SCRIPT}" ]; then
    echo "[harness] Generating speech via KittenTTS: \"${text}\""
    "${KITTENTTS_PY}" "${KITTENTTS_SCRIPT}" \
      --text "${text}" --output "${output}" --format wav 2>&1 | tail -3
  else
    echo "[harness] KittenTTS unavailable — using 3s sine wave fallback (440Hz)"
    ffmpeg -y -f lavfi -i "sine=frequency=440:duration=3" \
      -ar 48000 -ac 1 -sample_fmt s16 "${output}" 2>/dev/null
  fi
}

# Play audio into the virtual sink
play_audio() {
  local audio_file="$1"
  echo "[harness] Playing audio into ${SINK_NAME}..."
  ffplay -nodisp -autoexit -i "${audio_file}" \
    -output_device "${SINK_NAME}" 2>/dev/null || true
}

# === MAIN ===

SPEECH_TEXT="${1:-Hello Alice, what time is it?}"

ensure_sink

# Build coldmic daemon if needed
if [ "${COLDMIC_SKIP_BUILD:-0}" = "1" ] && [ -x "${REPO_ROOT}/bin/coldmicd" ]; then
  DAEMON_BIN="${REPO_ROOT}/bin/coldmicd"
elif ! [ -x "${REPO_ROOT}/bin/coldmicd" ]; then
  echo "[harness] Building coldmic daemon..."
  (cd "${REPO_ROOT}" && go build -o bin/coldmicd ./cmd/coldmicd/)
  DAEMON_BIN="${REPO_ROOT}/bin/coldmicd"
else
  DAEMON_BIN="${REPO_ROOT}/bin/coldmicd"
fi

echo "[harness] Configuring ColdMic to capture from ${MONITOR_SOURCE}..."

export COLDMIC_AUDIO_INPUT_FORMAT="pulse"
export COLDMIC_AUDIO_INPUT_DEVICE="${MONITOR_SOURCE}"
export COLDMIC_AUDIO_OUTPUT_DEVICE="${SINK_NAME}"

# Conversation mode config
export COLDMIC_BACKEND_BASE_URL="${COLDMIC_BACKEND_BASE_URL:-https://api.openai.com/v1}"
export COLDMIC_BACKEND_API_KEY="${COLDMIC_BACKEND_API_KEY:-dummy}"
export COLDMIC_BACKEND_MODEL="${COLDMIC_BACKEND_MODEL:-gpt-4o}"
export COLDMIC_BACKEND_SYSTEM_PROMPT="${COLDMIC_BACKEND_SYSTEM_PROMPT:-You are a helpful voice assistant.}"
export COLDMIC_WAKE_PHRASES="${COLDMIC_WAKE_PHRASES:-hello alice,alice}"
export COLDMIC_STOP_PHRASES="${COLDMIC_STOP_PHRASES:-thanks alice,goodbye,stop}"
export COLDMIC_VAD_THRESHOLD="${COLDMIC_VAD_THRESHOLD:-0.3}"
export COLDMIC_VAD_SILENCE_MS="${COLDMIC_VAD_SILENCE_MS:-800}"
export COLDMIC_TTS_ENGINE="${COLDMIC_TTS_ENGINE:-none}"
export COLDMIC_CONVERSATION_TIMEOUT="${COLDMIC_CONVERSATION_TIMEOUT:-30s}"
export COLDMIC_LOCAL_STT=""

# Start daemon in background
echo "[harness] Starting ColdMic daemon on ${DAEMON_ADDR}..."
"${DAEMON_BIN}" --addr "${DAEMON_ADDR}" &
DAEMON_PID=$!

# Wait for daemon to be healthy
for i in $(seq 1 10); do
  if curl -sf "http://${DAEMON_ADDR}/v1/session/status" >/dev/null 2>&1; then
    echo "[harness] Daemon healthy after ${i}s"
    break
  fi
  sleep 1
  if [ "$i" = "10" ]; then
    echo "[harness] ERROR: Daemon not responding after 10s"
    exit 1
  fi
done

# Start conversation mode
echo "[harness] Starting conversation mode..."
curl -sf -X POST "http://${DAEMON_ADDR}/v1/conversation/start" >/dev/null 2>&1 || true
sleep 2

# Double-start trick: first call returns stale status (known race documented in skill)
echo "[harness] Confirming conversation is active..."
CONV_STATUS=$(curl -sf "http://${DAEMON_ADDR}/v1/conversation/status" 2>&1 || echo "(error)")
echo "[harness] Conversation status: ${CONV_STATUS}"

# Generate and play test audio
generate_test_audio "/tmp/coldmic_test_input.wav" "${SPEECH_TEXT}"
play_audio "/tmp/coldmic_test_input.wav"

# Wait for the conversation pipeline to process
echo "[harness] Waiting ${COLDMIC_TEST_WAIT:-30}s for pipeline..."
sleep "${COLDMIC_TEST_WAIT:-30}"

# Collect results
echo
echo "=== Transcript ==="
curl -sf "http://${DAEMON_ADDR}/v1/session/transcript/latest" 2>&1 || echo "(no transcript)"
echo
echo "=== Conversation Status ==="
curl -sf "http://${DAEMON_ADDR}/v1/conversation/status" 2>&1 || echo "(no response)"
echo
echo "[harness] Test complete"
