package main

import (
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/kaz/pprotein/integration/echov4"
	"github.com/kaz/pprotein/internal/collect"
	"github.com/kaz/pprotein/internal/collect/group"
	"github.com/kaz/pprotein/internal/event"
	"github.com/kaz/pprotein/internal/extproc/alp"
	"github.com/kaz/pprotein/internal/extproc/slp"
	"github.com/kaz/pprotein/internal/mcp"
	"github.com/kaz/pprotein/internal/memo"
	pprofcollect "github.com/kaz/pprotein/internal/pprof"
	"github.com/kaz/pprotein/internal/storage"
	"github.com/kaz/pprotein/view"
	"github.com/labstack/echo/v4"
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

// For storing MySQL connection information
type MySQLConnection struct {
	Host     string
	Port     string
	Username string
	Password string
	Database string
}

// MCP server settings
func setupMCP(mcpPort string, apiPort string) {
	mcp.SetupMCP(mcpPort, apiPort)
}

func start() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	mcpPort := os.Getenv("MCP_PORT")
	if mcpPort == "" {
		mcpPort = "9001"
	}

	store, err := storage.New("data")
	if err != nil {
		return err
	}

	e := echo.New()
	echov4.Integrate(e)

	fs, err := view.FS()
	if err != nil {
		return err
	}
	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(fs))))

	api := e.Group("/api", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			return next(c)
		}
	})

	hub := event.NewHub()
	hub.RegisterHandlers(api.Group("/event"))

	pprofOpts := &collect.Options{
		Type:     "pprof",
		Ext:      "-pprof.pb.gz",
		Store:    store,
		EventHub: hub,
	}
	if err := pprofcollect.NewHandler(pprofOpts).Register(api.Group("/pprof")); err != nil {
		return err
	}

	alpOpts := &collect.Options{
		Type:     "httplog",
		Ext:      "-httplog.log",
		Store:    store,
		EventHub: hub,
	}
	alpHandler, err := alp.NewHandler(alpOpts, store)
	if err != nil {
		return err
	}
	if err := alpHandler.Register(api.Group("/httplog")); err != nil {
		return err
	}

	slpOpts := &collect.Options{
		Type:     "slowlog",
		Ext:      "-slowlog.log",
		Store:    store,
		EventHub: hub,
	}
	slpHandler, err := slp.NewHandler(slpOpts, store)
	if err != nil {
		return err
	}
	if err := slpHandler.Register(api.Group("/slowlog")); err != nil {
		return err
	}

	memoOpts := &collect.Options{
		Type:     "memo",
		Ext:      "-memo.log",
		Store:    store,
		EventHub: hub,
	}
	if err := memo.NewHandler(memoOpts).Register(api.Group("/memo")); err != nil {
		return err
	}

	grp, err := group.NewCollector(store, port)
	if err != nil {
		return err
	}
	grp.RegisterHandlers(api.Group("/group"))

	// Call setupMCP first and start the MCP server on a separate port
	setupMCP(mcpPort, port)

	// Implementation of a simple deletion endpoint
	api.DELETE("/data/:type/:id", func(c echo.Context) error {
		dataType := c.Param("type")
		id := c.Param("id")

		// Delete metadata from KV store
		if err := store.Delete(dataType, id); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
		}

		// Also delete the file (if it exists)
		path, _ := store.GetFilePath(id)
		os.Remove(path) // Ignore error (file may not exist)

		return c.JSON(http.StatusOK, map[string]string{
			"status": "deleted",
			"type":   dataType,
			"id":     id,
		})
	})

	// Display MCP port in server startup log as well
	log.Printf("Starting pprotein server on port %s, MCP server on port %s", port, mcpPort)

	// Start the main Echo server
	return e.Start(":" + port)
}

func main() {
	if err := start(); err != nil {
		panic(err)
	}
}
