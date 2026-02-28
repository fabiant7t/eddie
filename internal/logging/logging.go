package logging

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"

	ansiSolarizedBase01 = "\x1b[38;2;88;110;117m"
	ansiSolarizedBase1  = "\x1b[38;2;147;161;161m"
	ansiSolarizedBlue   = "\x1b[38;2;38;139;210m"
	ansiSolarizedGreen  = "\x1b[38;2;133;153;0m"
	ansiSolarizedYellow = "\x1b[38;2;181;137;0m"
	ansiSolarizedRed    = "\x1b[38;2;220;50;47m"
	ansiSolarizedViolet = "\x1b[38;2;108;113;196m"
)

// NewLogger creates a slog logger and enables colorized key=value formatting
// only when output is a color-capable terminal.
func NewLogger(level slog.Level, output *os.File) *slog.Logger {
	if output == nil {
		output = os.Stderr
	}
	return NewLoggerWithWriter(level, output, terminalSupportsColor(output))
}

// NewLoggerWithWriter creates a slog logger for any io.Writer and applies
// colorization according to the explicit toggle.
func NewLoggerWithWriter(level slog.Level, output io.Writer, colorize bool) *slog.Logger {
	if output == nil {
		output = os.Stderr
	}

	out := output
	if colorize {
		out = &keyValueColorWriter{target: output}
	}

	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
		Level: level,
	}))
}

type keyValueColorWriter struct {
	target io.Writer
	pending []byte
}

func (w *keyValueColorWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)

	for {
		newlineIndex := bytes.IndexByte(w.pending, '\n')
		if newlineIndex < 0 {
			break
		}

		line := w.pending[:newlineIndex+1]
		colored := colorizeSlogTextLine(line)
		n, err := w.target.Write(colored)
		if err != nil {
			return 0, err
		}
		if n != len(colored) {
			return 0, io.ErrShortWrite
		}

		w.pending = w.pending[newlineIndex+1:]
	}

	return len(p), nil
}

func colorizeSlogTextLine(line []byte) []byte {
	var out bytes.Buffer

	for i := 0; i < len(line); {
		if isWhitespace(line[i]) {
			out.WriteByte(line[i])
			i++
			continue
		}

		start := i
		inQuote := false
		escaped := false
		for i < len(line) {
			ch := line[i]
			if inQuote {
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == '"' {
					inQuote = false
				}
				i++
				continue
			}
			if ch == '"' {
				inQuote = true
				i++
				continue
			}
			if isWhitespace(ch) {
				break
			}
			i++
		}

		out.Write(colorizeToken(line[start:i]))
	}

	return out.Bytes()
}

func colorizeToken(token []byte) []byte {
	eqIndex := bytes.IndexByte(token, '=')
	if eqIndex <= 0 {
		return token
	}

	key := token[:eqIndex]
	if !isKeyToken(key) {
		return token
	}

	value := token[eqIndex+1:]
	var out bytes.Buffer
	out.WriteString(ansiSolarizedBase1)
	out.Write(key)
	out.WriteString(ansiReset)
	out.WriteString(ansiSolarizedBase1)
	out.WriteByte('=')
	out.WriteString(ansiReset)
	out.Write(colorizeValueForKey(string(key), value))

	return out.Bytes()
}

func isKeyToken(token []byte) bool {
	if len(token) == 0 {
		return false
	}
	for _, ch := range token {
		if !isKeyChar(ch) {
			return false
		}
	}
	return true
}

func isKeyChar(ch byte) bool {
	if ch >= 'a' && ch <= 'z' {
		return true
	}
	if ch >= 'A' && ch <= 'Z' {
		return true
	}
	if ch >= '0' && ch <= '9' {
		return true
	}
	return ch == '_' || ch == '.' || ch == '-'
}

func colorizeValueForKey(key string, value []byte) []byte {
	switch key {
	case "level":
		level := strings.ToUpper(strings.Trim(string(value), `"`))
		switch level {
		case "DEBUG":
			return wrapWithANSI(value, ansiSolarizedBase01)
		case "INFO":
			return wrapWithANSI(value, ansiSolarizedBlue)
		case "WARN":
			return wrapWithANSI(value, ansiBold+ansiSolarizedYellow)
		case "ERROR":
			return wrapWithANSI(value, ansiBold+ansiSolarizedRed)
		default:
			return wrapWithANSI(value, ansiSolarizedViolet)
		}
	case "result":
		result := strings.ToUpper(strings.Trim(string(value), `"`))
		if result == "SUCCESS" {
			return wrapWithANSI(value, ansiSolarizedGreen)
		}
		return wrapWithANSI(value, ansiSolarizedRed)
	case "name":
		return wrapWithANSI(value, ansiSolarizedYellow)
	default:
		return value
	}
}

func wrapWithANSI(value []byte, prefix string) []byte {
	var out bytes.Buffer
	out.WriteString(prefix)
	out.Write(value)
	out.WriteString(ansiReset)
	return out.Bytes()
}

func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func terminalSupportsColor(file *os.File) bool {
	if file == nil {
		return false
	}

	isTTY := term.IsTerminal(int(file.Fd()))
	if !isTTY {
		// Fallback for edge cases where fd-based detection is unavailable.
		if stat, err := file.Stat(); err == nil {
			isTTY = stat.Mode()&os.ModeCharDevice != 0
		}
	}

	return shouldColorizeOutput(
		isTTY,
		os.Getenv("TERM"),
		os.Getenv("NO_COLOR"),
		os.Getenv("CLICOLOR"),
		os.Getenv("CLICOLOR_FORCE"),
	)
}

func shouldColorizeOutput(isTTY bool, term, noColor, clicolor, clicolorForce string) bool {
	if noColor != "" {
		return false
	}
	if clicolor == "0" {
		return false
	}
	if clicolorForce != "" && clicolorForce != "0" {
		return true
	}
	if !isTTY {
		return false
	}
	return term != "" && term != "dumb"
}
