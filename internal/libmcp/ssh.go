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
func ExecuteSSHCommand(connectionName, host, port, username, password, keyPath, command string) (map[string]interface{}, error) {
	log.Println("Starting SSH command execution process")

	// Command is required
	if command == "" {
		log.Println("Error: No command specified for SSH execution")
		return nil, fmt.Errorf("Command is required")
	}
	log.Printf("Preparing to execute command: %s", command)

	// If connection name is specified, use that
	if connectionName != "" {
		log.Printf("Using named connection: '%s'", connectionName)

		// Register default settings if no connections are registered
		if len(sshConnections) == 0 {
			log.Println("No SSH connections registered, loading default settings")
			registerDefaultSSHConnection()
		}

		conn, exists := sshConnections[connectionName]
		if !exists {
			log.Printf("Error: Connection '%s' not found in registered connections", connectionName)
			return nil, fmt.Errorf("The specified connection setting '%s' does not exist", connectionName)
		}

		log.Printf("Found connection settings for '%s': host=%s, port=%s, user=%s",
			connectionName, conn.Host, conn.Port, conn.Username)

		host = conn.Host
		port = conn.Port
		username = conn.Username

		// Don't overwrite existing settings
		if password == "" {
			if conn.Password != "" {
				log.Printf("Using password from connection settings for '%s'", connectionName)
				password = conn.Password
			} else {
				log.Printf("No password specified in connection '%s'", connectionName)
			}
		} else {
			log.Printf("Using provided password instead of connection settings")
		}

		if keyPath == "" {
			if conn.KeyPath != "" {
				log.Printf("Using key path from connection settings: %s", conn.KeyPath)
				keyPath = conn.KeyPath
			} else {
				log.Printf("No key path specified in connection '%s'", connectionName)
			}
		} else {
			log.Printf("Using provided key path instead of connection settings")
		}
	} else {
		log.Printf("Using direct connection parameters: host=%s, port=%s, user=%s", host, port, username)
	}

	// Check required parameters
	if host == "" || username == "" {
		log.Println("Error: Host and username are required for SSH execution")
		return nil, fmt.Errorf("Host and username are required")
	}

	// Default port setting
	if port == "" {
		port = "22"
		log.Printf("No port specified, using default SSH port 22")
	} else {
		log.Printf("Using SSH port: %s", port)
	}

	// Execute SSH command
	var cmd *exec.Cmd
	if keyPath != "" {
		// If using private key authentication
		log.Printf("Using private key authentication with key: %s", keyPath)

		// Check if the key file exists
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			log.Printf("Warning: Private key file does not exist: %s", keyPath)
		}

		cmd = exec.Command("ssh",
			"-o", "StrictHostKeyChecking=no",
			"-i", keyPath,
			"-p", port,
			fmt.Sprintf("%s@%s", username, host),
			command)
		log.Printf("Created SSH command with key authentication: ssh -i %s -p %s %s@%s '%s'",
			keyPath, port, username, host, command)
	} else if password != "" {
		// If using password authentication (using sshpass)
		log.Printf("Using password authentication with sshpass")

		// Check if sshpass is installed
		if _, err := exec.LookPath("sshpass"); err != nil {
			log.Println("Warning: sshpass may not be installed, this could cause command execution to fail")
		}

		cmd = exec.Command("sshpass",
			"-p", password,
			"ssh",
			"-o", "StrictHostKeyChecking=no",
			"-p", port,
			fmt.Sprintf("%s@%s", username, host),
			command)
		log.Printf("Created SSH command with password authentication: sshpass -p *** ssh -p %s %s@%s '%s'",
			port, username, host, command)
	} else {
		log.Println("Error: No authentication method specified (neither password nor key)")
		return nil, fmt.Errorf("Please specify an authentication method (password or private key)")
	}

	// Get command output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	log.Printf("Executing SSH command to %s@%s...", username, host)
	err := cmd.Run()

	if err != nil {
		log.Printf("SSH command execution failed: %v", err)
	} else {
		log.Printf("SSH command execution completed successfully")
	}

	// Log output
	stdoutStr := stdout.String()
	stderrStr := stderr.String()

	if stdoutStr != "" {
		log.Printf("Command stdout (%d bytes): %s", len(stdoutStr), truncateIfTooLong(stdoutStr, 500))
	} else {
		log.Printf("Command stdout: <empty>")
	}

	if stderrStr != "" {
		log.Printf("Command stderr (%d bytes): %s", len(stderrStr), truncateIfTooLong(stderrStr, 500))
	} else {
		log.Printf("Command stderr: <empty>")
	}

	// Return results as a map
	result := map[string]interface{}{
		"host":       host,
		"command":    command,
		"stdout":     stdoutStr,
		"stderr":     stderrStr,
		"successful": err == nil,
	}

	if err != nil {
		result["error"] = err.Error()
	}

	log.Printf("SSH command execution process completed with status: %v", err == nil)
	return result, nil
}

// truncateIfTooLong truncates a string if it's too long and adds "..."
func truncateIfTooLong(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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
	log.Println("Starting to load SSH connection settings from environment variables")

	// Get all environment variables
	envVars := os.Environ()
	log.Printf("Processing %d environment variables for SSH connection settings", len(envVars))

	// Map for temporary storage of connection settings
	connectionMap := make(map[string]map[string]string)

	// Environment variable naming convention: SSH_CONN_<n>_<attribute>
	// Example: SSH_CONN_PROD_HOST=example.com
	prefix := "SSH_CONN_"

	sshEnvCount := 0
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

		sshEnvCount++
		// Remove prefix
		key = strings.TrimPrefix(key, prefix)
		log.Printf("Found SSH environment variable: %s", key)

		// Split into name and attribute (e.g., PROD_HOST → name=PROD, attribute=HOST)
		nameParts := strings.SplitN(key, "_", 2)
		if len(nameParts) != 2 {
			log.Printf("Warning: Malformed SSH environment variable: %s (expected format: %s<NAME>_<ATTRIBUTE>)", key, prefix)
			continue
		}

		name := nameParts[0]
		attr := nameParts[1]

		// Initialize map for this connection name (if needed)
		if _, exists := connectionMap[name]; !exists {
			log.Printf("Creating new connection settings for '%s'", name)
			connectionMap[name] = make(map[string]string)
		}

		// Save attribute value
		connectionMap[name][attr] = value
		log.Printf("Set %s=%s for connection '%s'", attr, value, name)
	}

	log.Printf("Found %d SSH environment variables for %d connection settings", sshEnvCount, len(connectionMap))

	// Get default user setting
	defaultUser := os.Getenv("SSH_DEFAULT_USER")
	if defaultUser == "" {
		defaultUser = os.Getenv("USER")
		if defaultUser == "" {
			defaultUser = "root"
		}
	}
	log.Printf("Using default SSH user: '%s'", defaultUser)

	// Convert connection settings to SSHConnection objects
	for name, attrs := range connectionMap {
		log.Printf("Processing connection settings for '%s'", name)
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
			log.Printf("Info: SSH connection setting '%s' will use default port 22", name)
		}

		// At least one of password or private key is needed
		password := attrs["PASS"]
		keyPath := attrs["KEY"]

		if password == "" && keyPath == "" {
			// Use default private key path
			log.Printf("No password or key path specified for '%s', using default key path", name)
			homeDir, err := os.UserHomeDir()
			if err == nil {
				keyPath = filepath.Join(homeDir, ".ssh", "id_ed25519")
				log.Printf("Using default key path: %s", keyPath)
			} else {
				keyPath = "/root/.ssh/id_ed25519"
				log.Printf("Could not determine user home directory, using fallback key path: %s", keyPath)
			}

			// Ensure SSH directory exists with correct permissions
			sshDir := filepath.Dir(keyPath)
			if _, err := os.Stat(sshDir); os.IsNotExist(err) {
				log.Printf("Creating SSH directory: %s", sshDir)
				if err := os.MkdirAll(sshDir, 0700); err != nil {
					log.Printf("Warning: Failed to create SSH directory %s: %v", sshDir, err)
				} else {
					log.Printf("Successfully created SSH directory with permissions 700")
				}
			} else {
				// Set correct permissions for existing directory
				if err := os.Chmod(sshDir, 0700); err != nil {
					log.Printf("Warning: Failed to set permissions on SSH directory %s: %v", sshDir, err)
				} else {
					log.Printf("Successfully set permissions 700 on SSH directory")
				}
			}

			// Set permissions for SSH key files if they exist
			if _, err := os.Stat(keyPath); err == nil {
				// Set private key permissions to 600
				if err := os.Chmod(keyPath, 0600); err != nil {
					log.Printf("Warning: Failed to set permissions on private key %s: %v", keyPath, err)
				} else {
					log.Printf("Successfully set permissions 600 on private key file")
				}
			}

			// Check for public key and set permissions to 644
			pubKeyPath := keyPath + ".pub"
			if _, err := os.Stat(pubKeyPath); err == nil {
				if err := os.Chmod(pubKeyPath, 0644); err != nil {
					log.Printf("Warning: Failed to set permissions on public key %s: %v", pubKeyPath, err)
				} else {
					log.Printf("Successfully set permissions 644 on public key file")
				}
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
		log.Printf("SSH connection setting '%s' registered with host '%s', user '%s', port '%s'", name, host, username, port)
	}

	log.Printf("Completed loading SSH connection settings, registered %d connections", len(sshConnections))
}

// Register SSH tools to the MCP server
func RegisterSSHTools(s *server.MCPServer) {
	loadSSHConnectionsFromEnv()

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
