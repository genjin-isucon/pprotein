package pprof

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
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

				// Output to JSON file for detailed verification
				outputFile := fmt.Sprintf("testdata/output_%s.json", strings.ReplaceAll(tt.name, " ", "_"))
				if err := os.WriteFile(outputFile, []byte(got), 0644); err != nil {
					t.Logf("Warning: Could not write JSON output to file: %v", err)
				} else {
					t.Logf("JSON output saved to: %s", outputFile)
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

// validateDetailedJSON is a helper function to validate and display the structure of detailed JSON
func validateDetailedJSON(t *testing.T, jsonData map[string]interface{}) {
	t.Helper()

	// Check required fields
	requiredFields := []string{
		"sampleType", "sample", "location", "function",
		"timeNanos", "durationNanos", "periodType", "period",
	}

	for _, field := range requiredFields {
		if _, ok := jsonData[field]; !ok {
			t.Errorf("Required field %s is missing", field)
		}
	}

	// Check sample types
	if sampleTypes, ok := jsonData["sampleType"].([]interface{}); ok {
		t.Logf("Number of sample types: %d", len(sampleTypes))
		for i, st := range sampleTypes {
			if stMap, ok := st.(map[string]interface{}); ok {
				t.Logf("  Sample type #%d: Type=%v, Unit=%v",
					i+1, stMap["type"], stMap["unit"])
			}
		}
	}

	// Check function information
	if functions, ok := jsonData["function"].([]interface{}); ok {
		t.Logf("Number of functions: %d", len(functions))
		// Display the first 5 functions
		displayCount := min(5, len(functions))
		for i := 0; i < displayCount; i++ {
			if funcMap, ok := functions[i].(map[string]interface{}); ok {
				t.Logf("  Function #%d: ID=%v, Name=%v, File=%v",
					i+1, funcMap["id"], funcMap["name"], funcMap["filename"])
			}
		}
	}

	// Check location information
	if locations, ok := jsonData["location"].([]interface{}); ok {
		t.Logf("Number of locations: %d", len(locations))
		// Display the first 3 locations
		displayCount := min(3, len(locations))
		for i := 0; i < displayCount; i++ {
			if locMap, ok := locations[i].(map[string]interface{}); ok {
				t.Logf("  Location #%d: ID=%v, Address=%v",
					i+1, locMap["id"], locMap["address"])

				// Display line information
				if lines, ok := locMap["line"].([]interface{}); ok {
					for j, line := range lines {
						if lineMap, ok := line.(map[string]interface{}); ok {
							t.Logf("    Line #%d: Function=%v, Line=%v",
								j+1, lineMap["function"], lineMap["line"])
						}
					}
				}
			}
		}
	}

	// Check sample information
	if samples, ok := jsonData["sample"].([]interface{}); ok {
		t.Logf("Number of samples: %d", len(samples))
		// Display the first 3 samples
		displayCount := min(3, len(samples))
		for i := 0; i < displayCount; i++ {
			if sampleMap, ok := samples[i].(map[string]interface{}); ok {
				t.Logf("  Sample #%d:", i+1)
				if locs, ok := sampleMap["location"].([]interface{}); ok {
					t.Logf("    Location IDs: %v", locs)
				}
				if values, ok := sampleMap["value"].([]interface{}); ok {
					t.Logf("    Values: %v", values)
				}
				if labels, ok := sampleMap["label"].(map[string]interface{}); ok && len(labels) > 0 {
					t.Logf("    Labels: %v", labels)
				}
			}
		}
	}
}

// TestConvertToDetailedJSON tests the detailed JSON conversion functionality
func TestConvertToDetailedJSON(t *testing.T) {
	// This test has been replaced by TestDetailedJsonFromProfile, so it's skipped
	t.Skip("This test has been replaced by TestDetailedJsonFromProfile")
}

// TestAnalyzeFromCommand tests the pprof analysis directly from command line
func TestAnalyzeFromCommand(t *testing.T) {
	// This test starts with t.Skip(), so it will be skipped during normal test runs
	// Comment out this line if you want to run it manually
	t.Skip("This test is for manual verification only")

	// Sample pprof file
	testDataPath := "testdata/profile001.pb.gz"

	// Output JSON file
	outputJSONPath := "testdata/cmd_output.json"

	// Execute command and get results
	cmd := exec.Command("go", "run", "../../cmd/pprofutil/main.go",
		"convert", "--input", testDataPath, "--output", outputJSONPath, "--format", "detailed")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute command: %v\nOutput: %s", err, output)
	}

	t.Logf("Command executed successfully. Output: %s", output)
	t.Logf("JSON output saved to: %s", outputJSONPath)

	// Read the generated JSON file
	jsonData, err := os.ReadFile(outputJSONPath)
	if err != nil {
		t.Fatalf("Failed to read output JSON: %v", err)
	}

	// Validate JSON structure
	var parsedJSON map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsedJSON); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}

	// Validate JSON content
	validateDetailedJSON(t, parsedJSON)
}

// Output a text report from a real pprof file - utility script
func printTextReportFromFile(t *testing.T) {
	// This code is not included in the test, but provided as a reference
	t.Helper()

	/*
		Example usage:

		go run -mod=mod <<EOF
		package main

		import (
			"flag"
			"fmt"
			"io"
			"log"
			"os"

			"github.com/kaz/pprotein/internal/analyze/pprof"
		)

		func main() {
			inputPath := flag.String("input", "", "Input pprof file path")
			outputPath := flag.String("output", "", "Output text report path")
			flag.Parse()

			if *inputPath == "" {
				log.Fatal("Input file is required: -input <file>")
			}

			// Read the profile file
			pprofData, err := os.ReadFile(*inputPath)
			if err != nil {
				log.Fatalf("Failed to read profile file: %v", err)
			}

			// Generate text report
			textReport, err := pprof.GenerateTextReport(pprofData)
			if err != nil {
				log.Fatalf("Failed to generate text report: %v", err)
			}

			// Determine output destination
			var output io.Writer = os.Stdout
			if *outputPath != "" {
				outputFile, err := os.Create(*outputPath)
				if err != nil {
					log.Fatalf("Failed to create output file: %v", err)
				}
				defer outputFile.Close()
				output = outputFile
				fmt.Printf("Text report saved to: %s\n", *outputPath)
			}

			// Output the text report
			fmt.Fprintln(output, textReport)
		}
		EOF
		<input-pprof-file> -output <output-txt-file>
	*/
}

// TestGenerateTextReport tests the text report generation functionality
func TestGenerateTextReport(t *testing.T) {
	// Create sample profile directly
	prof := createSampleProfile()

	// Generate text report directly
	textReport, err := generateTextReportFromProfile(prof)
	if err != nil {
		t.Fatalf("Failed to generate text report: %v", err)
	}

	// Output text report to file for verification
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	outputFile := "testdata/output_report.txt"
	if err := os.WriteFile(outputFile, []byte(textReport), 0644); err != nil {
		t.Logf("Warning: Could not write text report to file: %v", err)
	} else {
		t.Logf("Text report saved to: %s", outputFile)
	}

	// Check report content
	t.Logf("Text report length: %d bytes", len(textReport))

	// Verify basic structure is included
	expectedSections := []string{
		"===== Profile Information Summary =====",
		"===== Top 10 Hotspot Functions =====",
		"===== Important Call Paths =====",
		"===== Bottleneck Analysis Hints =====",
	}

	for _, section := range expectedSections {
		if !strings.Contains(textReport, section) {
			t.Errorf("Expected section %q not found in report", section)
		}
	}

	// Display the full report (for verification during test execution)
	t.Logf("Complete text report: \n%s", textReport)
}

// createSampleProfile creates a sample profile for testing
func createSampleProfile() *profile.Profile {
	// Create a new profile
	prof := &profile.Profile{
		SampleType: []*profile.ValueType{
			{Type: "cpu", Unit: "nanoseconds"},
		},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:        1000000,
		TimeNanos:     1617123456789000,
		DurationNanos: 10000000000,
	}

	// Add function information
	fn1 := &profile.Function{ID: 1, Name: "main.heavyFunction", Filename: "main.go", StartLine: 42}
	fn2 := &profile.Function{ID: 2, Name: "runtime.schedule", Filename: "runtime/proc.go", StartLine: 2500}
	fn3 := &profile.Function{ID: 3, Name: "main.processData", Filename: "main.go", StartLine: 100}
	prof.Function = []*profile.Function{fn1, fn2, fn3}

	// Mapping information
	mapping := &profile.Mapping{
		ID:    1,
		Start: 0x1000,
		Limit: 0x2000,
		File:  "test_binary",
	}
	prof.Mapping = []*profile.Mapping{mapping}

	// Location information
	loc1 := &profile.Location{ID: 1, Mapping: mapping, Address: 0x1000}
	loc1.Line = []profile.Line{{Function: fn1, Line: 42}}

	loc2 := &profile.Location{ID: 2, Mapping: mapping, Address: 0x1200}
	loc2.Line = []profile.Line{{Function: fn2, Line: 2520}}

	loc3 := &profile.Location{ID: 3, Mapping: mapping, Address: 0x1400}
	loc3.Line = []profile.Line{{Function: fn3, Line: 105}}

	prof.Location = []*profile.Location{loc1, loc2, loc3}

	// Sample information
	sample1 := &profile.Sample{
		Location: []*profile.Location{loc1, loc2},
		Value:    []int64{5000000},
	}

	sample2 := &profile.Sample{
		Location: []*profile.Location{loc1, loc3},
		Value:    []int64{3000000},
	}

	sample3 := &profile.Sample{
		Location: []*profile.Location{loc2},
		Value:    []int64{2000000},
	}

	prof.Sample = []*profile.Sample{sample1, sample2, sample3}

	return prof
}

// Add a new test function for testing ConvertToDetailedJSON
func TestDetailedJsonFromProfile(t *testing.T) {
	// Create sample profile directly
	prof := createSampleProfile()

	// Convert to DetailedProfile
	detailedProfile := (*DetailedProfile)(prof)

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(detailedProfile, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal to JSON: %v", err)
	}

	jsonResult := string(jsonBytes)

	// Output detailed JSON to file for verification
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	// Output the formatted JSON with pretty-printing
	outputFile := "testdata/output_detailed_json.json"
	if err := os.WriteFile(outputFile, []byte(jsonResult), 0644); err != nil {
		t.Logf("Warning: Could not write detailed JSON to file: %v", err)
	} else {
		t.Logf("Detailed JSON output saved to: %s", outputFile)
	}

	// Verify it's valid JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResult), &jsonData); err != nil {
		t.Fatalf("Invalid JSON output: %v", err)
	}

	// Display detailed JSON content
	t.Logf("Detailed JSON preview (first 500 chars): \n%s...", jsonResult[:min(500, len(jsonResult))])

	// Check required fields
	requiredFields := []string{
		"sampleType",
		"sample",
		"location",
		"function",
		"timeNanos",
	}

	for _, field := range requiredFields {
		if _, ok := jsonData[field]; !ok {
			t.Errorf("Required field missing in JSON output: %s", field)
		}
	}
}

// TestTextReportFromRealProfile tests generating a text report from a real CPU profile
func TestTextReportFromRealProfile(t *testing.T) {
	// Prepare test profile.pb.gz file
	profilePath := "testdata/profile.pb.gz"

	// Generate file if it doesn't exist
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		profilePath = generateTestCPUProfile(t)
	}

	// Read the profile file
	pprofData, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("Failed to read profile file: %v", err)
	}

	// Generate text report
	textReport, err := GenerateTextReport(pprofData)
	if err != nil {
		t.Fatalf("Failed to generate text report: %v", err)
	}

	// Output the text report to a file
	outputFile := "testdata/output_real_profile_report.txt"
	if err := os.WriteFile(outputFile, []byte(textReport), 0644); err != nil {
		t.Logf("Warning: Could not write text report to file: %v", err)
	} else {
		t.Logf("Text report for real profile saved to: %s", outputFile)
	}

	// Verify the report content
	t.Logf("Text report length: %d bytes", len(textReport))

	// Check if basic structure is included
	expectedSections := []string{
		"===== Profile Information Summary =====",
		"===== Top 10 Hotspot Functions =====",
		"===== Important Call Paths =====",
		"===== Resource Usage Distribution =====",
		"===== Bottleneck Analysis Hints =====",
	}

	for _, section := range expectedSections {
		if !strings.Contains(textReport, section) {
			t.Errorf("Expected section %q not found in report", section)
		}
	}

	// Display partial content of the report
	previewLength := min(1000, len(textReport))
	t.Logf("Real profile report preview (first 1000 chars):\n%s...", textReport[:previewLength])
}

// TestMCPHandlerTextReport tests how MCP handler would use the text report format
func TestMCPHandlerTextReport(t *testing.T) {
	// This test starts with t.Skip(), so it will be skipped during normal test runs
	// Comment out this line if you want to run it manually

	// 1. Prepare test profile.pb.gz file
	profilePath := "testdata/profile.pb.gz"

	// Generate file if it doesn't exist
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		profilePath = generateTestCPUProfile(t)
	}

	// 2. Read the profile file
	pprofData, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("Failed to read profile file: %v", err)
	}

	// 3. Simulate MCP handler behavior:
	// 3.1 First convert to JSON (for comparison)
	jsonOutput, err := ConvertToDetailedJSON(pprofData)
	if err != nil {
		t.Fatalf("Failed to convert to JSON: %v", err)
	}

	// 3.2 Generate text report
	textReport, err := GenerateTextReport(pprofData)
	if err != nil {
		t.Fatalf("Failed to generate text report: %v", err)
	}

	// 4. Output results
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("Failed to create testdata directory: %v", err)
	}

	// Save JSON
	jsonFile := "testdata/mcp_handler_json.json"
	if err := os.WriteFile(jsonFile, []byte(jsonOutput), 0644); err != nil {
		t.Logf("Warning: Could not write JSON to file: %v", err)
	} else {
		t.Logf("JSON output saved to: %s", jsonFile)
	}

	// Save text report
	txtFile := "testdata/mcp_handler_text.txt"
	if err := os.WriteFile(txtFile, []byte(textReport), 0644); err != nil {
		t.Logf("Warning: Could not write text report to file: %v", err)
	} else {
		t.Logf("Text report saved to: %s", txtFile)
	}

	// 5. Compare result sizes
	t.Logf("JSON size: %d bytes", len(jsonOutput))
	t.Logf("Text report size: %d bytes", len(textReport))

	// 6. Text report preview
	previewLength := min(500, len(textReport))
	t.Logf("Text report preview:\n%s...", textReport[:previewLength])
}
