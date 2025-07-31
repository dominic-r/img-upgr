package version

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestGetVersion(t *testing.T) {
	// Save original value to restore later
	originalVersion := Version
	defer func() { Version = originalVersion }()

	testCases := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "default value",
			version:  "dev",
			expected: "dev",
		},
		{
			name:     "specific version",
			version:  "1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "empty version",
			version:  "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test version
			Version = tc.version

			// Call function
			result := GetVersion()

			// Check result
			if result != tc.expected {
				t.Errorf("GetVersion() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGetInfo(t *testing.T) {
	// Save original values to restore later
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	// Set test values
	Version = "1.0.0"
	Commit = "abc123"
	BuildDate = "2025-01-01"

	// Expected output with current runtime info
	expected := fmt.Sprintf(
		"Version: 1.0.0, Commit: abc123, Build Date: 2025-01-01, Go: %s, OS/Arch: %s/%s",
		runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)

	// Call function
	result := GetInfo()

	// Check result
	if result != expected {
		t.Errorf("GetInfo() = %q, want %q", result, expected)
	}
}

func TestPrint(t *testing.T) {
	// Save original values to restore later
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	defer func() {
		Version = originalVersion
		Commit = originalCommit
		BuildDate = originalBuildDate
	}()

	// Set test values
	Version = "2.0.0"
	Commit = "def456"
	BuildDate = "2025-02-02"

	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	Print()

	// Restore stdout
	if err := w.Close(); err != nil {
		t.Fatalf("Failed to close pipe writer: %v", err)
	}
	os.Stdout = oldStdout

	// Read captured output
	var output strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := r.Read(buf)
		if err != nil || n == 0 {
			break
		}
		output.Write(buf[:n])
	}

	// Expected lines
	expectedLines := []string{
		"img-upgr version: 2.0.0",
		"Commit: def456",
		"Build Date: 2025-02-02",
		"Go Version: " + runtime.Version(),
		fmt.Sprintf("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH),
	}

	// Check each expected line is in the output
	outputStr := output.String()
	for _, line := range expectedLines {
		if !strings.Contains(outputStr, line) {
			t.Errorf("Print() output missing expected line: %q", line)
		}
	}
}

// TestVersionVariables tests that the version variables are initialized with expected default values
func TestVersionVariables(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Default Version = %q, want %q", Version, "dev")
	}

	if Commit != "none" {
		t.Errorf("Default Commit = %q, want %q", Commit, "none")
	}

	if BuildDate != "unknown" {
		t.Errorf("Default BuildDate = %q, want %q", BuildDate, "unknown")
	}
}
