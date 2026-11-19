package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"voxpipe/internal/text"
)

type nopLog struct{}

func (nopLog) Info(string) {}
func (nopLog) Warn(string) {}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// isolate points HOME and the XDG dirs at temp dirs so tests never touch the real ones.
func isolate(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

func TestLoadAppliesDefaults(t *testing.T) {
	isolate(t)
	cfg, err := LoadFile(writeConfig(t, "language = \"en\"\n"), nopLog{})
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

func TestLoadDefaultsModelDirToXDGData(t *testing.T) {
	home := isolate(t)
	cfg, err := LoadFile(writeConfig(t, "language = \"en\"\n"), nopLog{})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(home, ".local", "share", "voxpipe", "models")
	if cfg.ModelDir != want {
		t.Errorf("modelDir = %q, want %q", cfg.ModelDir, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Errorf("default model dir not created: %v", err)
	}
}

func TestLoadParsesEnumsCaseInsensitively(t *testing.T) {
	isolate(t)
	cfg, err := LoadFile(writeConfig(t, `
outputs = ["console", "ydotool", "CLIPBOARD"]
modelType = "small"
httpPort = 5042
deviceIndex = -1
`), nopLog{})
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

func TestLoadTextTransformsKeepLiteralBackslashes(t *testing.T) {
	isolate(t)
	cfg, err := LoadFile(writeConfig(t, `
[[textTransforms]]
description = "d"
find = '\b(gira)\b'
replace = 'Jira'
caseSensitive = true
isRegex = true
`), nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.TextTransforms[0]
	if got.Find != `\b(gira)\b` || got.Replace != "Jira" || !got.CaseSensitive || !got.IsRegex || got.Description != "d" {
		t.Errorf("got %+v", got)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	isolate(t)
	tests := map[string]string{
		"empty":          "  \n",
		"invalid toml":   "outputs = [",
		"unknown model":  `modelType = "Huge"`,
		"unknown output": `outputs = ["Printer"]`,
		"unknown key":    `modelTpye = "Base"`,
		"wrong type":     `whisperThreads = "eight"`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadFile(writeConfig(t, body), nopLog{}); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestLoadUnknownKeyErrorNamesTheKey(t *testing.T) {
	isolate(t)
	_, err := LoadFile(writeConfig(t, `modelTpye = "Base"`), nopLog{})
	if err == nil || !strings.Contains(err.Error(), "modelTpye") {
		t.Errorf("got %v", err)
	}
}

func TestLoadCreatesModelDir(t *testing.T) {
	isolate(t)
	dir := filepath.Join(t.TempDir(), "models", "nested")
	cfg, err := LoadFile(writeConfig(t, "modelDir = '"+dir+"'\n"), nopLog{})
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

func TestFindSearchOrder(t *testing.T) {
	home := isolate(t)
	cwd := t.TempDir()
	t.Chdir(cwd)

	xdg := filepath.Join(home, ".config", "voxpipe")
	if err := os.MkdirAll(xdg, 0o755); err != nil {
		t.Fatal(err)
	}
	xdgFile := filepath.Join(xdg, FileName)

	if _, err := Find(""); err == nil || !strings.Contains(err.Error(), "config.example.toml") {
		t.Errorf("missing config must hint at the example: %v", err)
	}

	os.WriteFile(xdgFile, []byte("x"), 0o644)
	if got, err := Find(""); err != nil || got != xdgFile {
		t.Errorf("xdg fallback: got %q, %v", got, err)
	}

	cwdFile := filepath.Join(cwd, FileName)
	os.WriteFile(cwdFile, []byte("x"), 0o644)
	if got, err := Find(""); err != nil || !samePath(t, got, cwdFile) {
		t.Errorf("cwd must win over xdg: got %q, %v", got, err)
	}

	explicit := filepath.Join(t.TempDir(), "custom.toml")
	os.WriteFile(explicit, []byte("x"), 0o644)
	if got, err := Find(explicit); err != nil || got != explicit {
		t.Errorf("explicit must win: got %q, %v", got, err)
	}
	if _, err := Find(filepath.Join(cwd, "nope.toml")); err == nil {
		t.Error("explicit missing file must fail, not fall through")
	}
}

func samePath(t *testing.T, a, b string) bool {
	t.Helper()
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

func TestExampleConfigLoadsAndRegexesCompile(t *testing.T) {
	isolate(t)
	cfg, err := LoadFile("../../config.example.toml", nopLog{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TextTransforms) == 0 || len(cfg.Outputs) != 2 || cfg.Outputs[1] != OutputYdotool {
		t.Errorf("unexpected example config: %+v", cfg)
	}

	r := text.NewReplacer()
	for _, rule := range cfg.TextTransforms {
		if !rule.IsRegex {
			continue
		}
		if _, err := r.Replace("sample text", rule.Find, rule.Replace, 0); err != nil {
			t.Errorf("rule %q does not compile: %v", rule.Description, err)
		}
	}
}

func TestLoadResolvesRelativeModelDirAgainstConfigFile(t *testing.T) {
	isolate(t)
	cfgDir := t.TempDir()
	t.Chdir(t.TempDir()) // launch directory must not matter

	path := filepath.Join(cfgDir, FileName)
	if err := os.WriteFile(path, []byte("modelDir = 'models/sub'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadFile(path, nopLog{})
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(cfgDir, "models", "sub")
	if cfg.ModelDir != want {
		t.Errorf("modelDir = %q, want %q", cfg.ModelDir, want)
	}
	if st, err := os.Stat(want); err != nil || !st.IsDir() {
		t.Errorf("model dir not created next to config: %v", err)
	}
}

func TestLoadRelativeConfigPathStillResolvesToAbsolute(t *testing.T) {
	isolate(t)
	cfgDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfgDir, FileName), []byte("modelDir = 'm'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cfgDir)

	cfg, err := LoadFile(FileName, nopLog{}) // relative config path
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.ModelDir) || !samePath(t, cfg.ModelDir, filepath.Join(cfgDir, "m")) {
		t.Errorf("modelDir = %q", cfg.ModelDir)
	}
}
