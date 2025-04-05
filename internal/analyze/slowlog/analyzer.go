package slowlog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/percona/go-mysql/log"
	parser "github.com/percona/go-mysql/log/slow"
	"github.com/percona/go-mysql/query"
)

// QueryStats is a structure that stores query statistics
type QueryStats struct {
	Pattern         string    `json:"pattern"`           // SQL query pattern
	Count           int       `json:"count"`             // Execution count
	TotalTime       float64   `json:"total_time"`        // Total execution time
	AvgTime         float64   `json:"avg_time"`          // Average execution time
	MaxTime         float64   `json:"max_time"`          // Maximum execution time
	MinTime         float64   `json:"min_time"`          // Minimum execution time
	RowsExamined    int64     `json:"rows_examined"`     // Total number of rows examined
	RowsExaminedAvg float64   `json:"rows_examined_avg"` // Average number of rows examined
	RowsSent        int64     `json:"rows_sent"`         // Total number of rows sent
	RowsSentAvg     float64   `json:"rows_sent_avg"`     // Average number of rows sent
	Example         string    `json:"example"`           // Example of query
	FirstSeen       time.Time `json:"first_seen"`        // Time first seen
	LastSeen        time.Time `json:"last_seen"`         // Time last seen
}

// SlowQuery is a structure that stores information about individual slow queries
type SlowQuery struct {
	Time         time.Time `json:"time"`          // Query execution time
	User         string    `json:"user"`          // User
	Host         string    `json:"host"`          // Host
	Db           string    `json:"db,omitempty"`  // Database
	QueryTime    float64   `json:"query_time"`    // Query execution time (seconds)
	LockTime     float64   `json:"lock_time"`     // Lock time (seconds)
	RowsSent     int       `json:"rows_sent"`     // Number of rows sent
	RowsExamined int       `json:"rows_examined"` // Number of rows examined
	Query        string    `json:"query"`         // SQL query
}

// Structure to store analysis results
type AnalysisResult struct {
	TopQueryPatterns []QueryStats `json:"top_query_patterns"` // Top query patterns
	SlowestQueries   []SlowQuery  `json:"slowest_queries"`    // Slowest queries
	TotalQueries     int          `json:"total_queries"`      // Total number of queries
	TotalTime        float64      `json:"total_time"`         // Total execution time
}

// Analyze parses MySQL slow logs using the Percona go-mysql library and returns the results in JSON format
func Analyze(logContent []byte, threshold float64) (string, error) {
	// Convert logContent to io.Reader (using a temporary file)
	tmpFile, err := os.CreateTemp("", "slowlog")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err = tmpFile.Write(logContent); err != nil {
		return "", fmt.Errorf("failed to write to temporary file: %v", err)
	}

	if _, err = tmpFile.Seek(0, 0); err != nil {
		return "", fmt.Errorf("failed to seek in temporary file: %v", err)
	}

	// Initialize Percona parser
	parser := parser.NewSlowLogParser(tmpFile, log.Options{
		DefaultLocation: time.UTC,
	})

	// Map to store statistics by pattern
	patternStats := make(map[string]*QueryStats)

	// List of slowest queries
	var slowQueries []SlowQuery

	// Total statistics
	totalQueries := 0
	totalTime := 0.0

	// Start the parser
	go parser.Start()

	// Timeout channel
	timeout := time.After(30 * time.Second)

	// Process events from the event channel
	eventChan := parser.EventChan()
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// If the channel is closed
				goto LOOP_END
			}
			if event == nil {
				continue
			}

			// Check if the query time exceeds the threshold
			queryTime := event.TimeMetrics["Query_time"]
			if queryTime >= threshold {
				// Add to slow queries
				slowQuery := SlowQuery{
					Time:         event.Ts,
					User:         event.User,
					Host:         event.Host,
					Db:           event.Db,
					QueryTime:    queryTime,
					LockTime:     event.TimeMetrics["Lock_time"],
					RowsSent:     int(event.NumberMetrics["Rows_sent"]),
					RowsExamined: int(event.NumberMetrics["Rows_examined"]),
					Query:        event.Query,
				}
				slowQueries = append(slowQueries, slowQuery)
			}

			// Normalize the query to group the same patterns
			fingerprintQuery := query.Fingerprint(event.Query)

			// Update statistics
			stats, exists := patternStats[fingerprintQuery]
			if !exists {
				stats = &QueryStats{
					Pattern:   fingerprintQuery,
					Count:     0,
					TotalTime: 0,
					MaxTime:   0,
					MinTime:   float64(^uint64(0) >> 1), // Initialize with maximum value
					Example:   event.Query,
					FirstSeen: event.Ts,
					LastSeen:  event.Ts,
				}
				patternStats[fingerprintQuery] = stats
			}

			// Update statistics
			stats.Count++
			stats.TotalTime += queryTime
			stats.LastSeen = event.Ts

			if queryTime > stats.MaxTime {
				stats.MaxTime = queryTime
			}
			if queryTime < stats.MinTime {
				stats.MinTime = queryTime
			}

			// Update row count statistics
			rowsExamined := int64(event.NumberMetrics["Rows_examined"])
			rowsSent := int64(event.NumberMetrics["Rows_sent"])
			stats.RowsExamined += rowsExamined
			stats.RowsSent += rowsSent

			totalQueries++
			totalTime += queryTime

		case <-timeout:
			// Timeout processing
			fmt.Printf("Slow log analysis has timed out")
			goto LOOP_END
		}
	}

LOOP_END:
	// Convert statistics to a slice and calculate averages
	var statsSlice []QueryStats
	for _, stat := range patternStats {
		if stat.Count > 0 {
			stat.AvgTime = stat.TotalTime / float64(stat.Count)
			stat.RowsExaminedAvg = float64(stat.RowsExamined) / float64(stat.Count)
			stat.RowsSentAvg = float64(stat.RowsSent) / float64(stat.Count)
			statsSlice = append(statsSlice, *stat)
		}
	}

	// Sort by total execution time (descending)
	sort.Slice(statsSlice, func(i, j int) bool {
		return statsSlice[i].TotalTime > statsSlice[j].TotalTime
	})

	// Sort the slowest queries by execution time (descending)
	sort.Slice(slowQueries, func(i, j int) bool {
		return slowQueries[i].QueryTime > slowQueries[j].QueryTime
	})

	// Return only the top 20 patterns and 10 slowest queries
	topPatterns := statsSlice
	if len(topPatterns) > 20 {
		topPatterns = topPatterns[:20]
	}

	topSlowQueries := slowQueries
	if len(topSlowQueries) > 10 {
		topSlowQueries = topSlowQueries[:10]
	}

	// Return results in JSON
	result := AnalysisResult{
		TopQueryPatterns: topPatterns,
		SlowestQueries:   topSlowQueries,
		TotalQueries:     totalQueries,
		TotalTime:        totalTime,
	}

	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to convert to JSON: %v", err)
	}

	return string(jsonResult), nil
}
