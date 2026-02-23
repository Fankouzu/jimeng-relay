package cmd

import (
	"bytes"
	"testing"
)

func TestRootHelp(t *testing.T) {
	rootCmd := RootCmd()
	b := bytes.NewBufferString("")
	rootCmd.SetOut(b)
	rootCmd.SetArgs([]string{"--help"})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatal(err)
	}

	out := b.String()
	subcommands := []string{"submit", "query", "wait", "download"}
	for _, sub := range subcommands {
		if !contains(out, sub) {
			t.Errorf("expected help output to contain %s", sub)
		}
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
