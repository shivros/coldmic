package audio

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseInputDevicesPulseOutput(t *testing.T) {
	raw := `Auto-detected sources for pulse:
* default [Default source]
  alsa_input.pci-0000_00_1f.3.analog-stereo [Built-in Audio Analog Stereo]
  bluez_input.11_22_33_44_55_66.0 [Headset]
`

	devices := ParseInputDevices(raw)
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
	if !devices[0].Default || devices[0].Name != "default" {
		t.Fatalf("expected default device first, got %+v", devices[0])
	}
	if devices[1].Index != 1 || devices[1].Name != "alsa_input.pci-0000_00_1f.3.analog-stereo" || devices[1].Description != "Built-in Audio Analog Stereo" {
		t.Fatalf("unexpected parsed device: %+v", devices[1])
	}
}

func TestParseInputDevicesIndexedOutput(t *testing.T) {
	raw := `Auto-detected sources for avfoundation:
[0] MacBook Pro Microphone
[2] USB Audio Device
`

	devices := ParseInputDevices(raw)
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
	if devices[1].Index != 2 || devices[1].Name != "USB Audio Device" || devices[1].Description != "" {
		t.Fatalf("unexpected indexed device: %+v", devices[1])
	}
}

func TestParseInputDevicesSkipsCannotListDiagnostics(t *testing.T) {
	devices := ParseInputDevices("Auto-detected sources for pulse:\nCannot list sources: Generic error\n")
	if len(devices) != 0 {
		t.Fatalf("expected no devices from diagnostic output, got %+v", devices)
	}
}

func TestListInputDevicesUsesCommandDefaults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper uses POSIX sh")
	}
	fakeFFmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	script := `#!/bin/sh
if [ "$1" != "-hide_banner" ] || [ "$2" != "-sources" ] || [ "$3" != "pulse" ]; then
  echo "unexpected args: $*" >&2
  exit 7
fi
cat <<'EOF'
Auto-detected sources for pulse:
* default [Default source]
  usb-mic [USB Microphone]
EOF
`
	if err := os.WriteFile(fakeFFmpeg, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	devices, err := ListInputDevices(context.Background(), fakeFFmpeg, "")
	if err != nil {
		t.Fatalf("ListInputDevices returned error: %v", err)
	}
	if len(devices) != 2 || devices[1].Name != "usb-mic" || devices[1].Description != "USB Microphone" {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}

func TestListInputDevicesReportsCommandFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper uses POSIX sh")
	}
	fakeFFmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho boom >&2\nexit 9\n"), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	_, err := ListInputDevices(context.Background(), fakeFFmpeg, "alsa")
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "alsa") {
		t.Fatalf("expected command output in error, got %v", err)
	}
}

func TestListInputDevicesReportsEmptySourceList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell helper uses POSIX sh")
	}
	fakeFFmpeg := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(fakeFFmpeg, []byte("#!/bin/sh\necho 'Auto-detected sources for pulse:'\n"), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	_, err := ListInputDevices(context.Background(), fakeFFmpeg, "pulse")
	if err == nil || !strings.Contains(err.Error(), "no pulse input devices") {
		t.Fatalf("expected empty source list error, got %v", err)
	}
}

func TestResolveInputDeviceFallsBackToDefault(t *testing.T) {
	devices := []InputDevice{
		{Index: 0, Name: "default", Default: true},
		{Index: 1, Name: "usb-mic"},
	}
	if got := ResolveInputDevice(devices, "1"); got.Name != "usb-mic" {
		t.Fatalf("expected index lookup, got %+v", got)
	}
	if got := ResolveInputDevice(devices, "missing"); got.Name != "default" {
		t.Fatalf("expected fallback default, got %+v", got)
	}
}

func TestResolveInputDeviceFallsBackToFirstAndSyntheticDefault(t *testing.T) {
	devices := []InputDevice{{Index: 7, Name: "line-in"}}
	if got := ResolveInputDevice(devices, "missing"); got.Name != "line-in" {
		t.Fatalf("expected first device fallback, got %+v", got)
	}
	if got := ResolveInputDevice(nil, "missing"); got.Name != "default" || !got.Default {
		t.Fatalf("expected synthetic default fallback, got %+v", got)
	}
}
