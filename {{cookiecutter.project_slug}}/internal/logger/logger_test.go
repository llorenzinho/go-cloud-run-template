package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"{{ cookiecutter.module_path }}/internal/logger"
)

func TestJSONForCloudLogging(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.WithWriter(&buf), logger.WithLevel("debug"), logger.WithAttrs(slog.String("version", "v1")))
	log.Warn("careful", "k", 1)

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "WARNING", line["severity"])
	assert.Equal(t, "careful", line["message"])
	assert.Equal(t, "v1", line["version"])
	assert.EqualValues(t, 1, line["k"])
}

func TestLevelFilter(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.WithWriter(&buf), logger.WithLevel("warn"))
	log.Info("hidden")
	assert.Empty(t, buf.String())

	logger.New(logger.WithWriter(&buf), logger.WithLevel("nope")).Info("shown")
	assert.Contains(t, buf.String(), "shown", "unknown level falls back to info")
}

func TestTextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger.New(logger.WithWriter(&buf), logger.WithFormat("TEXT")).Info("hello")
	assert.Contains(t, buf.String(), "msg=hello")
}
