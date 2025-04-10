package mcp

import (
	"database/sql"
)

// MCP request structure
type MCPRequest struct {
	Function  string                 `json:"function"`
	Arguments map[string]interface{} `json:"arguments"`
}

// MCP response structure
type MCPResponse struct {
	Result interface{} `json:"result"`
	Error  *MCPError   `json:"error,omitempty"`
}

// MCP error structure
type MCPError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Storage for MySQL connection information
type MySQLConnection struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
	Conn     *sql.DB
}

// Active MySQL connection
var activeConnection *MySQLConnection
