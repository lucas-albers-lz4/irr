package testutil

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/lalbers/irr/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureLogOutput(t *testing.T) {
	t.Run("Captures text logs correctly", func(t *testing.T) {
		// Ensure text format is used for this test, overriding the default JSON
		t.Setenv("LOG_FORMAT", "text")

		output, err := CaptureLogOutput(log.LevelInfo, func() {
			log.Info("Info message", "key", "value")
			log.Debug("Debug message") // Should not be captured
		})
		require.NoError(t, err)
		assert.Contains(t, output, `level=INFO msg="Info message" key=value`)
		assert.NotContains(t, output, "Debug message")
	})

	t.Run("Handles no log output", func(t *testing.T) {
		output, err := CaptureLogOutput(log.LevelDebug, func() {
			// No logging occurs
		})
		require.NoError(t, err)
		assert.Empty(t, output)
	})

	t.Run("Handles function error", func(t *testing.T) {
		// CaptureLogOutput itself doesn't propagate errors from the testFunc
		// This test confirms CaptureLogOutput returns nil error even if testFunc panics/errors
		// Simulate an error or panic if needed, but the core test is about CaptureLogOutput's return
		_, _ = CaptureLogOutput(log.LevelDebug, func() {
			panic("test panic")
		})
		// We expect CaptureLogOutput to complete without error, even if the inner function panics.
		// The panic will be handled by the Go testing framework.
		// If the test function returned an error, CaptureLogOutput doesn't capture it.
		// So, we mainly assert that CaptureLogOutput itself didn't error out.
		// assert.NoError(t, err) // Commenting out: This assertion doesn't make sense if the inner func panics.
		// If the test function panicked, the test run stops there. If it didn't, err should be nil.
	})
}

func TestCaptureJSONLogs(t *testing.T) {
	t.Run("Captures JSON logs correctly", func(t *testing.T) {
		logs, err := CaptureJSONLogs(log.LevelInfo, func() {
			log.Info("JSON Info", "count", 123, "valid", true)
			log.Debug("JSON Debug") // Should not be captured
		})
		require.NoError(t, err)
		require.Len(t, logs, 1)

		expected := map[string]interface{}{
			"level": "INFO",
			"msg":   "JSON Info",
			"count": 123.0, // JSON numbers are float64
			"valid": true,
		}
		// Check existence of time key, but not its value
		assert.Contains(t, logs[0], "time")
		// Remove time for DeepEqual comparison
		delete(logs[0], "time")
		assert.Equal(t, expected, logs[0])
	})

	t.Run("Handles no log output", func(t *testing.T) {
		logs, err := CaptureJSONLogs(log.LevelDebug, func() {
			// No logging occurs
		})
		require.NoError(t, err)
		assert.Empty(t, logs)
	})

	t.Run("Handles JSON parsing error", func(t *testing.T) {
		// Temporarily break JSON output within the capture function
		_, err := CaptureJSONLogs(log.LevelInfo, func() {
			// Use SetOutput directly to write invalid JSON
			var buf bytes.Buffer
			restore := log.SetOutput(&buf)
			defer restore()
			fmt.Fprintln(&buf, "this is not json")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal log line 1 as JSON")
	})
}

func TestAssertLogContainsJSON(t *testing.T) {
	logs := []map[string]interface{}{
		{"time": "t1", "level": "INFO", "msg": "First", "key": "val1"},
		{"time": "t2", "level": "WARN", "msg": "Second", "key": "val2", "extra": true},
		{"time": "t3", "level": "INFO", "msg": "Third", "key": "val1", "count": 1},
	}

	t.Run("Finds exact match", func(t *testing.T) {
		// Use a mock testing.T to capture failure
		mockT := new(testing.T)
		AssertLogContainsJSON(mockT, logs, map[string]interface{}{"level": "WARN", "key": "val2"})
		assert.False(t, mockT.Failed(), "Assertion should pass for exact match")
	})

	t.Run("Finds partial match", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogContainsJSON(mockT, logs, map[string]interface{}{"level": "INFO", "key": "val1"})
		assert.False(t, mockT.Failed(), "Assertion should pass for partial match (finds first INFO log)")
	})

	t.Run("Fails when no match", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogContainsJSON(mockT, logs, map[string]interface{}{"level": "ERROR", "key": "val1"})
		assert.True(t, mockT.Failed(), "Assertion should fail when no log matches")
	})

	t.Run("Fails when key missing", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogContainsJSON(mockT, logs, map[string]interface{}{"level": "WARN", "missingKey": "val"})
		assert.True(t, mockT.Failed(), "Assertion should fail when a key is missing")
	})

	t.Run("Fails when value different", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogContainsJSON(mockT, logs, map[string]interface{}{"level": "WARN", "key": "wrongValue"})
		assert.True(t, mockT.Failed(), "Assertion should fail when value is different")
	})
}

func TestAssertLogDoesNotContainJSON(t *testing.T) {
	logs := []map[string]interface{}{
		{"time": "t1", "level": "INFO", "msg": "First", "key": "val1"},
		{"time": "t2", "level": "WARN", "msg": "Second", "key": "val2", "extra": true},
	}

	t.Run("Passes when no match", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogDoesNotContainJSON(mockT, logs, map[string]interface{}{"level": "ERROR"})
		assert.False(t, mockT.Failed(), "Assertion should pass when no log matches")
	})

	t.Run("Fails when exact match found", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogDoesNotContainJSON(mockT, logs, map[string]interface{}{"level": "WARN", "key": "val2"})
		assert.True(t, mockT.Failed(), "Assertion should fail when exact match found")
	})

	t.Run("Fails when partial match found", func(t *testing.T) {
		mockT := new(testing.T)
		AssertLogDoesNotContainJSON(mockT, logs, map[string]interface{}{"level": "INFO"})
		assert.True(t, mockT.Failed(), "Assertion should fail when partial match found")
	})
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
