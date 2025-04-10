package libmcp

import (
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer is an interface that provides functionality as an MCP server
type MCPServer interface {
	// Start the server
	Start(port string) error

	// Stop the server
	Stop() error

	RegisterSSHTools() error
}

// mcpServerImpl is an implementation of the MCPServer interface
type mcpServerImpl struct {
	server  *server.MCPServer
	started bool
}

// NewMCPServer creates a new MCP server instance
func NewMCPServer(name string, version string) MCPServer {
	s := server.NewMCPServer(
		name,
		version,
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	return &mcpServerImpl{
		server:  s,
		started: false,
	}
}

// Start starts the MCP server
func (s *mcpServerImpl) Start(port string) error {
	if s.started {
		return fmt.Errorf("Server is already running")
	}

	// Start server (run in a separate goroutine)
	go func() {
		log.Printf("Starting MCP server on port %s", port)
		sseServer := server.NewSSEServer(s.server)
		if err := sseServer.Start(":" + port); err != nil {
			log.Printf("MCP server error: %v", err)
		}
	}()

	s.started = true
	return nil
}

// Stop stops the MCP server
func (s *mcpServerImpl) Stop() error {
	if !s.started {
		return fmt.Errorf("Server is not running")
	}

	// If the current mcp-go library does not provide an explicit stop function,
	// it needs to be designed to stop automatically when the process ends

	s.started = false
	return nil
}

// RegisterSSHTools registers SSH related tools
func (s *mcpServerImpl) RegisterSSHTools() error {
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
			mcp.Description("SSH username (required if connection is not specified)"),
		),
		mcp.WithString("password",
			mcp.Description("SSH password (one authentication method is required)"),
		),
		mcp.WithString("key_path",
			mcp.Description("Path to SSH private key (one authentication method is required)"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Command to execute"),
		),
	)

	// SSH connection settings list retrieval tool
	sshConnectionListTool := mcp.NewTool("ssh_connection_list",
		mcp.WithDescription("Retrieves a list of registered SSH connection settings"),
	)

	// SSH connection settings registration tool
	sshConnectionRegisterTool := mcp.NewTool("ssh_connection_register",
		mcp.WithDescription("Registers new SSH connection settings"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the connection settings (identifier)"),
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
			mcp.Description("SSH username"),
		),
		mcp.WithString("password",
			mcp.Description("SSH password (required if key_path is not specified)"),
		),
		mcp.WithString("key_path",
			mcp.Description("Path to SSH private key (required if password is not specified)"),
		),
	)

	// Register SSH tool handlers
	s.server.AddTool(sshCommandTool, handleSSHCommand)
	s.server.AddTool(sshConnectionListTool, handleSSHConnectionList)
	s.server.AddTool(sshConnectionRegisterTool, handleSSHConnectionRegister)

	return nil
}

// RegisterToolsToServer registers all tools to the MCP server
func RegisterToolsToServer(mcpServer *server.MCPServer) error {
	// Register SSH tools
	RegisterSSHTools(mcpServer)

	return nil
}
