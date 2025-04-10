package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/mark3labs/mcp-go/mcp"
)

// MySQL connection handler
func handleMySQLConnect(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Connecting to MySQL")

	// Get parameters
	host, _ := request.Params.Arguments["host"].(string)
	port, _ := request.Params.Arguments["port"].(string)
	username, _ := request.Params.Arguments["username"].(string)
	password, _ := request.Params.Arguments["password"].(string)
	database, _ := request.Params.Arguments["database"].(string)

	// Check required parameters
	if host == "" || username == "" || password == "" {
		return nil, fmt.Errorf("Host, username, and password are required")
	}

	// Save connection information
	activeConnection = &MySQLConnection{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Database: database,
	}

	// Test connection
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		username, password, host, port, database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL connection error: %v", err)
	}
	defer db.Close()

	// Test connection (Ping)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Failed to ping MySQL server: %v", err)
	}

	result := map[string]interface{}{
		"status":   "Connection successful",
		"host":     host,
		"port":     port,
		"username": username,
		"database": database,
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// MySQL query execution handler
func handleMySQLQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Executing MySQL query")

	// Check connection
	if activeConnection == nil {
		return nil, fmt.Errorf("Not connected to MySQL. Please run mysql_connect first")
	}

	// Get parameters
	sqlQuery, _ := request.Params.Arguments["sql"].(string)

	if sqlQuery == "" {
		return nil, fmt.Errorf("SQL query is required")
	}

	// Create DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		activeConnection.Username,
		activeConnection.Password,
		activeConnection.Host,
		activeConnection.Port,
		activeConnection.Database)

	// Database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL connection error: %v", err)
	}
	defer db.Close()

	// Execute query
	rows, err := db.Query(sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("Query execution error: %v", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("Error getting column information: %v", err)
	}

	// Slice to store results
	var results []map[string]interface{}

	// Buffer for scanning row data
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range columns {
		valuePtrs[i] = &values[i]
	}

	// Get row data
	for rows.Next() {
		err := rows.Scan(valuePtrs...)
		if err != nil {
			return nil, fmt.Errorf("Data scan error: %v", err)
		}

		// Convert row data to map
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]

			// Convert byte array to string
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}

		results = append(results, row)
	}

	// Error check
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during query execution: %v", err)
	}

	// Return results in JSON format
	response := map[string]interface{}{
		"columns": columns,
		"rows":    results,
		"count":   len(results),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// Database list retrieval handler
func handleMySQLListDatabases(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Retrieving MySQL database list")

	// Check connection
	if activeConnection == nil {
		return nil, fmt.Errorf("Not connected to MySQL. Please run mysql_connect first")
	}

	// Create DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/",
		activeConnection.Username,
		activeConnection.Password,
		activeConnection.Host,
		activeConnection.Port)

	// Database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL connection error: %v", err)
	}
	defer db.Close()

	// Database list retrieval query
	rows, err := db.Query("SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("Error retrieving database list: %v", err)
	}
	defer rows.Close()

	// Slice to store results
	var databases []string

	// Get database names
	for rows.Next() {
		var dbName string
		if err := rows.Scan(&dbName); err != nil {
			return nil, fmt.Errorf("Data scan error: %v", err)
		}
		databases = append(databases, dbName)
	}

	// Error check
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during query execution: %v", err)
	}

	// Return results in JSON format
	response := map[string]interface{}{
		"databases": databases,
		"count":     len(databases),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// Table list retrieval handler
func handleMySQLListTables(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Retrieving MySQL table list")

	// Check connection
	if activeConnection == nil {
		return nil, fmt.Errorf("Not connected to MySQL. Please run mysql_connect first")
	}

	// Get parameters
	dbName, _ := request.Params.Arguments["database"].(string)

	// If database name is not specified, use the database of the current connection
	if dbName == "" {
		dbName = activeConnection.Database
		if dbName == "" {
			return nil, fmt.Errorf("Database not specified")
		}
	}

	// Create DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		activeConnection.Username,
		activeConnection.Password,
		activeConnection.Host,
		activeConnection.Port,
		dbName)

	// Database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL connection error: %v", err)
	}
	defer db.Close()

	// Table list retrieval query
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return nil, fmt.Errorf("Error retrieving table list: %v", err)
	}
	defer rows.Close()

	// Slice to store results
	var tables []string

	// Get table names
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("Data scan error: %v", err)
		}
		tables = append(tables, tableName)
	}

	// Error check
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during query execution: %v", err)
	}

	// Return results in JSON format
	response := map[string]interface{}{
		"database": dbName,
		"tables":   tables,
		"count":    len(tables),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// Table details retrieval handler
func handleMySQLDescribeTable(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Retrieving MySQL table details")

	// Check connection
	if activeConnection == nil {
		return nil, fmt.Errorf("Not connected to MySQL. Please run mysql_connect first")
	}

	// Get parameters
	tableName, _ := request.Params.Arguments["table"].(string)

	if tableName == "" {
		return nil, fmt.Errorf("Table name is required")
	}

	// Check database name
	dbName := activeConnection.Database
	if dbName == "" {
		return nil, fmt.Errorf("Database not specified")
	}

	// Create DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		activeConnection.Username,
		activeConnection.Password,
		activeConnection.Host,
		activeConnection.Port,
		dbName)

	// Database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("MySQL connection error: %v", err)
	}
	defer db.Close()

	// Table details retrieval query
	rows, err := db.Query(fmt.Sprintf("DESCRIBE %s", tableName))
	if err != nil {
		return nil, fmt.Errorf("Error retrieving table details: %v", err)
	}
	defer rows.Close()

	// Slice to store results
	var columns []map[string]interface{}

	// Get column information
	for rows.Next() {
		var field, fieldType, null, key, defaultValue, extra string
		if err := rows.Scan(&field, &fieldType, &null, &key, &defaultValue, &extra); err != nil {
			return nil, fmt.Errorf("Data scan error: %v", err)
		}

		columns = append(columns, map[string]interface{}{
			"Field":   field,
			"Type":    fieldType,
			"Null":    null,
			"Key":     key,
			"Default": defaultValue,
			"Extra":   extra,
		})
	}

	// Error check
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("Error during query execution: %v", err)
	}

	// Return results in JSON format
	response := map[string]interface{}{
		"database": dbName,
		"table":    tableName,
		"columns":  columns,
		"count":    len(columns),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}
