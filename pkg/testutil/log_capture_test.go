package testutil

import (
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/stretchr/testify/assert"
)

func TestCaptureLogOutput(t *testing.T) {
	// Test basic functionality
	output, err := CaptureLogOutput(log.LevelInfo, func() {
		log.Infof("This is an info message")
		log.Debugf("This is a debug message") // Should not be captured at LevelInfo
	})
	assert.NoError(t, err)
	assert.Contains(t, output, "This is an info message")
	assert.NotContains(t, output, "This is a debug message")

	// Test with debug level
	output, err = CaptureLogOutput(log.LevelDebug, func() {
		log.Infof("This is an info message")
		log.Debugf("This is a debug message")
	})
	assert.NoError(t, err)
	assert.Contains(t, output, "This is an info message")
	assert.Contains(t, output, "This is a debug message")

	// Verify original log level is restored
	savedLevel := log.CurrentLevel()
	_, err = CaptureLogOutput(log.LevelDebug, func() {
		// Do nothing, just changing level
	})
	assert.NoError(t, err)
	assert.Equal(t, savedLevel, log.CurrentLevel())
}

func TestContainsLog(t *testing.T) {
	// Test ContainsLog helper
	testOutput := "line 1\nERROR Some error message\nline 3"

	assert.True(t, ContainsLog(testOutput, "ERROR Some error"))
	assert.False(t, ContainsLog(testOutput, "WARNING"))
}
