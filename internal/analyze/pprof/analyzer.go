package pprof

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

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

// ConvertToDetailedJSON converts pprof data to a detailed JSON representation
func ConvertToDetailedJSON(pprofData []byte) (string, error) {
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

	// Convert to detailed Profile structure
	detailedProfile := (*DetailedProfile)(prof)

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(detailedProfile, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON marshaling error: %v", err)
	}

	return string(jsonBytes), nil
}

// DetailedProfile wraps profile.Profile for detailed JSON marshaling
type DetailedProfile profile.Profile

// MarshalJSON implements custom JSON marshaling for DetailedProfile
func (p *DetailedProfile) MarshalJSON() ([]byte, error) {
	q := struct {
		SampleType        []*profile.ValueType `json:"sampleType"`
		DefaultSampleType string               `json:"defaultSampleType"`
		Sample            []*DetailedSample    `json:"sample"`
		Mapping           []*profile.Mapping   `json:"mapping"`
		Location          []*DetailedLocation  `json:"location"`
		Function          []*profile.Function  `json:"function"`
		Comments          []string             `json:"comments"`
		DropFrames        string               `json:"dropFrames"`
		KeepFrames        string               `json:"keepFrames"`
		TimeNanos         int64                `json:"timeNanos"`
		DurationNanos     int64                `json:"durationNanos"`
		PeriodType        *profile.ValueType   `json:"periodType"`
		Period            int64                `json:"period"`
	}{
		SampleType:        p.SampleType,
		DefaultSampleType: p.DefaultSampleType,
		Sample:            make([]*DetailedSample, len(p.Sample)),
		Mapping:           p.Mapping,
		Location:          make([]*DetailedLocation, len(p.Location)),
		Function:          p.Function,
		Comments:          p.Comments,
		DropFrames:        p.DropFrames,
		KeepFrames:        p.KeepFrames,
		TimeNanos:         p.TimeNanos,
		DurationNanos:     p.DurationNanos,
		PeriodType:        p.PeriodType,
		Period:            p.Period,
	}

	for i, s := range p.Sample {
		q.Sample[i] = (*DetailedSample)(s)
	}

	for i, l := range p.Location {
		q.Location[i] = (*DetailedLocation)(l)
	}

	return json.Marshal(q)
}

// DetailedSample wraps profile.Sample for detailed JSON marshaling
type DetailedSample profile.Sample

// MarshalJSON implements custom JSON marshaling for DetailedSample
func (p *DetailedSample) MarshalJSON() ([]byte, error) {
	q := struct {
		Location []uint64            `json:"location"`
		Value    []int64             `json:"value"`
		Label    map[string][]string `json:"label"`
		NumLabel map[string][]int64  `json:"numLabel"`
		NumUnit  map[string][]string `json:"numUnit"`
	}{
		Location: make([]uint64, len(p.Location)),
		Value:    p.Value,
		Label:    p.Label,
		NumLabel: p.NumLabel,
		NumUnit:  p.NumUnit,
	}

	for i, l := range p.Location {
		q.Location[i] = l.ID
	}

	return json.Marshal(q)
}

// DetailedLocation wraps profile.Location for detailed JSON marshaling
type DetailedLocation profile.Location

// MarshalJSON implements custom JSON marshaling for DetailedLocation
func (p *DetailedLocation) MarshalJSON() ([]byte, error) {
	q := struct {
		ID       uint64         `json:"id"`
		Mapping  uint64         `json:"mapping"`
		Address  uint64         `json:"address"`
		Line     []DetailedLine `json:"line"`
		IsFolded bool           `json:"isFolded"`
	}{
		ID:       p.ID,
		Mapping:  p.Mapping.ID,
		Address:  p.Address,
		Line:     make([]DetailedLine, len(p.Line)),
		IsFolded: p.IsFolded,
	}

	for i, l := range p.Line {
		q.Line[i] = DetailedLine(l)
	}

	return json.Marshal(q)
}

// DetailedLine wraps profile.Line for detailed JSON marshaling
type DetailedLine profile.Line

// MarshalJSON implements custom JSON marshaling for DetailedLine
func (p *DetailedLine) MarshalJSON() ([]byte, error) {
	q := struct {
		Function uint64 `json:"function"`
		Line     int64  `json:"line"`
		Column   int64  `json:"column"`
	}{
		Function: p.Function.ID,
		Line:     p.Line,
		Column:   p.Column,
	}

	return json.Marshal(q)
}

// GenerateTextReport creates a human-readable text report from pprof data
// highlighting performance bottlenecks
func GenerateTextReport(pprofData []byte) (string, error) {
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

	return generateTextReportFromProfile(prof)
}

// generateTextReportFromProfile creates a human-readable text report
// from an already parsed profile
func generateTextReportFromProfile(prof *profile.Profile) (string, error) {
	var report strings.Builder

	// 1. Profile Information Summary
	report.WriteString("===== Profile Information Summary =====\n")
	if prof.TimeNanos > 0 {
		fmt.Fprintf(&report, "Measurement Time: %v nanoseconds\n", prof.DurationNanos)
	}
	if prof.PeriodType != nil {
		fmt.Fprintf(&report, "Measurement Unit: %s (%s)\n", prof.PeriodType.Type, prof.PeriodType.Unit)
	}
	if len(prof.SampleType) > 0 {
		report.WriteString("Sample Types: ")
		for i, st := range prof.SampleType {
			if i > 0 {
				report.WriteString(", ")
			}
			fmt.Fprintf(&report, "%s (%s)", st.Type, st.Unit)
		}
		report.WriteString("\n")
	}
	report.WriteString("\n")

	// 2. Hotspot functions (functions consuming the most resources)
	report.WriteString("===== Top 10 Hotspot Functions =====\n")

	// Calculate cumulative values for each function
	funcCumulative := make(map[uint64]int64)
	for _, sample := range prof.Sample {
		if len(sample.Value) == 0 || len(sample.Location) == 0 {
			continue
		}

		// Use the first value (typically CPU time)
		value := sample.Value[0]

		// Accumulate sample values by function
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				funcCumulative[line.Function.ID] += value
			}
		}
	}

	// Convert function ID and value combinations to a slice
	type funcValue struct {
		funcID uint64
		value  int64
	}
	funcValues := make([]funcValue, 0, len(funcCumulative))
	for id, value := range funcCumulative {
		funcValues = append(funcValues, funcValue{id, value})
	}

	// Sort in descending order by value
	sort.Slice(funcValues, func(i, j int) bool {
		return funcValues[i].value > funcValues[j].value
	})

	// Display top 50 functions
	count := 0
	for _, fv := range funcValues {
		if count >= 50 {
			break
		}

		// Get function information
		var funcName, fileName string
		var startLine int64
		for _, fn := range prof.Function {
			if fn.ID == fv.funcID {
				funcName = fn.Name
				fileName = fn.Filename
				startLine = fn.StartLine
				break
			}
		}

		if funcName != "" {
			percentOfTotal := 0.0
			totalValue := int64(0)
			if len(prof.Sample) > 0 && len(prof.Sample[0].Value) > 0 {
				for _, sample := range prof.Sample {
					if len(sample.Value) > 0 {
						totalValue += sample.Value[0]
					}
				}
				if totalValue > 0 {
					percentOfTotal = float64(fv.value) / float64(totalValue) * 100
				}
			}

			fmt.Fprintf(&report, "%d. %s (%s:%d)\n", count+1, funcName, fileName, startLine)
			fmt.Fprintf(&report, "   Value: %d (%0.2f%%)\n", fv.value, percentOfTotal)
			fmt.Fprintf(&report, "\n")
			count++
		}
	}

	// 3. Important call paths (call stacks)
	report.WriteString("===== Important Call Paths =====\n")

	// Get call stacks sorted by sample value
	type sampleInfo struct {
		value    int64
		callPath []string
	}

	var samplePaths []sampleInfo
	for _, sample := range prof.Sample {
		if len(sample.Value) == 0 || len(sample.Location) == 0 {
			continue
		}

		// Use the first value (typically CPU time)
		value := sample.Value[0]

		// Build call path
		var callPath []string
		for i := len(sample.Location) - 1; i >= 0; i-- { // Build path in reverse order
			loc := sample.Location[i]
			if len(loc.Line) == 0 {
				continue
			}

			// Use the last line (typically the caller)
			line := loc.Line[len(loc.Line)-1]

			for _, fn := range prof.Function {
				if fn.ID == line.Function.ID {
					callPath = append(callPath, fn.Name)
					break
				}
			}
		}

		if len(callPath) > 0 {
			samplePaths = append(samplePaths, sampleInfo{value, callPath})
		}
	}

	// Sort in descending order by value
	sort.Slice(samplePaths, func(i, j int) bool {
		return samplePaths[i].value > samplePaths[j].value
	})

	// Display top 50 call paths
	for i, sp := range samplePaths {
		if i >= 50 {
			break
		}

		// Calculate ratio to total
		totalValue := int64(0)
		for _, sample := range prof.Sample {
			if len(sample.Value) > 0 {
				totalValue += sample.Value[0]
			}
		}
		percentOfTotal := 0.0
		if totalValue > 0 {
			percentOfTotal = float64(sp.value) / float64(totalValue) * 100
		}

		fmt.Fprintf(&report, "Path %d - Value: %d (%0.2f%%)\n", i+1, sp.value, percentOfTotal)

		// Display call path
		for j, funcName := range sp.callPath {
			indentation := strings.Repeat("  ", j)
			fmt.Fprintf(&report, "%s-> %s\n", indentation, funcName)
		}
		fmt.Fprintf(&report, "\n")
	}

	// 4. Resource usage distribution
	if len(prof.SampleType) > 0 {
		report.WriteString("===== Resource Usage Distribution =====\n")
		for i, sampleType := range prof.SampleType {
			if i >= len(prof.Sample[0].Value) {
				continue
			}

			fmt.Fprintf(&report, "Measurement: %s (%s)\n", sampleType.Type, sampleType.Unit)

			// Calculate total values
			totalValue := int64(0)
			for _, sample := range prof.Sample {
				if i < len(sample.Value) {
					totalValue += sample.Value[i]
				}
			}

			fmt.Fprintf(&report, "Total: %d %s\n\n", totalValue, sampleType.Unit)
		}
	}

	// 5. Profiling hints
	report.WriteString("===== Bottleneck Analysis Hints =====\n")
	report.WriteString("1. Focus on top functions (especially those consuming more than 10% of total resources)\n")
	report.WriteString("2. Deep call paths may indicate excessive recursion or library calls\n")
	report.WriteString("3. Consider optimizing functions that appear in multiple call paths\n")
	report.WriteString("4. Consider algorithm improvements, caching, and parallel processing for optimization\n")

	return report.String(), nil
}
