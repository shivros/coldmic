package audio

import "testing"

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
