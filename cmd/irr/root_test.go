package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRootCommand_NoSubcommand(t *testing.T) {
	cmd := getRootCmd() // Use the helper from test_helpers_test.go
	_, err := executeCommand(cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a subcommand is required")
}

func TestRootCommand_Help(t *testing.T) {
	cmd := getRootCmd()
	output, err := executeCommand(cmd, "help")
	assert.NoError(t, err)
	assert.Contains(t, output, "irr (Image Relocation and Rewrite) is a tool")
}
