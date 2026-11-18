package logging

import (
	"bytes"
	"strings"
	"testing"
)

func newTest() (*Logger, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	l := New(&out, &errOut)
	l.color = false
	return l, &out, &errOut
}

func TestLevelsRouteToStreams(t *testing.T) {
	l, out, errOut := newTest()

	l.Info("hello")
	l.Warn("careful")
	l.Error("boom")

	if got := out.String(); got != "[APP] [INFO] hello\n" {
		t.Errorf("stdout: %q", got)
	}
	if got := errOut.String(); got != "[APP] [WARN] careful\n[APP] [ERROR] boom\n" {
		t.Errorf("stderr: %q", got)
	}
}

func TestDebugToggle(t *testing.T) {
	l, out, _ := newTest()

	l.DebugEnabled = false
	l.Debug("hidden")
	if out.Len() != 0 {
		t.Errorf("debug should be suppressed: %q", out.String())
	}

	l.DebugEnabled = true
	l.Debug("shown")
	if !strings.Contains(out.String(), "[DEBUG] shown") {
		t.Errorf("debug missing: %q", out.String())
	}
}

func TestNoDoubleTagging(t *testing.T) {
	l, out, _ := newTest()
	l.Info("[info] already tagged")
	if got := out.String(); got != "[info] already tagged\n" {
		t.Errorf("got %q", got)
	}
}

func TestToggleAlert(t *testing.T) {
	l, out, _ := newTest()

	l.ToggleAlert("CAPTURING", true)
	l.ToggleAlert("CAPTURING", false)

	if got := out.String(); got != "CAPTURING\r         \r" {
		t.Errorf("got %q", got)
	}
}

func TestColorWrapsLine(t *testing.T) {
	var out bytes.Buffer
	l := New(&out, &out)
	l.Info("x")
	if got := out.String(); got != ansiBlue+"[APP] [INFO] x"+ansiReset+"\n" {
		t.Errorf("got %q", got)
	}
}
