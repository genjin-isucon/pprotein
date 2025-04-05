package slowlog

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestAnalyze(t *testing.T) {
	// Prepare a sample of slowlog for testing
	sampleLog := `# Time: 2023-04-01T12:00:00.000000Z
# User@Host: testuser[testuser] @ localhost []
# Query_time: 2.000000  Lock_time: 0.000010 Rows_sent: 1  Rows_examined: 10000
SET timestamp=1680350400;
SELECT * FROM users WHERE status = 'active';

# Time: 2023-04-01T12:01:00.000000Z
# User@Host: testuser[testuser] @ localhost []
# Query_time: 1.500000  Lock_time: 0.000020 Rows_sent: 5  Rows_examined: 5000
SET timestamp=1680350460;
SELECT * FROM users WHERE status = 'active';

# Time: 2023-04-01T12:02:00.000000Z
# User@Host: admin[admin] @ localhost []
# Query_time: 3.200000  Lock_time: 0.000030 Rows_sent: 100  Rows_examined: 50000
SET timestamp=1680350520;
SELECT * FROM orders WHERE created_at > '2023-01-01';
`

	// Add sample log to a temporary file
	tmpFilePath := "test_slowlog.log"
	err := os.WriteFile(tmpFilePath, []byte(sampleLog), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFilePath) // Delete file after test

	// Read log from file
	logContent, err := os.ReadFile(tmpFilePath)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Analyze slowlog (threshold 0.5 seconds)
	result, err := Analyze(logContent, 0.5)
	if err != nil {
		t.Fatalf("Failed to analyze slowlog: %v", err)
	}

	// Decode result from JSON to struct
	var analysisResult AnalysisResult
	err = json.Unmarshal([]byte(result), &analysisResult)
	if err != nil {
		t.Fatalf("Failed to decode JSON result: %v", err)
	}

	// Verify result
	t.Logf("Analysis result: %s", result)

	// Verify the total query count
	if analysisResult.TotalQueries != 3 {
		t.Errorf("Total query count is different from expected. Expected: 3, Actual: %d", analysisResult.TotalQueries)
	}

	// Verify the top query pattern
	if len(analysisResult.TopQueryPatterns) == 0 {
		t.Errorf("No query patterns found")
	} else {
		// Verify the slowest query pattern
		slowestPattern := analysisResult.TopQueryPatterns[0]
		if slowestPattern.MaxTime < 3.0 {
			t.Errorf("The time of the slowest query is less than expected. Expected: 3.0 or more, Actual: %f", slowestPattern.MaxTime)
		}
	}

	// Verify the slowest query
	if len(analysisResult.SlowestQueries) == 0 {
		t.Errorf("No slow queries found")
	} else {
		slowestQuery := analysisResult.SlowestQueries[0]
		if slowestQuery.QueryTime < 3.0 {
			t.Errorf("The time of the slowest query is less than expected. Expected: 3.0 or more, Actual: %f", slowestQuery.QueryTime)
		}
	}

	// Additional test - Save result to file
	outputFilePath := "slowlog_analysis_result.json"
	err = os.WriteFile(outputFilePath, []byte(result), 0644)
	if err != nil {
		t.Fatalf("Failed to write result file: %v", err)
	}
	defer os.Remove(outputFilePath) // Delete file after test

	t.Logf("Analysis result saved to file: %s", outputFilePath)
}

// Test with larger dataset
func TestAnalyzeWithLargeDataset(t *testing.T) {
	// If there is a test dataset with a large amount of data in a real project,
	// specify the file path here to read
	const largeLogFilePath = "testdata/large_slowlog.log"

	// Check if the test data directory and file exist
	if _, err := os.Stat("testdata"); os.IsNotExist(err) {
		os.Mkdir("testdata", 0755)
	}

	// If the large test log file does not exist, generate it
	if _, err := os.Stat(largeLogFilePath); os.IsNotExist(err) {
		// Generate a log file with hundreds of queries
		var largeLog string
		startTime := time.Date(2023, 4, 1, 0, 0, 0, 0, time.UTC)

		for i := 0; i < 200; i++ {
			currentTime := startTime.Add(time.Duration(i) * time.Minute)
			timeStr := currentTime.Format("2006-01-02T15:04:05.000000Z")

			queryTime := 0.5 + float64(i%10)/2.0 // Variation of 0.5 seconds to 5.0 seconds
			rowsExamined := 1000 * (i%20 + 1)    // 1000〜20000 rows
			rowsSent := 10 * (i%10 + 1)          // 10〜100 rows

			// Variation of queries
			var query string
			switch i % 5 {
			case 0:
				query = "SELECT * FROM users WHERE id > 1000 LIMIT 100"
			case 1:
				query = "SELECT * FROM orders WHERE created_at > '2023-01-01'"
			case 2:
				query = "SELECT products.*, categories.name FROM products JOIN categories ON products.category_id = categories.id"
			case 3:
				query = "SELECT COUNT(*) FROM access_logs WHERE accessed_at > NOW() - INTERVAL 1 DAY"
			case 4:
				query = "SELECT user_id, SUM(amount) FROM payments GROUP BY user_id ORDER BY SUM(amount) DESC"
			}

			logEntry := `# Time: ` + timeStr + `
# User@Host: user` + string(rune('A'+(i%3))) + `[user` + string(rune('A'+(i%3))) + `] @ localhost []
# Query_time: ` + fmt.Sprintf("%.6f", queryTime) + `  Lock_time: 0.000` + fmt.Sprintf("%02d", i%100) + ` Rows_sent: ` + fmt.Sprintf("%d", rowsSent) + `  Rows_examined: ` + fmt.Sprintf("%d", rowsExamined) + `
SET timestamp=` + fmt.Sprintf("%d", currentTime.Unix()) + `;
` + query + `;

`
			largeLog += logEntry
		}

		err = os.WriteFile(largeLogFilePath, []byte(largeLog), 0644)
		if err != nil {
			t.Skipf("Failed to create large test file: %v", err)
			return
		}
	}

	// Check if the file exists
	if _, err := os.Stat(largeLogFilePath); os.IsNotExist(err) {
		t.Skipf("Test file not found: %s", largeLogFilePath)
		return
	}

	// Read log from file
	logContent, err := os.ReadFile(largeLogFilePath)
	if err != nil {
		t.Fatalf("Failed to read test file: %v", err)
	}

	// Analyze slowlog (threshold 1.0 seconds)
	result, err := Analyze(logContent, 1.0)
	if err != nil {
		t.Fatalf("Failed to analyze slowlog: %v", err)
	}

	// Decode result from JSON to struct
	var analysisResult AnalysisResult
	err = json.Unmarshal([]byte(result), &analysisResult)
	if err != nil {
		t.Fatalf("Failed to decode JSON result: %v", err)
	}

	// Verify result
	t.Logf("Analysis result for large dataset: Top pattern count=%d, Slowest query count=%d, Total query count=%d",
		len(analysisResult.TopQueryPatterns),
		len(analysisResult.SlowestQueries),
		analysisResult.TotalQueries)

	// Performance measurement (optional)
	if testing.Verbose() {
		t.Logf("Total analysis time: %.2f seconds", analysisResult.TotalTime)
	}

	// Additional test - Save result to file
	outputFilePath := "slowlog_large_analysis_result.json"
	err = os.WriteFile(outputFilePath, []byte(result), 0644)
	if err != nil {
		t.Fatalf("Failed to write result file: %v", err)
	}

}
