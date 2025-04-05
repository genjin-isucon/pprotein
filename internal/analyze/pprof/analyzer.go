package pprof

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/pprof/profile"
)

// Analyze parses pprof binary data and returns it in Speedscope JSON format
func Analyze(pprofData []byte, profileType string) (string, error) {
	// Convert according to the parsing format
	return convertPprofToStructuredJSON(pprofData, profileType)
}

// Function to convert pprof data into structured JSON for LLM analysis
func convertPprofToStructuredJSON(pprofData []byte, profileType string) (string, error) {
	// Create a temporary file and write pprof data
	tempFile, err := os.CreateTemp("", "pprof-*.pb.gz")
	if err != nil {
		return "", fmt.Errorf("temporary file creation error: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	if _, err := tempFile.Write(pprofData); err != nil {
		return "", fmt.Errorf("temporary file write error: %v", err)
	}
	tempFile.Close() // Close after writing

	// Open profile file
	f, err := os.Open(tempFile.Name())
	if err != nil {
		return "", fmt.Errorf("profile file open error: %v", err)
	}
	defer f.Close()

	// Parse the profile
	prof, err := profile.Parse(f)
	if err != nil {
		return "", fmt.Errorf("pprof parsing error: %v", err)
	}

	// Generate structured JSON
	structuredJSON, err := generateStructuredJSON(prof, profileType)
	if err != nil {
		return "", fmt.Errorf("JSON generation error: %v", err)
	}

	return structuredJSON, nil
}

// Generate structured JSON from profile data for LLM analysis
func generateStructuredJSON(prof *profile.Profile, profileType string) (string, error) {
	// Prepare result data structure
	result := map[string]interface{}{
		"metadata": map[string]interface{}{
			"profileType": profileType,
			"timeNanos":   prof.TimeNanos,
			"duration":    prof.DurationNanos,
			"period":      prof.Period,
			"periodType":  prof.PeriodType.Type,
			"periodUnit":  prof.PeriodType.Unit,
		},
	}

	// Create function mapping
	functionMap := make(map[uint64]map[string]interface{})
	for _, fn := range prof.Function {
		functionMap[fn.ID] = map[string]interface{}{
			"name":      fn.Name,
			"filename":  fn.Filename,
			"startLine": fn.StartLine,
		}
	}

	// Structure location information
	locationMap := make(map[uint64]map[string]interface{})
	var stackTraces []interface{}

	for _, loc := range prof.Location {
		var callStack []interface{}

		for _, line := range loc.Line {
			if funcData, exists := functionMap[line.Function.ID]; exists {
				callInfo := map[string]interface{}{
					"function": funcData["name"],
					"filename": funcData["filename"],
					"line":     line.Line,
				}
				callStack = append(callStack, callInfo)
			}
		}

		if len(callStack) > 0 {
			structuredLoc := map[string]interface{}{
				"id":        loc.ID,
				"address":   loc.Address,
				"callStack": callStack,
			}
			stackTraces = append(stackTraces, structuredLoc)
			locationMap[loc.ID] = structuredLoc
		}
	}

	result["stackTraces"] = stackTraces

	// Structure sample information
	var samples []interface{}
	for _, sample := range prof.Sample {
		// Collect location IDs corresponding to the sample
		var locationIDs []uint64
		for _, loc := range sample.Location {
			locationIDs = append(locationIDs, loc.ID)
		}

		// Collect value information
		var values []int64
		for _, v := range sample.Value {
			values = append(values, v)
		}

		simplifiedSample := map[string]interface{}{
			"locationIDs": locationIDs,
			"values":      values,
			"labels":      sample.Label,
		}
		samples = append(samples, simplifiedSample)
	}

	result["samples"] = samples

	// Convert structured JSON to string
	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}
