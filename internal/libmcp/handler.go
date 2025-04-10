package libmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
)

// SSH connection settings list retrieval handler
func handleSSHConnectionList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Retrieving SSH connection settings list")
	connections, err := ListSSHConnections()
	if err != nil {
		return nil, err
	}

	// Return results as JSON
	result := map[string]interface{}{
		"connections": connections,
		"count":       len(connections),
	}
	return newToolResultJSON(result)
}

// SSH connection settings registration handler
func handleSSHConnectionRegister(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Registering SSH connection settings")

	// Get parameters
	name, _ := request.Params.Arguments["name"].(string)
	host, _ := request.Params.Arguments["host"].(string)
	port, _ := request.Params.Arguments["port"].(string)
	username, _ := request.Params.Arguments["username"].(string)
	password, _ := request.Params.Arguments["password"].(string)
	keyPath, _ := request.Params.Arguments["key_path"].(string)

	err := RegisterSSHConnection(name, host, port, username, password, keyPath)
	if err != nil {
		return nil, err
	}

	return newToolResultJSON(map[string]interface{}{
		"status": "success",
		"name":   name,
	})
}

// SSH command execution handler
func handleSSHCommand(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Executing SSH command on remote host")

	// Get parameters
	connectionName, _ := request.Params.Arguments["connection"].(string)
	host, _ := request.Params.Arguments["host"].(string)
	port, _ := request.Params.Arguments["port"].(string)
	username, _ := request.Params.Arguments["username"].(string)
	password, _ := request.Params.Arguments["password"].(string)
	keyPath, _ := request.Params.Arguments["key_path"].(string)
	command, _ := request.Params.Arguments["command"].(string)

	result, err := ExecuteSSHCommand(connectionName, host, port, username, password, keyPath, command)
	if err != nil {
		return nil, err
	}

	return newToolResultJSON(result)
}

// Helper function to return tool results in JSON format
func newToolResultJSON(data interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("JSON encoding error: %v", err)
	}
	return mcp.NewToolResultText(string(jsonData)), nil
}
