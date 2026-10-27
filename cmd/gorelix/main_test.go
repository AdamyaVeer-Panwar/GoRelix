package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestExecutableStarts(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("executable failed: %v\nOutput: %s", err, output)
	}

	if !strings.Contains(string(output), "GoRelix") {
		t.Fatalf("expected output to identify GoRelix, got: %s", output)
	}
}

func TestExecutableRejectsUnknownCommand(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "unknown-command")
	output, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatalf("expected non-zero exit status, got output: %s", output)
	}

	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("expected process exit error, got: %T", err)
	}

	if !strings.Contains(string(output), "unknown command") {
		t.Fatalf("expected explanatory error, got: %s", output)
	}
}

func TestProcessEnvironmentIsAvailable(t *testing.T) {
	// The executable relies on the operating system process environment.
	// This test ensures the test process can inspect it.
	if _, ok := os.LookupEnv("PATH"); !ok {
		t.Fatal("expected PATH to be available in the test environment")
	}
}