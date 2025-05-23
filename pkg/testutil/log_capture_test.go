package testutil

import (
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/log"
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

	t.Run("Handles function error", func(_ *testing.T) {
		// CaptureLogOutput itself doesn't propagate errors from the testFunc
		// This test confirms CaptureLogOutput returns nil error even if testFunc panics/errors
		_, err := CaptureLogOutput(log.LevelDebug, func() {
			panic("test panic")
		})
		// We expect CaptureLogOutput to recover from the panic and return an error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "panic during log capture: test panic")
	})
}

func TestCaptureJSONLogs(t *testing.T) {
	t.Run("Captures JSON logs correctly", func(t *testing.T) {
		_, logs, err := CaptureJSONLogs(log.LevelInfo, func() {
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
		_, logs, err := CaptureJSONLogs(log.LevelDebug, func() {
			// No logging occurs
		})
		require.NoError(t, err)
		assert.Empty(t, logs)
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

func TestContainsAll(t *testing.T) {
	tests := []struct {
		name     string
		actual   map[string]interface{}
		expected map[string]interface{}
		want     bool
	}{
		{
			name:     "Exact match",
			actual:   map[string]interface{}{"level": "INFO", "msg": "test", "count": 10.0},
			expected: map[string]interface{}{"level": "INFO", "msg": "test", "count": 10},
			want:     true,
		},
		{
			name:     "Partial match (expected is subset)",
			actual:   map[string]interface{}{"level": "INFO", "msg": "test", "count": 10.0, "extra": true},
			expected: map[string]interface{}{"level": "INFO", "count": 10},
			want:     true,
		},
		{
			name:     "Empty expected map",
			actual:   map[string]interface{}{"level": "INFO", "msg": "test"},
			expected: map[string]interface{}{},
			want:     true,
		},
		{
			name:     "Empty actual map",
			actual:   map[string]interface{}{},
			expected: map[string]interface{}{"level": "INFO"},
			want:     false,
		},
		{
			name:     "Key missing in actual",
			actual:   map[string]interface{}{"level": "INFO"},
			expected: map[string]interface{}{"level": "INFO", "msg": "test"},
			want:     false,
		},
		{
			name:     "Value mismatch",
			actual:   map[string]interface{}{"level": "WARN", "msg": "test"},
			expected: map[string]interface{}{"level": "INFO", "msg": "test"},
			want:     false,
		},
		{
			name:     "Type mismatch (float vs string)",
			actual:   map[string]interface{}{"level": "INFO", "count": 10.0},
			expected: map[string]interface{}{"level": "INFO", "count": "10"},
			want:     false,
		},
		{
			name:     "Type mismatch (string vs float)",
			actual:   map[string]interface{}{"level": "INFO", "count": "10"},
			expected: map[string]interface{}{"level": "INFO", "count": 10.0},
			want:     false,
		},
		{
			name:     "Int64 comparison",
			actual:   map[string]interface{}{"level": "INFO", "large_count": float64(123456789012345)},
			expected: map[string]interface{}{"level": "INFO", "large_count": int64(123456789012345)},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAll(tt.actual, tt.expected)
			assert.Equal(t, tt.want, got)
		})
	}
}
