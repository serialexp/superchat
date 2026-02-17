package server

import (
	"io"
	"log"
	"os"
	"testing"
)

// TestMain sets up package-level test state once before any test runs.
// This avoids data races from individual tests writing to package-level
// loggers while goroutines from previous tests may still be reading them.
func TestMain(m *testing.M) {
	// Initialize loggers once — no test should modify these after this point
	errorLog = log.New(io.Discard, "ERROR: ", log.LstdFlags)
	debugLog = log.New(io.Discard, "DEBUG: ", log.LstdFlags)
	log.SetOutput(io.Discard)

	os.Exit(m.Run())
}
