// Package config loads and validates config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"voxpipe/internal/text"
)

// FileName is the config file name searched for in each location.
const FileName = "config.toml"

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

func (m *ModelKind) UnmarshalText(b []byte) error {
	v, err := parseEnum(string(b), modelKinds, "modelType")
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

func (o *OutputTarget) UnmarshalText(b []byte) error {
	v, err := parseEnum(string(b), outputTargets, "outputs")
	*o = v
	return err
}

// parseEnum matches s case-insensitively against the allowed values.
func parseEnum[T ~string](s string, allowed []T, field string) (T, error) {
	for _, a := range allowed {
		if strings.EqualFold(s, string(a)) {
			return a, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("%s: unknown value %q", field, s)
}

// AppConfig is the DTO for config.toml. Unknown keys are rejected so typos surface.
type AppConfig struct {
	// DeviceIndex of -1 (or unset) selects the system default device.
	DeviceIndex *int   `toml:"deviceIndex"`
	DeviceName  string `toml:"deviceName"`
	HTTPPort    *int   `toml:"httpPort"`

	Language  string    `toml:"language"`
	ModelType ModelKind `toml:"modelType"`
	// ModelDir is where model files are stored/loaded. A relative path is relative to the config
	// file's directory. Defaults to the XDG data directory.
	ModelDir string `toml:"modelDir"`

	// WhisperThreads is the number of threads Whisper uses for inference.
	WhisperThreads int `toml:"whisperThreads"`
	// FrameLength is the number of audio frames per read from the microphone.
	FrameLength int `toml:"frameLength"`

	// TextTransforms are ordered rules applied to transcribed text.
	TextTransforms []text.TransformConfig `toml:"textTransforms"`

	Outputs []OutputTarget `toml:"outputs"`

	// CaptureTimeoutSeconds is the max capture time before auto-stop.
	CaptureTimeoutSeconds int `toml:"captureTimeoutSeconds"`
}

// HasHTTPSource reports whether the HTTP command source is enabled.
func (c *AppConfig) HasHTTPSource() bool {
	return c.HTTPPort != nil && *c.HTTPPort > 0
}

// Defaults returns an AppConfig holding the built-in defaults.
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

// DefaultModelDir is $XDG_DATA_HOME/voxpipe/models, or ~/.local/share/voxpipe/models.
func DefaultModelDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "voxpipe", "models"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory for default modelDir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "voxpipe", "models"), nil
}

// Find locates the config file. An explicit path (from --config) must exist. Otherwise the
// search order is ./config.toml, then $XDG_CONFIG_HOME/voxpipe/config.toml
// (~/.config/voxpipe/config.toml).
func Find(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config file: %w", err)
		}
		return explicit, nil
	}

	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, FileName))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "voxpipe", FileName))
	}

	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s not found; looked in: %s (copy config.example.toml to one of these)",
		FileName, strings.Join(candidates, ", "))
}

// Load finds and loads the config file; see Find for the search order.
func Load(explicit string, log Logger) (*AppConfig, error) {
	path, err := Find(explicit)
	if err != nil {
		return nil, err
	}
	log.Info("Using config: " + path)
	return LoadFile(path, log)
}

// LoadFile reads, decodes and validates the config at path. A missing modelDir is created.
func LoadFile(path string, log Logger) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("%s is empty", filepath.Base(path))
	}

	cfg := Defaults()
	dec := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		var derr *toml.DecodeError
		var serr *toml.StrictMissingError
		switch {
		case errors.As(err, &derr):
			return nil, fmt.Errorf("parse %s:\n%s", path, derr.String())
		case errors.As(err, &serr):
			return nil, fmt.Errorf("parse %s:\n%s", path, serr.String())
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if strings.TrimSpace(cfg.ModelDir) == "" {
		if cfg.ModelDir, err = DefaultModelDir(); err != nil {
			return nil, err
		}
	}

	full, err := resolveModelDir(cfg.ModelDir, path)
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

	return &cfg, nil
}

// resolveModelDir expands "~" and makes the result absolute. A relative path is taken
// relative to the directory of the config file, not the process working directory, so the
// result does not depend on where the binary is launched from.
func resolveModelDir(modelDir, configPath string) (string, error) {
	p := ExpandPath(modelDir)
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}

	base, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return "", err
	}
	return filepath.Join(base, p), nil
}
