package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/kaz/pprotein/internal/analyze/pprof"
	"github.com/kaz/pprotein/internal/analyze/slowlog"
	"github.com/kaz/pprotein/internal/collect"
)

// Get group list handler
func handleGroupList(port string) (interface{}, error) {
	log.Println("Executing group_list function")

	// Map to store results
	result := map[string]interface{}{
		"groups": []string{},
	}

	// Collect entries from all endpoints
	endpoints := []string{"pprof", "httplog", "slowlog", "memo"}
	uniqueGroups := make(map[string]struct{})

	for _, endpoint := range endpoints {
		log.Printf("Fetching entries from endpoint: %s", endpoint)

		// Get data from each endpoint
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", port, endpoint), nil)
		if err != nil {
			log.Printf("Error creating request for %s: %v", endpoint, err)
			continue
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error fetching from %s: %v", endpoint, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Unexpected status code from %s: %d", endpoint, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			log.Printf("Error decoding response from %s: %v", endpoint, err)
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Collect unique group IDs
		for _, entry := range entries {
			if entry.Snapshot.GroupId != "" {
				uniqueGroups[entry.Snapshot.GroupId] = struct{}{}
			}
		}

		log.Printf("Found %d entries from %s", len(entries), endpoint)
	}

	// Convert unique group IDs to a slice and sort in descending order
	groupIDs := make([]string, 0, len(uniqueGroups))
	for gid := range uniqueGroups {
		groupIDs = append(groupIDs, gid)
	}

	sort.Slice(groupIDs, func(i, j int) bool {
		return groupIDs[i] > groupIDs[j] // Descending order
	})

	result["groups"] = groupIDs
	log.Printf("group_list completed, found %d groups", len(groupIDs))
	return result, nil
}

// Get group data handler
func handleGroupData(port string, groupID string) (interface{}, error) {
	log.Printf("Executing group_data function with group_id: %s", groupID)

	result := map[string]interface{}{
		"group_id": groupID,
		"data":     map[string][]interface{}{},
	}

	// Get data from each collector
	endpoints := []string{"pprof", "httplog", "slowlog", "memo"}

	for _, endpoint := range endpoints {
		log.Printf("Fetching group data from endpoint: %s", endpoint)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", port, endpoint), nil)
		if err != nil {
			log.Printf("Error creating request for %s: %v", endpoint, err)
			continue
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error fetching from %s: %v", endpoint, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Unexpected status code from %s: %d", endpoint, resp.StatusCode)
			resp.Body.Close()
			continue
		}

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			log.Printf("Error decoding response from %s: %v", endpoint, err)
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// GroupIDでフィルタリング
		var filtered []interface{}
		for _, entry := range entries {
			if entry.Snapshot.GroupId == groupID {
				filtered = append(filtered, entry)
			}
		}

		if len(filtered) > 0 {
			result["data"].(map[string][]interface{})[endpoint] = filtered
			log.Printf("Found %d filtered entries from %s", len(filtered), endpoint)
		}
	}

	log.Printf("group_data completed for group_id: %s", groupID)
	return result, nil
}

// Get group file handler
func handleGroupFile(port string, groupID string, fileType string, entryID string) ([]byte, string, error) {
	log.Printf("Executing group_file function with group_id: %s, type: %s, entry_id: %s", groupID, fileType, entryID)

	// If httplog, return analysis result
	if fileType == "httplog" {
		result, contentType, err := handleHttpLogAnalysis(port, groupID, fileType, entryID)
		if err != nil {
			return nil, "", err
		}
		return []byte(result), contentType, nil
	}

	// If slowlog, return analysis result
	if fileType == "slowlog" {
		result, contentType, err := handleSlowLogAnalysis(port, groupID, fileType, entryID)
		if err != nil {
			return nil, "", err
		}
		return []byte(result), contentType, nil
	}

	// If pprof, return analysis result
	if fileType == "pprof" {
		// If there is a special format specification
		format := strings.ToLower(strings.TrimSpace(entryID))
		if format == "speedscope" {
			// Return Speedscope JSON format
			result, contentType, err := handlePprofAnalysis(port, groupID, fileType, "")
			if err != nil {
				return nil, "", err
			}
			return []byte(result), contentType, nil
		}

		if format == "detailed_json" {
			// Return detailed JSON format
			result, contentType, err := handlePprofDetailedJSON(port, groupID)
			if err != nil {
				return nil, "", err
			}
			return []byte(result), contentType, nil
		}

		if entryID != "" && !strings.HasPrefix(entryID, "format=") {
			// Get text report for specific entry ID (default format)
			result, contentType, err := handlePprofTextReportWithEntryID(port, groupID, entryID)
			if err != nil {
				return nil, "", err
			}
			return []byte(result), contentType, nil
		}

		result, contentType, err := handlePprofTextReport(port, groupID)
		if err != nil {
			return nil, "", err
		}
		return []byte(result), contentType, nil
	}

	// Get ID from metadata first
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", port, fileType), nil)
	if err != nil {
		return nil, "", fmt.Errorf("error creating request for %s: %v", fileType, err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error fetching from %s: %v", fileType, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code from %s: %d", fileType, resp.StatusCode)
	}

	var entries []*collect.Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, "", fmt.Errorf("error decoding response from %s: %v", fileType, err)
	}

	// Filter by group ID and entry ID
	var selectedID string
	for _, entry := range entries {
		if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
			if entryID == "" || entry.Snapshot.ID == entryID {
				selectedID = entry.Snapshot.ID
				break
			}
		}
	}

	if selectedID == "" {
		return nil, "", fmt.Errorf("no matching entry found for group_id: %s, type: %s", groupID, fileType)
	}

	// Get data directly - use data API endpoint
	dataURL := fmt.Sprintf("http://localhost:%s/api/%s/data/%s", port, fileType, selectedID)
	log.Printf("Fetching file data from: %s", dataURL)

	dataResp, err := http.Get(dataURL)
	if err != nil {
		return nil, "", fmt.Errorf("error fetching file data: %v", err)
	}
	defer dataResp.Body.Close()

	if dataResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
	}

	fileContent, err := io.ReadAll(dataResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading file content: %v", err)
	}

	contentType := determineContentType(fileType, selectedID)

	log.Printf("Successfully fetched file for group_id: %s, type: %s, id: %s, size: %d bytes",
		groupID, fileType, selectedID, len(fileContent))
	return fileContent, contentType, nil
}

// Determine Content-Type based on file type
func determineContentType(fileType string, filePath string) string {
	switch fileType {
	case "pprof":
		return "application/octet-stream"
	case "httplog", "slowlog", "memo":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func handleHttpLogAnalysis(apiPort, groupID, fileType, entryID string) (string, string, error) {
	// まず適切なエントリIDを取得
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", apiPort, fileType), nil)
	if err != nil {
		return "", "", fmt.Errorf("error creating request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("error fetching from %s: %v", fileType, err)
	}
	defer resp.Body.Close()

	var entries []*collect.Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", "", fmt.Errorf("error decoding response: %v", err)
	}

	// 適切なエントリを選択
	var selectedID string
	for _, entry := range entries {
		if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
			if entryID == "" || entry.Snapshot.ID == entryID {
				selectedID = entry.Snapshot.ID
				break
			}
		}
	}

	if selectedID == "" {
		return "", "", fmt.Errorf("no matching entry found")
	}

	// 解析済みデータを直接取得
	analysisURL := fmt.Sprintf("http://localhost:%s/api/%s/%s", apiPort, fileType, selectedID)
	log.Printf("Fetching analysis data from: %s", analysisURL)

	analysisResp, err := http.Get(analysisURL)
	if err != nil {
		return "", "", fmt.Errorf("error fetching analysis: %v", err)
	}
	defer analysisResp.Body.Close()

	if analysisResp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status code: %d", analysisResp.StatusCode)
	}

	// 解析済みデータを読み込み
	analysisData, err := io.ReadAll(analysisResp.Body)
	if err != nil {
		return "", "", fmt.Errorf("error reading analysis: %v", err)
	}

	// ALPの出力をJSONに変換するなどの処理が必要であれば実装
	// ここでは簡単にALPの結果をJSONにラップする例
	result := map[string]interface{}{
		"alp_analysis": string(analysisData),
		"source":       "pre-analyzed",
	}

	jsonResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", "", err
	}

	return string(jsonResult), "application/json", nil
}

func handleSlowLogAnalysis(port, groupID, fileType, entryID string) (string, string, error) {
	// Helper function to get raw file content
	getRawFileContent := func() ([]byte, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", port, fileType), nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		// Decode with collect.Entry type
		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, fmt.Errorf("JSON decode error: %v", err)
		}

		// Filter by group ID and entry ID
		var selectedID string
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
				if entryID == "" || entry.Snapshot.ID == entryID {
					selectedID = entry.Snapshot.ID
					break
				}
			}
		}

		if selectedID == "" {
			return nil, fmt.Errorf("no matching entry found: group_id=%s, type=%s", groupID, fileType)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/%s/data/%s", port, fileType, selectedID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading file content: %v", err)
		}

		return fileContent, nil
	}

	// Get raw file content
	fileContent, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Analyze with slowlog package (threshold 0.5 seconds)
	result, err := slowlog.Analyze(fileContent, 0.5)
	if err != nil {
		return "", "", err
	}

	return result, "application/json", nil
}

// pprof file analysis handler
func handlePprofAnalysis(port, groupID, fileType, entryID string) (string, string, error) {
	// Helper function to get raw file content
	getRawFileContent := func() ([]byte, string, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/%s", port, fileType), nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, "", fmt.Errorf("JSON decode error: %v", err)
		}

		// Filter by group ID and entry ID
		var selectedID string
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
				if entryID == "" || entry.Snapshot.ID == entryID {
					selectedID = entry.Snapshot.ID
					break
				}
			}
		}

		if selectedID == "" {
			return nil, "", fmt.Errorf("no matching entry found: group_id=%s, type=%s", groupID, fileType)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/%s/data/%s", port, fileType, selectedID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, "", fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file content: %v", err)
		}

		// Profile type is inferred from file path
		var profileType string
		if strings.Contains(selectedID, "cpu") {
			profileType = "cpu"
		} else if strings.Contains(selectedID, "heap") {
			profileType = "heap"
		} else {
			// Default profile type
			profileType = "unknown"
		}

		return fileContent, profileType, nil
	}

	// Get raw file content
	fileContent, profileType, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Analyze with analyze/pprof package
	result, err := pprof.Analyze(fileContent, profileType)
	if err != nil {
		return "", "", fmt.Errorf("pprof analysis error: %v", err)
	}

	return result, "application/json", nil
}

// pprof file detailed JSON handler
func handlePprofDetailedJSON(port, groupID string) (string, string, error) {
	// Helper function to get raw file content
	getRawFileContent := func() ([]byte, string, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/pprof", port), nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, "", fmt.Errorf("JSON decode error: %v", err)
		}

		// Filter by group ID and get the latest entry
		var latestEntry *collect.Entry
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
				if latestEntry == nil || entry.Snapshot.Datetime.After(latestEntry.Snapshot.Datetime) {
					latestEntry = entry
				}
			}
		}

		if latestEntry == nil {
			return nil, "", fmt.Errorf("no matching entry found: group_id=%s", groupID)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/pprof/data/%s", port, latestEntry.Snapshot.ID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, "", fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file content: %v", err)
		}

		// Profile type is inferred from file path
		var profileType string
		if strings.Contains(latestEntry.Snapshot.ID, "cpu") {
			profileType = "cpu"
		} else if strings.Contains(latestEntry.Snapshot.ID, "heap") {
			profileType = "heap"
		} else {
			// Default profile type
			profileType = "unknown"
		}

		return fileContent, profileType, nil
	}

	// Get raw file content
	fileContent, _, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Convert to detailed JSON format
	detailedJSON, err := pprof.ConvertToDetailedJSON(fileContent)
	if err != nil {
		return "", "", fmt.Errorf("pprof JSON conversion error: %v", err)
	}

	return detailedJSON, "application/json", nil
}

// pprof file detailed JSON handler with specific entry ID
func handlePprofDetailedJSONWithEntryID(port, groupID, entryID string) (string, string, error) {
	// Helper function to get raw file content for specific entry ID
	getRawFileContent := func() ([]byte, string, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/pprof", port), nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, "", fmt.Errorf("JSON decode error: %v", err)
		}

		// 指定されたグループIDとエントリIDの組み合わせを確認
		var foundEntry *collect.Entry
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID && entry.Snapshot.ID == entryID {
				foundEntry = entry
				break
			}
		}

		if foundEntry == nil {
			return nil, "", fmt.Errorf("no matching entry found: group_id=%s, entry_id=%s", groupID, entryID)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/pprof/data/%s", port, entryID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, "", fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file content: %v", err)
		}

		// Profile type is inferred from file path
		var profileType string
		if strings.Contains(entryID, "cpu") {
			profileType = "cpu"
		} else if strings.Contains(entryID, "heap") {
			profileType = "heap"
		} else {
			// Default profile type
			profileType = "unknown"
		}

		return fileContent, profileType, nil
	}

	// Get raw file content for specific entry
	fileContent, _, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Convert to detailed JSON format
	detailedJSON, err := pprof.ConvertToDetailedJSON(fileContent)
	if err != nil {
		return "", "", fmt.Errorf("pprof JSON conversion error: %v", err)
	}

	return detailedJSON, "application/json", nil
}

// alp config file retrieval handler
func handleGetAlpConfig(port string) (string, error) {
	log.Println("Executing alp_config_get function")

	// Get API endpoint for config file
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/httplog/config", port), nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error fetching config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	configContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading config: %v", err)
	}

	return string(configContent), nil
}

// alp config file update handler
func handleUpdateAlpConfig(port string, config string) error {
	log.Println("Executing alp_config_update function")

	// API endpoint to update config file - use POST method
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://localhost:%s/api/httplog/config", port),
		bytes.NewBufferString(config))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error updating config: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// pprof text report handler
func handlePprofTextReport(port, groupID string) (string, string, error) {
	// Helper function to get raw file content
	getRawFileContent := func() ([]byte, string, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/pprof", port), nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, "", fmt.Errorf("JSON decode error: %v", err)
		}

		// Filter by group ID and get the latest entry
		var latestEntry *collect.Entry
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID {
				if latestEntry == nil || entry.Snapshot.Datetime.After(latestEntry.Snapshot.Datetime) {
					latestEntry = entry
				}
			}
		}

		if latestEntry == nil {
			return nil, "", fmt.Errorf("no matching entry found: group_id=%s", groupID)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/pprof/data/%s", port, latestEntry.Snapshot.ID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, "", fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file content: %v", err)
		}

		// Profile type is inferred from file path
		var profileType string
		if strings.Contains(latestEntry.Snapshot.ID, "cpu") {
			profileType = "cpu"
		} else if strings.Contains(latestEntry.Snapshot.ID, "heap") {
			profileType = "heap"
		} else {
			// Default profile type
			profileType = "unknown"
		}

		return fileContent, profileType, nil
	}

	// Get raw file content
	fileContent, profileType, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Convert to text report format
	textReport, err := pprof.GenerateTextReport(fileContent)
	if err != nil {
		return "", "", fmt.Errorf("pprof text report generation error: %v", err)
	}

	// Wrap the text report in JSON structure
	jsonWrapper := map[string]interface{}{
		"format":       "text_report",
		"profile_type": profileType,
		"report":       textReport,
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(jsonWrapper, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("JSON marshaling error: %v", err)
	}

	return string(jsonData), "application/json", nil
}

// pprof text report handler with specific entry ID
func handlePprofTextReportWithEntryID(port, groupID, entryID string) (string, string, error) {
	// Helper function to get raw file content for specific entry ID
	getRawFileContent := func() ([]byte, string, error) {
		// Get ID from metadata first
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%s/api/pprof", port), nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating request: %v", err)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("error calling API: %v", err)
		}
		defer resp.Body.Close()

		var entries []*collect.Entry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return nil, "", fmt.Errorf("JSON decode error: %v", err)
		}

		// 指定されたグループIDとエントリIDの組み合わせを確認
		var foundEntry *collect.Entry
		for _, entry := range entries {
			if entry.Snapshot != nil && entry.Snapshot.GroupId == groupID && entry.Snapshot.ID == entryID {
				foundEntry = entry
				break
			}
		}

		if foundEntry == nil {
			return nil, "", fmt.Errorf("no matching entry found: group_id=%s, entry_id=%s", groupID, entryID)
		}

		// Get data directly
		dataURL := fmt.Sprintf("http://localhost:%s/api/pprof/data/%s", port, entryID)
		log.Printf("Fetching data from: %s", dataURL)

		dataResp, err := http.Get(dataURL)
		if err != nil {
			return nil, "", fmt.Errorf("error fetching data: %v", err)
		}
		defer dataResp.Body.Close()

		if dataResp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("unexpected status code from data endpoint: %d", dataResp.StatusCode)
		}

		fileContent, err := io.ReadAll(dataResp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("error reading file content: %v", err)
		}

		// Profile type is inferred from file path
		var profileType string
		if strings.Contains(entryID, "cpu") {
			profileType = "cpu"
		} else if strings.Contains(entryID, "heap") {
			profileType = "heap"
		} else {
			// Default profile type
			profileType = "unknown"
		}

		return fileContent, profileType, nil
	}

	// Get raw file content for specific entry
	fileContent, profileType, err := getRawFileContent()
	if err != nil {
		return "", "", err
	}

	// Convert to text report format
	textReport, err := pprof.GenerateTextReport(fileContent)
	if err != nil {
		return "", "", fmt.Errorf("pprof text report generation error: %v", err)
	}

	// Wrap the text report in JSON structure
	jsonWrapper := map[string]interface{}{
		"format":       "text_report",
		"profile_type": profileType,
		"entry_id":     entryID,
		"report":       textReport,
	}

	// Convert to JSON
	jsonData, err := json.MarshalIndent(jsonWrapper, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("JSON marshaling error: %v", err)
	}

	return string(jsonData), "application/json", nil
}
