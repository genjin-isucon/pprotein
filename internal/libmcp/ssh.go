package libmcp

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// For storing SSH connection information
type SSHConnection struct {
	Name     string `json:"name"`     // Connection name (identifier)
	Host     string `json:"host"`     // Hostname or IP address
	Port     string `json:"port"`     // Port number
	Username string `json:"username"` // Username
	Password string `json:"password"` // Password (optional)
	KeyPath  string `json:"key_path"` // Private key path (optional)
}

// Map of saved SSH connections
var sshConnections = make(map[string]*SSHConnection)

// RegisterSSHConnection registers new SSH connection settings
func RegisterSSHConnection(name, host, port, username, password, keyPath string) error {
	// Check required parameters
	if name == "" || host == "" || username == "" {
		return fmt.Errorf("Name, host, and username are required")
	}

	// Default port setting
	if port == "" {
		port = "22"
	}

	// At least one authentication method is required
	if password == "" && keyPath == "" {
		return fmt.Errorf("Please specify either a password or a private key path")
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
	return nil
}

// ListSSHConnections returns a list of registered SSH connection settings
func ListSSHConnections() ([]map[string]interface{}, error) {
	// Register default settings if no connections are registered
	if len(sshConnections) == 0 {
		registerDefaultSSHConnection()
	}

	// Convert connection settings list to slice
	connections := make([]map[string]interface{}, 0, len(sshConnections))
	for _, conn := range sshConnections {
		// Mask sensitive information
		connMap := map[string]interface{}{
			"name":     conn.Name,
			"host":     conn.Host,
			"port":     conn.Port,
			"username": conn.Username,
		}

		if conn.Password != "" {
			connMap["password"] = "********"
		}

		if conn.KeyPath != "" {
			connMap["key_path"] = conn.KeyPath
		}

		connections = append(connections, connMap)
	}

	return connections, nil
}

// ExecuteSSHCommand executes an SSH command on a remote host
func ExecuteSSHCommand(connection, host, port, username, password, keyPath, command string) (map[string]interface{}, error) {
	log.Println("Executing SSH command on remote host")

	// Command is required
	if command == "" {
		return nil, fmt.Errorf("Command is required")
	}

	// If connection name is specified, use that
	if connection != "" {
		// Register default settings if no connections are registered
		if len(sshConnections) == 0 {
			registerDefaultSSHConnection()
		}

		conn, exists := sshConnections[connection]
		if !exists {
			return nil, fmt.Errorf("The specified connection setting '%s' does not exist", connection)
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

	// Execute SSH command
	var cmd *exec.Cmd
	if keyPath != "" {
		// If using private key authentication
		cmd = exec.Command("ssh",
			"-o", "StrictHostKeyChecking=no",
			"-i", keyPath,
			"-p", port,
			fmt.Sprintf("%s@%s", username, host),
			command)
	} else if password != "" {
		// If using password authentication (using sshpass)
		cmd = exec.Command("sshpass",
			"-p", password,
			"ssh",
			"-o", "StrictHostKeyChecking=no",
			"-p", port,
			fmt.Sprintf("%s@%s", username, host),
			command)
	} else {
		return nil, fmt.Errorf("Please specify an authentication method (password or private key)")
	}

	// Get command output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Return results as a map
	result := map[string]interface{}{
		"host":       host,
		"command":    command,
		"stdout":     stdout.String(),
		"stderr":     stderr.String(),
		"successful": err == nil,
	}

	if err != nil {
		result["error"] = err.Error()
	}

	return result, nil
}

// registerDefaultSSHConnection registers default SSH connection settings
func registerDefaultSSHConnection() {
	// Load connection settings from environment variables
	loadSSHConnectionsFromEnv()

	// Add default settings if there are no settings in environment variables
	if len(sshConnections) == 0 {
		// Get default private key path
		homeDir, err := os.UserHomeDir()
		keyPath := "/root/.ssh/id_ed25519" // Default value
		if err == nil {
			keyPath = filepath.Join(homeDir, ".ssh", "id_ed25519")
		}

		// User name priority:
		// 1. Environment variable SSH_DEFAULT_USER
		// 2. Environment variable USER
		defaultUsername := os.Getenv("SSH_DEFAULT_USER")
		if defaultUsername == "" {
			defaultUsername = os.Getenv("USER")
			if defaultUsername == "" {
				defaultUsername = "root"
			}
		}

		defaultConn := &SSHConnection{
			Name:     "localhost",
			Host:     "localhost",
			Port:     "22",
			Username: defaultUsername,
			KeyPath:  keyPath,
		}

		sshConnections[defaultConn.Name] = defaultConn
		log.Printf("Default SSH connection setting '%s' registered with user '%s'", defaultConn.Name, defaultConn.Username)
	}
}

// loadSSHConnectionsFromEnv loads SSH connection settings from environment variables
func loadSSHConnectionsFromEnv() {
	// Get all environment variables
	envVars := os.Environ()

	// Map for temporary storage of connection settings
	connectionMap := make(map[string]map[string]string)

	// Environment variable naming convention: SSH_CONN_<n>_<attribute>
	// Example: SSH_CONN_PROD_HOST=example.com
	prefix := "SSH_CONN_"

	for _, env := range envVars {
		// Split environment variable name and value
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check if it's an environment variable for SSH connection settings
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// Remove prefix
		key = strings.TrimPrefix(key, prefix)

		// Split into name and attribute (e.g., PROD_HOST → name=PROD, attribute=HOST)
		nameParts := strings.SplitN(key, "_", 2)
		if len(nameParts) != 2 {
			continue
		}

		name := nameParts[0]
		attr := nameParts[1]

		// Initialize map for this connection name (if needed)
		if _, exists := connectionMap[name]; !exists {
			connectionMap[name] = make(map[string]string)
		}

		// Save attribute value
		connectionMap[name][attr] = value
	}

	// Get default user setting
	defaultUser := os.Getenv("SSH_DEFAULT_USER")
	if defaultUser == "" {
		defaultUser = os.Getenv("USER")
		if defaultUser == "" {
			defaultUser = "root"
		}
	}

	// Convert connection settings to SSHConnection objects
	for name, attrs := range connectionMap {
		host, hostExists := attrs["HOST"]

		// Host is required
		if !hostExists {
			log.Printf("Warning: SSH connection setting '%s' does not have a host setting", name)
			continue
		}

		// Username setting, use default if USER attribute is not specified
		username, userExists := attrs["USER"]
		if !userExists {
			username = defaultUser
			log.Printf("Info: SSH connection setting '%s' will use default user '%s'", name, username)
		}

		port := attrs["PORT"]
		if port == "" {
			port = "22" // Default port
		}

		// At least one of password or private key is needed
		password := attrs["PASS"]
		keyPath := attrs["KEY"]

		if password == "" && keyPath == "" {
			// Use default private key path
			homeDir, err := os.UserHomeDir()
			if err == nil {
				keyPath = filepath.Join(homeDir, ".ssh", "id_ed25519")
			} else {
				keyPath = "/root/.ssh/id_ed25519"
			}
		}

		conn := &SSHConnection{
			Name:     name,
			Host:     host,
			Port:     port,
			Username: username,
			Password: password,
			KeyPath:  keyPath,
		}

		sshConnections[name] = conn
		log.Printf("SSH connection setting '%s' registered with user '%s' from environment variables", name, username)
	}
}

// Register SSH tools to the MCP server
func RegisterSSHTools(s *server.MCPServer) {
	// SSH command execution tool
	sshCommandTool := mcp.NewTool("ssh_command",
		mcp.WithDescription("Execute commands on a remote host via SSH"),
		mcp.WithString("connection",
			mcp.Description("Name of the registered connection settings to use (optional)"),
		),
		mcp.WithString("host",
			mcp.Description("Target hostname or IP address (required if connection is not specified)"),
		),
		mcp.WithString("port",
			mcp.Description("SSH port number (default: 22)"),
		),
		mcp.WithString("username",
			mcp.Description("Username for SSH connection (required if connection is not specified)"),
		),
		mcp.WithString("password",
			mcp.Description("Password for SSH connection (required if key_path is not specified)"),
		),
		mcp.WithString("key_path",
			mcp.Description("Path to the private key file for SSH connection (required if password is not specified)"),
		),
		mcp.WithString("command",
			mcp.Description("Command to execute"),
			mcp.Required(),
		),
	)

	// Tool to get the list of SSH connection settings
	sshConnectionListTool := mcp.NewTool("ssh_connection_list",
		mcp.WithDescription("Retrieves a list of registered SSH connection settings"),
	)

	// Tool to register SSH connection settings
	sshConnectionRegisterTool := mcp.NewTool("ssh_connection_register",
		mcp.WithDescription("Registers new SSH connection settings"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the connection settings"),
		),
		mcp.WithString("host",
			mcp.Required(),
			mcp.Description("Target hostname or IP address"),
		),
		mcp.WithString("port",
			mcp.Description("SSH port number (default: 22)"),
			mcp.DefaultString("22"),
		),
		mcp.WithString("username",
			mcp.Required(),
			mcp.Description("Username for SSH connection"),
		),
		mcp.WithString("password",
			mcp.Description("Password for SSH connection (required if key_path is not specified)"),
		),
		mcp.WithString("key_path",
			mcp.Description("Path to the private key file for SSH connection (required if password is not specified)"),
		),
	)

	// Register SSH tool handlers
	s.AddTool(sshCommandTool, handleSSHCommand)
	s.AddTool(sshConnectionListTool, handleSSHConnectionList)
	s.AddTool(sshConnectionRegisterTool, handleSSHConnectionRegister)
}
