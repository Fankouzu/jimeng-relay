package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunServer_MissingConfig(t *testing.T) {
	// Ensure environment variables are clear
	os.Clearenv()

	// Attempt to run server without config
	err := runServer()

	// Should return error due to missing required config
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required configuration")
}

func TestRun_HelpWithoutConfig(t *testing.T) {
	os.Clearenv()
	var out bytes.Buffer
	err := run([]string{"help"}, &out)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "jimeng-server serve")
}

func TestRun_KeyHelpWithoutConfig(t *testing.T) {
	os.Clearenv()
	var out bytes.Buffer
	err := run([]string{"key", "help"}, &out)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "jimeng-server key create")
}

func TestRun_UnknownCommand(t *testing.T) {
	os.Clearenv()
	var out bytes.Buffer
	err := run([]string{"bad-cmd"}, &out)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown command"))
}
