package pprof

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name         string
		pprofPath    string
		profileType  string
		wantErr      bool
		validateJSON bool
	}{
		{
			name:         "CPU profile analysis",
			pprofPath:    "testdata/profile.pb.gz",
			profileType:  "cpu",
			wantErr:      false,
			validateJSON: true,
		},
		{
			name:         "Invalid profile data",
			pprofPath:    "testdata/invalid.pb.gz",
			profileType:  "cpu",
			wantErr:      true,
			validateJSON: false,
		},
	}

	// Create test data directory
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create test data directory: %v", err)
	}

	// Create invalid profile data
	invalidFilePath := "testdata/invalid.pb.gz"
	if err := os.WriteFile(invalidFilePath, []byte("invalid data"), 0644); err != nil {
		t.Fatalf("Failed to create invalid test file: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if the test file exists
			if _, err := os.Stat(tt.pprofPath); os.IsNotExist(err) && !tt.wantErr {
				t.Skipf("Test data %s not found. Skipping test", tt.pprofPath)
				return
			}

			// Read test file
			var pprofData []byte
			var err error

			// If the test file is invalid, do not read the file
			if tt.pprofPath == invalidFilePath {
				pprofData = []byte("invalid data")
			} else {
				pprofData, err = os.ReadFile(tt.pprofPath)
				if err != nil && !tt.wantErr {
					t.Fatalf("Failed to read test file: %v", err)
				}
			}

			// Execute the Analyze function
			got, err := Analyze(pprofData, tt.profileType)

			// Error verification
			if (err != nil) != tt.wantErr {
				t.Errorf("Analyze() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify JSON and display content in successful cases
			if err == nil && tt.validateJSON {
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(got), &jsonData); err != nil {
					t.Errorf("The returned data is not a valid JSON: %v", err)
					return
				}

				// Display main parts of profile data
				t.Logf("===== Profile analysis result =====")

				// Display metadata
				if meta, ok := jsonData["metadata"].(map[string]interface{}); ok {
					t.Logf("Profile type: %v", meta["profileType"])
					t.Logf("Duration: %v nanoseconds", meta["duration"])
					t.Logf("Measurement unit: %v (%v)", meta["periodType"], meta["periodUnit"])
				}

				// Display detailed stack traces
				if traces, ok := jsonData["stackTraces"].([]interface{}); ok {
					t.Logf("Stack trace count: %d", len(traces))

					// Determine the number of traces to display
					displayCount := len(traces)

					// Display all stack traces
					for i := 0; i < displayCount; i++ {
						trace := traces[i].(map[string]interface{})
						t.Logf("Stack #%d:", i+1)

						if callStack, ok := trace["callStack"].([]interface{}); ok {
							// Display all functions in the call stack
							for j, callObj := range callStack {
								call := callObj.(map[string]interface{})
								funcName := call["function"]
								fileName := call["filename"]
								line := call["line"]

								t.Logf("    %d: %v (%v:%v)", j, funcName, fileName, line)
							}
						} else {
							t.Logf("    (No call stack)")
						}
						t.Logf("-----------")
					}

					// Display the relationship between samples and related stack traces
					if samples, ok := jsonData["samples"].([]interface{}); ok && len(samples) > 0 {
						t.Logf("\n=== Sample and stack trace relationship ===")
						sampleCount := min(5, len(samples)) // Display only the first 5 samples

						for i := 0; i < sampleCount; i++ {
							sample := samples[i].(map[string]interface{})
							if locs, ok := sample["locationIDs"].([]interface{}); ok {
								t.Logf("Sample #%d - Location ID: %v, Value: %v",
									i+1, locs, sample["values"])
							}
						}
					}
				}

				// Display the number of samples
				if samples, ok := jsonData["samples"].([]interface{}); ok {
					t.Logf("Sample count: %d", len(samples))
				}

				// Display the complete JSON data for debugging (optional)
				t.Logf("Complete JSON analysis result:\n%s", got[:min(1000, len(got))]+"...")

				// Basic structure verification
				metadata, ok := jsonData["metadata"].(map[string]interface{})
				if !ok {
					t.Error("JSON does not contain the metadata field")
					return
				}

				if pt, ok := metadata["profileType"].(string); !ok || pt != tt.profileType {
					t.Errorf("profileType does not match. got = %v, want = %v", pt, tt.profileType)
				}

				// Check if stackTraces exist
				if _, ok := jsonData["stackTraces"]; !ok {
					t.Error("JSON does not contain the stackTraces field")
				}

				// Check if samples exist
				if samples, ok := jsonData["samples"]; !ok {
					t.Error("JSON does not contain the samples field")
				} else {
					// Check if samples is an array
					if _, ok := samples.([]interface{}); !ok {
						t.Error("samples field is not an array")
					}
				}
			}
		})
	}
}

// Helper function to copy test pprof file to test data directory
func copyProfileToTestdata(t *testing.T, sourcePath string) {
	t.Helper()

	// Read the profile file
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("Profile file read error: %v", err)
	}

	// Write to the test data directory
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Test data directory creation error: %v", err)
	}

	destPath := "testdata/profile.pb.gz"
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		t.Fatalf("Test data file write error: %v", err)
	}

	t.Logf("Profile copied to test data: %s -> %s", sourcePath, destPath)
}

// Function to generate a test CPU profile
func generateTestCPUProfile(t *testing.T) string {
	t.Helper()

	// Create the test data directory
	testdataDir := "testdata"
	if err := os.MkdirAll(testdataDir, 0755); err != nil {
		t.Fatalf("Test data directory creation error: %v", err)
	}

	profilePath := filepath.Join(testdataDir, "profile.pb.gz")

	// If the profile data already exists, use it
	if _, err := os.Stat(profilePath); err == nil {
		return profilePath
	}

	// Create a temporary Go program to generate the profile
	tempDir, err := os.MkdirTemp("", "pprof-test-*")
	if err != nil {
		t.Fatalf("Temporary directory creation error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple Go program to generate the profile
	progPath := filepath.Join(tempDir, "profile_gen.go")
	progSource := `package main

import (
	"os"
	"runtime/pprof"
)

func main() {
	f, _ := os.Create("` + profilePath + `")
	defer f.Close()
	pprof.StartCPUProfile(f)

	// Process to load CPU
	for i := 0; i < 1000000; i++ {
		_ = i * i
	}

	pprof.StopCPUProfile()
}
`
	if err := os.WriteFile(progPath, []byte(progSource), 0644); err != nil {
		t.Fatalf("Test program creation error: %v", err)
	}

	// Run the program to generate the profile
	cmd := exec.Command("go", "run", progPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Profile generation error: %v\nOutput: %s", err, output)
	}

	t.Logf("Test CPU profile generated: %s", profilePath)
	return profilePath
}
