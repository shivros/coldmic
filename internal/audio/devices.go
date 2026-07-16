package audio

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// InputDevice describes an audio capture device that can be passed to ffmpeg.
type InputDevice struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"is_default"`
}

// ListInputDevices enumerates input devices through ffmpeg's source listing.
func ListInputDevices(ctx context.Context, command string, inputFormat string) ([]InputDevice, error) {
	if command == "" {
		command = "ffmpeg"
	}
	if inputFormat == "" {
		inputFormat = "pulse"
	}

	cmd := exec.CommandContext(ctx, command, "-hide_banner", "-sources", inputFormat)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list %s input devices with %s: %w: %s", inputFormat, command, err, stringsTrimSpaceSafe(output.String()))
	}

	devices := ParseInputDevices(output.String())
	if len(devices) == 0 {
		return nil, fmt.Errorf("no %s input devices reported by %s", inputFormat, command)
	}
	return devices, nil
}

var indexedDeviceRE = regexp.MustCompile(`^\[(\d+)\]\s*(.+)$`)

// ParseInputDevices parses ffmpeg -sources output into stable device records.
func ParseInputDevices(raw string) []InputDevice {
	var devices []InputDevice
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ":") || strings.HasPrefix(line, "Auto-detected") || strings.HasPrefix(line, "Cannot list") {
			continue
		}

		isDefault := strings.HasPrefix(line, "*")
		line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if line == "" {
			continue
		}

		index := len(devices)
		indexed := false
		if matches := indexedDeviceRE.FindStringSubmatch(line); matches != nil {
			indexed = true
			parsed, err := strconv.Atoi(matches[1])
			if err == nil {
				index = parsed
			}
			line = strings.TrimSpace(matches[2])
		}

		name, description := splitDeviceLine(line)
		if indexed && description != "" && !strings.Contains(line, "[") {
			name = line
			description = ""
		}
		if name == "" {
			continue
		}
		devices = append(devices, InputDevice{
			Index:       index,
			Name:        name,
			Description: description,
			Default:     isDefault || name == "default",
		})
	}
	return devices
}

func splitDeviceLine(line string) (string, string) {
	if open := strings.Index(line, "["); open > 0 && strings.HasSuffix(line, "]") {
		return strings.TrimSpace(line[:open]), strings.TrimSpace(strings.TrimSuffix(line[open+1:], "]"))
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", ""
	}
	name := fields[0]
	description := strings.TrimSpace(strings.TrimPrefix(line, name))
	return name, description
}

// ResolveInputDevice returns a configured device when available, otherwise default.
func ResolveInputDevice(devices []InputDevice, selected string) InputDevice {
	selected = strings.TrimSpace(selected)
	if selected != "" {
		if idx, err := strconv.Atoi(selected); err == nil {
			for _, device := range devices {
				if device.Index == idx {
					return device
				}
			}
		}
		for _, device := range devices {
			if device.Name == selected {
				return device
			}
		}
	}
	for _, device := range devices {
		if device.Default {
			return device
		}
	}
	if len(devices) > 0 {
		return devices[0]
	}
	return InputDevice{Name: "default", Default: true}
}
