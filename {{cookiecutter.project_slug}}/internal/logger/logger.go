// Package logger builds the process slog.Logger. Production logs are JSON
// lines on stdout, which Cloud Logging parses into structured entries.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Formats.
const (
	FormatJSON = "json"
	FormatText = "text"
)

type options struct {
	level  slog.Level
	format string
	w      io.Writer
	attrs  []slog.Attr
}

// Option configures New.
type Option func(*options)

// WithLevel sets the minimum level from its name (debug|info|warn|error);
// unknown names fall back to info.
func WithLevel(name string) Option {
	return func(o *options) {
		if err := o.level.UnmarshalText([]byte(name)); err != nil {
			o.level = slog.LevelInfo
		}
	}
}

// WithFormat selects json (default) or text output.
func WithFormat(format string) Option {
	return func(o *options) { o.format = strings.ToLower(format) }
}

// WithWriter sets the destination (default os.Stdout).
func WithWriter(w io.Writer) Option {
	return func(o *options) { o.w = w }
}

// WithAttrs adds attributes to every record, e.g. the service version.
func WithAttrs(attrs ...slog.Attr) Option {
	return func(o *options) { o.attrs = append(o.attrs, attrs...) }
}

// New returns a logger: JSON on stdout at info level unless options say otherwise.
func New(opts ...Option) *slog.Logger {
	o := options{level: slog.LevelInfo, format: FormatJSON, w: os.Stdout}
	for _, opt := range opts {
		opt(&o)
	}
	hopts := &slog.HandlerOptions{Level: o.level, ReplaceAttr: cloudLogging}
	var h slog.Handler = slog.NewJSONHandler(o.w, hopts)
	if o.format == FormatText {
		h = slog.NewTextHandler(o.w, &slog.HandlerOptions{Level: o.level})
	}
	return slog.New(h.WithAttrs(o.attrs))
}

// cloudLogging renames the level and message keys to the ones Cloud Logging
// reads (severity, message), so log levels show up in the console.
func cloudLogging(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		a.Key = "severity"
		if l, ok := a.Value.Any().(slog.Level); ok && l == slog.LevelWarn {
			a.Value = slog.StringValue("WARNING")
		}
	case slog.MessageKey:
		a.Key = "message"
	}
	return a
}
