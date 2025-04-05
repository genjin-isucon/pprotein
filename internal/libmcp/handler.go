package libmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os/exec"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleIPAddress is a handler that retrieves IP addresses from network interfaces
func handleIPAddress(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Implementation similar to GetIPAddressInfo
	interfaceInfos := []map[string]interface{}{}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		if len(addrs) > 0 {
			ipAddresses := []string{}
			for _, addr := range addrs {
				ip, _, err := net.ParseCIDR(addr.String())
				if err == nil {
					ipAddresses = append(ipAddresses, ip.String())
				} else {
					ipAddresses = append(ipAddresses, addr.String())
				}
			}

			interfaceInfo := map[string]interface{}{
				"name":         iface.Name,
				"mac_address":  iface.HardwareAddr.String(),
				"ip_addresses": ipAddresses,
				"mtu":          iface.MTU,
				"flags":        iface.Flags.String(),
			}
			interfaceInfos = append(interfaceInfos, interfaceInfo)
		}
	}

	result := map[string]interface{}{
		"interfaces": interfaceInfos,
		"count":      len(interfaceInfos),
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("JSON encoding error: %v", err)
	}

	return mcp.NewToolResultText(string(jsonData)), nil
}

// SSH connection settings list retrieval handler
func handleSSHConnectionList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Println("Retrieving SSH connection settings list")

	// Convert connection settings list to slice
	connections := make([]SSHConnection, 0, len(sshConnections))
	for _, conn := range sshConnections {
		// Mask sensitive information
		masked := *conn
		if masked.Password != "" {
			masked.Password = "********"
		}
		connections = append(connections, masked)
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

	// Check required parameters
	if name == "" || host == "" || username == "" {
		return nil, fmt.Errorf("Name, host, and username are required")
	}

	// Default port setting
	if port == "" {
		port = "22"
	}

	// At least one authentication method is required
	if password == "" && keyPath == "" {
		return nil, fmt.Errorf("Please specify either a password or a private key path")
	}

	// Save connection settings
	sshConnections[name] = &SSHConnection{
		Name:     name,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		KeyPath:  keyPath,
	}

	log.Printf("SSH connection setting '%s' has been registered", name)

	// Return results as JSON
	result := map[string]interface{}{
		"status": "success",
		"name":   name,
	}

	return newToolResultJSON(result)
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

	// Command is required
	if command == "" {
		return nil, fmt.Errorf("Command is required")
	}

	// If connection name is specified, use that
	if connectionName != "" {
		conn, exists := sshConnections[connectionName]
		if !exists {
			return nil, fmt.Errorf("The specified connection setting '%s' does not exist", connectionName)
		}

		host = conn.Host
		port = conn.Port
		username = conn.Username

		// Don't overwrite existing settings
		if password == "" {
			password = conn.Password
		}
		if keyPath == "" {
			keyPath = conn.KeyPath
		}
	}

	// Check required parameters
	if host == "" || username == "" {
		return nil, fmt.Errorf("Host and username are required")
	}

	// Default port setting
	if port == "" {
		port = "22"
	}

	// Build arguments for SSH command execution
	args := []string{}

	// Specify private key
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}

	// Specify port
	args = append(args, "-p", port)

	// Specify host
	hostArg := username + "@" + host
	args = append(args, hostArg)

	// Specify command
	args = append(args, command)

	// Execute SSH command
	cmd := exec.Command("ssh", args...)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Build result
	result := map[string]interface{}{
		"stdout": stdout.String(),
		"stderr": stderr.String(),
		"status": "success",
	}

	if err != nil {
		result["error"] = err.Error()
		result["status"] = "error"
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
