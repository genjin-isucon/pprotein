package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kaz/pprotein/internal/libmcp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SetupMCP sets up and starts a new MCP server
func SetupMCP(port string, apiPort string) {
	// Debug log
	log.Println("Setting up MCP server on port", port)

	// Create a new MCP server
	s := server.NewMCPServer(
		"pprotein MCP Server",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Create a tool to get the group list
	groupListTool := mcp.NewTool("group_list",
		mcp.WithDescription("Retrieves a list of group IDs"),
	)

	// Register handler for the group list retrieval tool
	s.AddTool(groupListTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := handleGroupList(apiPort)
		if err != nil {
			return nil, err
		}

		jsonData, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %v", err)
		}

		return mcp.NewToolResultText(string(jsonData)), nil
	})

	// Create a tool to get group data
	groupDataTool := mcp.NewTool("group_data",
		mcp.WithDescription("Retrieves data for a specific group ID"),
		mcp.WithString("group_id",
			mcp.Description("The ID of the group to retrieve data for"),
			mcp.Required(),
		),
	)

	// Register handler for group data retrieval tool
	s.AddTool(groupDataTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		groupID, ok := request.Params.Arguments["group_id"].(string)
		if !ok || groupID == "" {
			return nil, fmt.Errorf("group_id is required")
		}

		result, err := handleGroupData(apiPort, groupID)
		if err != nil {
			return nil, err
		}

		jsonData, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText(string(jsonData)), nil
	})

	// Create group file retrieval tool
	groupFileTool := mcp.NewTool("group_file",
		mcp.WithDescription("Retrieves the physical file for a specific group ID and type"),
		mcp.WithString("group_id",
			mcp.Description("The ID of the group to retrieve the file for"),
			mcp.Required(),
		),
		mcp.WithString("type",
			mcp.Description("The type of file to retrieve (pprof, httplog, slowlog, memo)"),
			mcp.Required(),
		),
		mcp.WithString("entry_id",
			mcp.Description("The specific entry ID (optional, defaults to the first entry)"),
		),
	)

	// Register handler for group file retrieval tool
	s.AddTool(groupFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		groupID, ok := request.Params.Arguments["group_id"].(string)
		if !ok || groupID == "" {
			return nil, fmt.Errorf("group_id is required")
		}

		fileType, ok := request.Params.Arguments["type"].(string)
		if !ok || fileType == "" {
			return nil, fmt.Errorf("type is required")
		}

		// Check if the type is valid
		validTypes := map[string]bool{"pprof": true, "httplog": true, "slowlog": true, "memo": true}
		if !validTypes[fileType] {
			return nil, fmt.Errorf("invalid type: %s, must be one of pprof, httplog, slowlog, memo", fileType)
		}

		entryID, _ := request.Params.Arguments["entry_id"].(string)

		fileContent, contentType, err := handleGroupFile(apiPort, groupID, fileType, entryID)
		if err != nil {
			return nil, err
		}

		// For JSON content, return as JSON
		if contentType == "application/json" {
			// Return JSON response as is
			return mcp.NewToolResultText(string(fileContent)), nil
		} else {
			// For non-JSON content, Base64 encode and return
			resultMap := map[string]interface{}{
				"content_type": contentType,
				"data":         base64.StdEncoding.EncodeToString(fileContent),
			}
			jsonData, err := json.Marshal(resultMap)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %v", err)
			}
			return mcp.NewToolResultText(string(jsonData)), nil
		}
	})

	// Create alp configuration file retrieval tool
	alpConfigGetTool := mcp.NewTool("alp_config_get",
		mcp.WithDescription("Retrieves the alp configuration file"),
	)

	// Register handler for alp configuration file retrieval tool
	s.AddTool(alpConfigGetTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		configContent, err := handleGetAlpConfig(apiPort)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText(configContent), nil
	})

	// Create alp configuration file update tool
	alpConfigUpdateTool := mcp.NewTool("alp_config_update",
		mcp.WithDescription("Updates the alp configuration file"),
		mcp.WithString("config",
			mcp.Description("The YAML formatted content of the configuration file to update"),
			mcp.Required(),
		),
	)

	// Register handler for alp configuration file update tool
	s.AddTool(alpConfigUpdateTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		config, ok := request.Params.Arguments["config"].(string)
		if !ok || config == "" {
			return nil, fmt.Errorf("config is required")
		}

		err := handleUpdateAlpConfig(apiPort, config)
		if err != nil {
			return nil, err
		}

		return mcp.NewToolResultText("Configuration file updated successfully"), nil
	})

	// Create MySQL connection tool
	connectTool := mcp.NewTool("mysql_connect",
		mcp.WithDescription("Establishes a connection to the MySQL database and saves the connection information for use in subsequent queries"),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("MySQL host address"),
		),
		mcp.WithString("port",
			mcp.Description("MySQL port"),
			mcp.DefaultString("3306"),
		),
		mcp.WithString("username",
			mcp.Required(),
			mcp.Description("MySQL username"),
		),
		mcp.WithString("password",
			mcp.Required(),
			mcp.Description("MySQL password"),
		),
		mcp.WithString("database",
			mcp.Description("MySQL database name (optional)"),
			mcp.DefaultString(""),
		),
	)

	// Create query tool
	queryTool := mcp.NewTool("mysql_query",
		mcp.WithDescription("Executes an SQL query against the currently connected MySQL database"),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("The SQL query to execute"),
		),
	)

	// Create database list tool
	listDatabasesTool := mcp.NewTool("mysql_list_databases",
		mcp.WithDescription("Retrieves a list of all databases available on the currently connected MySQL server"),
	)

	// Create table list tool
	listTablesTool := mcp.NewTool("mysql_list_tables",
		mcp.WithDescription("Retrieves a list of all tables in the specified database, or in the currently connected database if no database is specified"),
		mcp.WithString("database",
			mcp.Description("Database name (optional, if not specified, uses the current connection)"),
		),
	)

	// Create table details tool
	describeTableTool := mcp.NewTool("mysql_describe_table",
		mcp.WithDescription("Retrieves detailed information about the structure of the specified table in the currently connected MySQL database"),
		mcp.WithString("table",
			mcp.Required(),
			mcp.Description("Table name"),
		),
	)

	// Register tool handlers
	s.AddTool(connectTool, handleMySQLConnect)
	s.AddTool(queryTool, handleMySQLQuery)
	s.AddTool(listDatabasesTool, handleMySQLListDatabases)
	s.AddTool(listTablesTool, handleMySQLListTables)
	s.AddTool(describeTableTool, handleMySQLDescribeTable)

	// Register resource handler to the server
	resource := mcp.NewResource("pprotein://groups", "application/json")
	s.AddResource(resource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		result, err := handleGroupList(apiPort)
		if err != nil {
			return nil, err
		}

		jsonData, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		// Use TextResourceContents
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "pprotein://groups",
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		}, nil
	})

	// Register tools to the server
	libmcp.RegisterToolsToServer(s)

	// Start server (run in a separate goroutine)
	go func() {
		log.Printf("Starting MCP server on port %s", port)
		sseServer := server.NewSSEServer(s)
		if err := sseServer.Start(":" + port); err != nil {
			log.Printf("MCP server error: %v", err)
		}
	}()

	log.Println("MCP server setup complete on port", port)
}
