package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestShouldColorizeOutput(t *testing.T) {
	tests := []struct {
		name          string
		isTTY         bool
		term          string
		noColor       string
		clicolor      string
		clicolorForce string
		want          bool
	}{
		{
			name:  "tty and normal terminal",
			isTTY: true,
			term:  "xterm-256color",
			want:  true,
		},
		{
			name:  "non tty",
			isTTY: false,
			term:  "xterm-256color",
			want:  false,
		},
		{
			name:  "dumb terminal",
			isTTY: true,
			term:  "dumb",
			want:  false,
		},
		{
			name:    "no color env disables colors",
			isTTY:   true,
			term:    "xterm-256color",
			noColor: "1",
			want:    false,
		},
		{
			name:     "clicolor zero disables colors",
			isTTY:    true,
			term:     "xterm-256color",
			clicolor: "0",
			want:     false,
		},
		{
			name:          "clicolor force enables colors",
			isTTY:         false,
			term:          "",
			clicolorForce: "1",
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldColorizeOutput(tt.isTTY, tt.term, tt.noColor, tt.clicolor, tt.clicolorForce)
			if got != tt.want {
				t.Fatalf("shouldColorizeOutput() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorizeSlogTextLineColorsKeysAndEquals(t *testing.T) {
	input := []byte("time=2026-02-27T18:46:40Z level=INFO msg=config spec_path=/tmp/spec.yaml\n")
	output := string(colorizeSlogTextLine(input))

	wantPieces := []string{
		ansiSolarizedBase1 + "time" + ansiReset + ansiSolarizedBase1 + "=" + ansiReset,
		ansiSolarizedBase1 + "level" + ansiReset + ansiSolarizedBase1 + "=" + ansiReset,
		ansiSolarizedBase1 + "msg" + ansiReset + ansiSolarizedBase1 + "=" + ansiReset,
		ansiSolarizedBase1 + "spec_path" + ansiReset + ansiSolarizedBase1 + "=" + ansiReset,
	}
	for _, piece := range wantPieces {
		if !strings.Contains(output, piece) {
			t.Fatalf("colorized output missing %q: %q", piece, output)
		}
	}
}

func TestColorizeSlogTextLineDoesNotColorInsideQuotedValues(t *testing.T) {
	input := []byte(`time=2026-02-27T18:46:40Z msg="contains a=b pair" spec=ok` + "\n")
	output := string(colorizeSlogTextLine(input))

	if !strings.Contains(output, `"contains a=b pair"`) {
		t.Fatalf("quoted value content changed unexpectedly: %q", output)
	}
	if strings.Contains(output, ansiSolarizedBase1+"a"+ansiReset+ansiSolarizedBase1+"="+ansiReset+"b") {
		t.Fatalf("unexpected coloring inside quoted value: %q", output)
	}
}

func TestColorizeSlogTextLineColorsLevelBySeverity(t *testing.T) {
	debugLine := string(colorizeSlogTextLine([]byte("level=DEBUG\n")))
	if !strings.Contains(debugLine, ansiSolarizedBase01+"DEBUG"+ansiReset) {
		t.Fatalf("debug level was not colored as expected: %q", debugLine)
	}

	infoLine := string(colorizeSlogTextLine([]byte("level=INFO\n")))
	if !strings.Contains(infoLine, ansiSolarizedBlue+"INFO"+ansiReset) {
		t.Fatalf("info level was not colored as expected: %q", infoLine)
	}

	warnLine := string(colorizeSlogTextLine([]byte("level=WARN\n")))
	if !strings.Contains(warnLine, ansiBold+ansiSolarizedYellow+"WARN"+ansiReset) {
		t.Fatalf("warn level was not colored as expected: %q", warnLine)
	}

	errorLine := string(colorizeSlogTextLine([]byte("level=ERROR\n")))
	if !strings.Contains(errorLine, ansiBold+ansiSolarizedRed+"ERROR"+ansiReset) {
		t.Fatalf("error level was not colored as expected: %q", errorLine)
	}
}

func TestColorizeSlogTextLineColorsResultByOutcome(t *testing.T) {
	successLine := string(colorizeSlogTextLine([]byte("result=success\n")))
	if !strings.Contains(successLine, ansiSolarizedGreen+"success"+ansiReset) {
		t.Fatalf("success result was not colored as expected: %q", successLine)
	}

	failureLine := string(colorizeSlogTextLine([]byte("result=failure\n")))
	if !strings.Contains(failureLine, ansiSolarizedRed+"failure"+ansiReset) {
		t.Fatalf("failure result was not colored as expected: %q", failureLine)
	}
}

func TestColorizeSlogTextLineColorsNameYellow(t *testing.T) {
	nameLine := string(colorizeSlogTextLine([]byte("name=my-spec\n")))
	if !strings.Contains(nameLine, ansiSolarizedYellow+"my-spec"+ansiReset) {
		t.Fatalf("name value was not colored as expected: %q", nameLine)
	}
}

func TestKeyValueColorWriterBuffersUntilNewline(t *testing.T) {
	var out bytes.Buffer
	writer := &keyValueColorWriter{target: &out}

	if n, err := writer.Write([]byte("level=INF")); err != nil || n != len("level=INF") {
		t.Fatalf("first Write() = (%d, %v), want (%d, nil)", n, err, len("level=INF"))
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output before newline, got %q", out.String())
	}

	if n, err := writer.Write([]byte("O\n")); err != nil || n != len("O\n") {
		t.Fatalf("second Write() = (%d, %v), want (%d, nil)", n, err, len("O\n"))
	}

	got := out.String()
	if !strings.Contains(got, ansiSolarizedBase1+"level"+ansiReset+ansiSolarizedBase1+"="+ansiReset) {
		t.Fatalf("buffered line key was not colored as expected: %q", got)
	}
	if !strings.Contains(got, ansiSolarizedBlue+"INFO"+ansiReset) {
		t.Fatalf("buffered line value was not colored as expected: %q", got)
	}
}

func TestKeyValueColorWriterHandlesSplitKeyTokens(t *testing.T) {
	var out bytes.Buffer
	writer := &keyValueColorWriter{target: &out}

	if n, err := writer.Write([]byte("lev")); err != nil || n != len("lev") {
		t.Fatalf("first Write() = (%d, %v), want (%d, nil)", n, err, len("lev"))
	}
	if n, err := writer.Write([]byte("el=ERROR\n")); err != nil || n != len("el=ERROR\n") {
		t.Fatalf("second Write() = (%d, %v), want (%d, nil)", n, err, len("el=ERROR\n"))
	}

	got := out.String()
	if !strings.Contains(got, ansiSolarizedBase1+"level"+ansiReset+ansiSolarizedBase1+"="+ansiReset) {
		t.Fatalf("split key token was not colored as expected: %q", got)
	}
	if !strings.Contains(got, ansiBold+ansiSolarizedRed+"ERROR"+ansiReset) {
		t.Fatalf("split key level value was not colored as expected: %q", got)
	}
}

func TestNewLoggerWithWriterColorToggle(t *testing.T) {
	var coloredOut bytes.Buffer
	coloredLogger := NewLoggerWithWriter(slog.LevelInfo, &coloredOut, true)
	coloredLogger.Info("config", "name", "api-health")
	colored := coloredOut.String()
	if !strings.Contains(colored, ansiSolarizedYellow+"api-health"+ansiReset) {
		t.Fatalf("expected ANSI colorized output, got %q", colored)
	}

	var plainOut bytes.Buffer
	plainLogger := NewLoggerWithWriter(slog.LevelInfo, &plainOut, false)
	plainLogger.Info("config", "name", "api-health")
	plain := plainOut.String()
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("expected plain output without ANSI escapes, got %q", plain)
	}
}
