package testutil

import (
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestCaptureLogOutput(t *testing.T) {
	// Test basic functionality (Info level)
	output, err := CaptureLogOutput(log.LevelInfo, func() {
		log.Info("This is an info message")
		log.Debug("This is a debug message") // Should not be captured at LevelInfo
	})
	assert.NoError(t, err)
	assert.Contains(t, output, `msg="This is an info message"`)
	assert.NotContains(t, output, `msg="This is a debug message"`)

	// Test with debug level
	output, err = CaptureLogOutput(log.LevelDebug, func() {
		log.Info("This is an info message")
		log.Debug("This is a debug message")
	})
	assert.NoError(t, err)
	assert.Contains(t, output, `msg="This is an info message"`)
	assert.Contains(t, output, `msg="This is a debug message"`)

	// Verify original log level is restored
	savedLevel := log.CurrentLevel()
	_, err = CaptureLogOutput(log.LevelDebug, func() {
		// Do nothing, just changing level
	})
	assert.NoError(t, err)
	assert.Equal(t, savedLevel, log.CurrentLevel())
}

func TestContainsLog(t *testing.T) {
	// Test ContainsLog helper - This helper might need adjustment for slog format
	testOutput := "time=... level=ERROR msg=\"Some error message\" key=value\nline 3"

	// Test with slog format in mind
	assert.True(t, ContainsLog(testOutput, "level=ERROR"))
	assert.True(t, ContainsLog(testOutput, `msg="Some error message"`))
	assert.False(t, ContainsLog(testOutput, "level=WARNING"))
	assert.False(t, ContainsLog(testOutput, `msg="Another message"`))
}
