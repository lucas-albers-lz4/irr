package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLoader(t *testing.T) {
	t.Run("Should return a non-nil Loader", func(t *testing.T) {
		loader := NewLoader()
		assert.NotNil(t, loader, "NewLoader should return a non-nil instance")
		// Optionally, we could assert the type, but NotNil might be sufficient
		// assert.IsType(t, &DefaultLoader{}, loader)
	})
}
