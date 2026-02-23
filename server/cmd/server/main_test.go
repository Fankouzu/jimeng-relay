package main

import (
	"os"
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
