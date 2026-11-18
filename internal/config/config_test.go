package config

import (
	"os"
	"path/filepath"
	"testing"
)

type nopLog struct{}

func (nopLog) Info(string) {}
func (nopLog) Warn(string) {}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "AppConfig.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := LoadFile(writeConfig(t, `{}`), nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "en" || cfg.ModelType != ModelBase || cfg.WhisperThreads != 8 ||
		cfg.FrameLength != 512 || cfg.CaptureTimeoutSeconds != 60 {
		t.Errorf("defaults wrong: %+v", cfg)
	}
	if len(cfg.Outputs) != 1 || cfg.Outputs[0] != OutputConsole {
		t.Errorf("default outputs: %v", cfg.Outputs)
	}
	if cfg.HasHTTPSource() {
		t.Error("http source must be off when httpPort unset")
	}
}

func TestLoadParsesEnumsCaseInsensitively(t *testing.T) {
	cfg, err := LoadFile(writeConfig(t, `{
		"outputs": ["console", "ydotool", "CLIPBOARD"],
		"modelType": "small",
		"httpPort": 5042,
		"deviceIndex": -1
	}`), nopLog{})
	if err != nil {
		t.Fatal(err)
	}

	want := []OutputTarget{OutputConsole, OutputYdotool, OutputClipboard}
	for i, w := range want {
		if cfg.Outputs[i] != w {
			t.Errorf("outputs[%d] = %q, want %q", i, cfg.Outputs[i], w)
		}
	}
	if cfg.ModelType != ModelSmall {
		t.Errorf("modelType = %q", cfg.ModelType)
	}
	if !cfg.HasHTTPSource() {
		t.Error("http source should be enabled")
	}
	if cfg.DeviceIndex == nil || *cfg.DeviceIndex != -1 {
		t.Errorf("deviceIndex = %v", cfg.DeviceIndex)
	}
}

func TestLoadParsesTextTransforms(t *testing.T) {
	cfg, err := LoadFile(writeConfig(t, `{"textTransforms": [
		{"description": "d", "find": "a", "replace": "b", "caseSensitive": true, "isRegex": true}
	]}`), nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.TextTransforms[0]
	if got.Find != "a" || got.Replace != "b" || !got.CaseSensitive || !got.IsRegex || got.Description != "d" {
		t.Errorf("got %+v", got)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	tests := map[string]string{
		"empty":          "  \n",
		"invalid json":   `{`,
		"unknown model":  `{"modelType": "Huge"}`,
		"unknown output": `{"outputs": ["Printer"]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFile(writeConfig(t, body), nopLog{}); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestLoadCreatesModelDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "models", "nested")
	cfg, err := LoadFile(writeConfig(t, `{"modelDir": "`+filepath.ToSlash(dir)+`"}`), nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelDir != dir {
		t.Errorf("modelDir = %q, want %q", cfg.ModelDir, dir)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Errorf("model dir not created: %v", err)
	}
}

func TestExpandPath(t *testing.T) {
	t.Setenv("HOME", "/home/test")

	tests := map[string]string{
		"~":          "/home/test",
		"~/models":   "/home/test/models",
		"/abs/path":  "/abs/path",
		"rel/path":   "rel/path",
		"":           "",
		"~other/dir": "~other/dir",
	}
	for in, want := range tests {
		if got := ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepoAppConfigLoads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := LoadFile("../../AppConfig.json", nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TextTransforms) == 0 || len(cfg.Outputs) != 2 || cfg.Outputs[1] != OutputYdotool {
		t.Errorf("unexpected repo config: %+v", cfg)
	}
}
