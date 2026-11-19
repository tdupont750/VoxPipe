// Package config loads and validates AppConfig.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"voxpipe/internal/text"
)

// ModelKind is the Whisper model size.
type ModelKind string

const (
	ModelBase   ModelKind = "Base"
	ModelTiny   ModelKind = "Tiny"
	ModelSmall  ModelKind = "Small"
	ModelMedium ModelKind = "Medium"
	ModelLarge  ModelKind = "Large"
)

var modelKinds = []ModelKind{ModelBase, ModelTiny, ModelSmall, ModelMedium, ModelLarge}

func (m *ModelKind) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, modelKinds, "modelType")
	*m = v
	return err
}

// OutputTarget is a destination for transcribed text.
type OutputTarget string

const (
	OutputConsole   OutputTarget = "Console"
	OutputClipboard OutputTarget = "Clipboard"
	OutputYdotool   OutputTarget = "Ydotool"
)

var outputTargets = []OutputTarget{OutputConsole, OutputClipboard, OutputYdotool}

func (o *OutputTarget) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, outputTargets, "outputs")
	*o = v
	return err
}

// parseEnum decodes a JSON string and matches it case-insensitively against the allowed values.
func parseEnum[T ~string](b []byte, allowed []T, field string) (T, error) {
	var zero T
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return zero, fmt.Errorf("%s: expected a string: %w", field, err)
	}
	for _, a := range allowed {
		if strings.EqualFold(s, string(a)) {
			return a, nil
		}
	}
	return zero, fmt.Errorf("%s: unknown value %q", field, s)
}

// AppConfig is the DTO for AppConfig.json. encoding/json matches keys case-insensitively.
type AppConfig struct {
	// DeviceIndex of -1 (or unset) selects the system default device.
	DeviceIndex *int   `json:"deviceIndex"`
	DeviceName  string `json:"deviceName"`
	HTTPPort    *int   `json:"httpPort"`

	Language  string    `json:"language"`
	ModelType ModelKind `json:"modelType"`
	// ModelDir is the directory where model files are stored/loaded.
	ModelDir string `json:"modelDir"`

	// WhisperThreads is the number of threads Whisper uses for inference.
	WhisperThreads int `json:"whisperThreads"`
	// FrameLength is the number of audio frames per read from the microphone.
	FrameLength int `json:"frameLength"`

	// TextTransforms are ordered rules applied to transcribed text.
	TextTransforms []text.TransformConfig `json:"textTransforms"`

	Outputs []OutputTarget `json:"outputs"`

	// CaptureTimeoutSeconds is the max capture time before auto-stop.
	CaptureTimeoutSeconds int `json:"captureTimeoutSeconds"`
}

// HasHTTPSource reports whether the HTTP command source is enabled.
func (c *AppConfig) HasHTTPSource() bool {
	return c.HTTPPort != nil && *c.HTTPPort > 0
}

// Defaults returns an AppConfig with the same defaults as the C# AppConfig class.
func Defaults() AppConfig {
	return AppConfig{
		Language:              "en",
		ModelType:             ModelBase,
		WhisperThreads:        8,
		FrameLength:           512,
		Outputs:               []OutputTarget{OutputConsole},
		CaptureTimeoutSeconds: 60,
	}
}

// Logger is the subset of logging.Logger that config loading needs.
type Logger interface {
	Info(msg string)
	Warn(msg string)
}

// ExpandPath expands a leading "~", "~/" or "~\" to the user's home directory.
func ExpandPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}

	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, path[2:])
	}
	return path
}

// Find locates AppConfig.json: first in the current directory, then next to the executable.
func Find() (string, error) {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "AppConfig.json"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "AppConfig.json"))
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("AppConfig.json not found in current working directory or application directory")
}

// Load finds and loads AppConfig.json.
func Load(log Logger) (*AppConfig, error) {
	path, err := Find()
	if err != nil {
		return nil, err
	}
	return LoadFile(path, log)
}

// LoadFile reads, decodes and validates the config at path. A missing modelDir is created.
func LoadFile(path string, log Logger) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, errors.New("AppConfig.json is empty")
	}

	cfg := Defaults()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse AppConfig.json: %w", err)
	}

	if strings.TrimSpace(cfg.ModelDir) != "" {
		full, err := filepath.Abs(ExpandPath(cfg.ModelDir))
		if err != nil {
			return nil, fmt.Errorf("invalid modelDir %q: %w", cfg.ModelDir, err)
		}
		if _, err := os.Stat(full); os.IsNotExist(err) {
			log.Warn("Model directory does not exist: " + full)
			if err := os.MkdirAll(full, 0o755); err != nil {
				return nil, fmt.Errorf("invalid modelDir %q: %w", cfg.ModelDir, err)
			}
			log.Info("Created model directory: " + full)
		}
		cfg.ModelDir = full
	}

	return &cfg, nil
}
