package httplog

import (
	"encoding/json"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// SlowRequest is a structure that stores information about slow requests
type SlowRequest struct {
	Time    string  // Request time
	URI     string  // Request URI
	Method  string  // HTTP method
	ReqTime float64 // Processing time
	Host    string  // Hostname
}

// EndpointStats is a structure that stores statistics per endpoint
type EndpointStats struct {
	Count       int         // Number of requests
	TotalTime   float64     // Total processing time
	AvgTime     float64     // Average processing time
	MaxTime     float64     // Maximum processing time
	StatusCodes map[int]int // Status code counts
}

// AlpConfig represents the ALP configuration file structure
type AlpConfig struct {
	MatchingGroups []string `yaml:"matching_groups"`
}

// Analyze parses raw HTTP logs and returns results in JSON format
func Analyze(logContent []byte, slowThreshold float64) (string, error) {
	lines := strings.Split(string(logContent), "\n")

	// Get ALP config
	config, err := loadAlpConfig()
	if err != nil {
		log.Printf("Failed to load ALP config, using default URI patterns: %v", err)
	}

	// 1. Aggregate by endpoint
	endpointStats := analyzeLog(lines, config)

	// 2. Extract slow requests (above threshold)
	slowRequests := extractSlowRequests(lines, slowThreshold)

	// Return results in JSON format
	result := map[string]interface{}{
		"endpoint_stats": endpointStats,
		"slow_requests":  slowRequests[:min(10, len(slowRequests))], // 10 slowest requests
		"config_used":    config != nil && len(config.MatchingGroups) > 0,
	}

	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonResult), nil
}

// loadAlpConfig loads the ALP configuration file
func loadAlpConfig() (*AlpConfig, error) {
	// Try to find the ALP config file in different locations
	configPaths := []string{
		"data/alp.yml",
		"internal/extproc/alp/alp.yml",
		"/home/purplehaze/Projects/pprotein/data/alp.yml",
		"/home/purplehaze/Projects/pprotein/internal/extproc/alp/alp.yml",
	}

	var configBytes []byte
	var err error

	for _, path := range configPaths {
		configBytes, err = os.ReadFile(path)
		if err == nil {
			log.Printf("Loaded ALP config from %s", path)
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var config AlpConfig
	if err := yaml.Unmarshal(configBytes, &config); err != nil {
		return nil, err
	}

	log.Printf("Loaded %d matching groups from ALP config", len(config.MatchingGroups))
	return &config, nil
}

// extractSlowRequests extracts requests from log lines where processing time exceeds the threshold
func extractSlowRequests(logLines []string, thresholdSeconds float64) []SlowRequest {
	var slowRequests []SlowRequest

	for _, line := range logLines {
		fields := strings.Split(line, "\t")
		reqtime, _ := strconv.ParseFloat(extractField(fields, "reqtime:"), 64)

		if reqtime >= thresholdSeconds {
			slowRequests = append(slowRequests, SlowRequest{
				Time:    extractField(fields, "time:"),
				URI:     extractField(fields, "uri:"),
				Method:  extractField(fields, "method:"),
				ReqTime: reqtime,
				Host:    extractField(fields, "vhost:"),
			})
		}
	}

	// Sort by processing time in descending order
	sort.Slice(slowRequests, func(i, j int) bool {
		return slowRequests[i].ReqTime > slowRequests[j].ReqTime
	})

	return slowRequests
}

// analyzeLog extracts statistics per endpoint from log lines
func analyzeLog(logLines []string, config *AlpConfig) map[string]*EndpointStats {
	stats := make(map[string]*EndpointStats)

	for _, line := range logLines {
		fields := strings.Split(line, "\t")
		// Extract necessary fields
		uri := extractField(fields, "uri:")
		reqtime, _ := strconv.ParseFloat(extractField(fields, "reqtime:"), 64)
		status, _ := strconv.Atoi(extractField(fields, "status:"))

		// Patternize URI (replace ID with :id or use ALP config)
		patternURI := patternizeURI(uri, config)

		if _, exists := stats[patternURI]; !exists {
			stats[patternURI] = &EndpointStats{
				StatusCodes: make(map[int]int),
			}
		}

		s := stats[patternURI]
		s.Count++
		s.TotalTime += reqtime
		if reqtime > s.MaxTime {
			s.MaxTime = reqtime
		}
		s.StatusCodes[status]++
	}

	// Calculate average time
	for _, s := range stats {
		s.AvgTime = s.TotalTime / float64(s.Count)
	}

	return stats
}

// extractField extracts the value of a field that starts with fieldPrefix from log lines
func extractField(fields []string, fieldPrefix string) string {
	for _, field := range fields {
		if strings.HasPrefix(field, fieldPrefix) {
			return field[len(fieldPrefix):]
		}
	}
	return ""
}

// patternizeURI replaces ID with :id in URI for patternization or uses ALP config patterns
func patternizeURI(uri string, config *AlpConfig) string {
	// If ALP config is available, use matching groups
	if config != nil && len(config.MatchingGroups) > 0 {
		for i, pattern := range config.MatchingGroups {
			r, err := regexp.Compile(pattern)
			if err != nil {
				log.Printf("Invalid regex pattern in ALP config: %s", pattern)
				continue
			}

			if r.MatchString(uri) {
				// Use the pattern as the group name
				return "group_" + strconv.Itoa(i+1) + ": " + pattern
			}
		}
	}

	// Fall back to default patternization if no matching group found
	re := regexp.MustCompile(`/\d+(/|$)`)
	return re.ReplaceAllString(uri, "/:id$1")
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
