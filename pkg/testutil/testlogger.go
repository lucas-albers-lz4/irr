package testutil

import (
	"io"
	"os"
	"sync"
	"testing"
)

var (
	// originalOut holds the original standard output
	originalOut *os.File
	// originalErr holds the original standard error
	originalErr *os.File
	// mutex protects concurrent access to logger state
	mutex sync.Mutex
)

// SuppressLogging redirects all logging output to /dev/null during test execution
// Call the returned function to restore original logging
func SuppressLogging() func() {
	mutex.Lock()
	defer mutex.Unlock()

	originalOut = os.Stdout
	originalErr = os.Stderr

	null, err := os.Open(os.DevNull)
	if err != nil {
		panic(err)
	}
	os.Stdout = null
	os.Stderr = null

	return func() {
		mutex.Lock()
		defer mutex.Unlock()

		os.Stdout = originalOut
		os.Stderr = originalErr
		err := null.Close()
		if err != nil {
			panic(err)
		}
	}
}

// CaptureLogging captures all logging output during test execution
// Call the returned function to restore original logging and retrieve the captured output
func CaptureLogging() func() (string, string) {
	mutex.Lock()
	defer mutex.Unlock()

	originalOut = os.Stdout
	originalErr = os.Stderr

	outReader, outWriter, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	errReader, errWriter, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	os.Stdout = outWriter
	os.Stderr = errWriter

	outChan := make(chan string)
	errChan := make(chan string)

	go func() {
		outBytes, err := io.ReadAll(outReader)
		if err != nil {
			panic(err)
		}
		outChan <- string(outBytes)
	}()

	go func() {
		errBytes, err := io.ReadAll(errReader)
		if err != nil {
			panic(err)
		}
		errChan <- string(errBytes)
	}()

	return func() (string, string) {
		mutex.Lock()
		defer mutex.Unlock()

		err := outWriter.Close()
		if err != nil {
			panic(err)
		}
		err = errWriter.Close()
		if err != nil {
			panic(err)
		}

		os.Stdout = originalOut
		os.Stderr = originalErr

		return <-outChan, <-errChan
	}
}

// UseTestLogger sets up a logger that only prints output if the test fails
// This should be called at the beginning of a test with defer t.Cleanup(restore)
func UseTestLogger(t *testing.T) func() {
	t.Helper()

	// Only capture output on non-verbose test runs
	if !testing.Verbose() {
		output, errOutput := CaptureLogging()()
		t.Cleanup(func() {
			// Only print the output if the test fails
			if t.Failed() {
				t.Logf("Standard output captured during test:\n%s", output)
				t.Logf("Standard error captured during test:\n%s", errOutput)
			}
		})
	}

	return func() {}
}
