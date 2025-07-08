// Package main provides a client for connecting to MCP servers.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config holds the client configuration
type Config struct {
	Transport string
	Address   string
	Port      int
	Command   string
	Args      []string
	Timeout   time.Duration
}

// Client represents an MCP client
type Client struct {
	config  Config
	client  *mcp.Client
	session *mcp.ClientSession
}

// NewClient creates a new MCP client
func NewClient(config Config) *Client {
	return &Client{
		config: config,
	}
}

// Connect establishes a connection to the MCP server
func (c *Client) Connect(ctx context.Context) error {
	var transport mcp.Transport
	var err error

	switch c.config.Transport {
	case "stdio":
		transport, err = c.connectStdio()
	case "sse":
		transport, err = c.connectSSE()
	case "streamable-http":
		transport, err = c.connectStreamableHTTP()
	default:
		return fmt.Errorf("unsupported transport type: %s", c.config.Transport)
	}

	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}

	c.client = mcp.NewClient("yardstick-client", "1.0.0", nil)
	session, err := c.client.Connect(ctx, transport)
	if err != nil {
		return err
	}
	c.session = session
	return nil
}

// connectStdio creates a stdio transport connection
func (c *Client) connectStdio() (mcp.Transport, error) {
	if c.config.Command == "" {
		return nil, fmt.Errorf("command is required for stdio transport")
	}

	// #nosec G204 - Command and args are from user configuration, this is intentional
	cmd := exec.Command(c.config.Command, c.config.Args...)
	return mcp.NewCommandTransport(cmd), nil
}

// connectSSE creates an SSE transport connection
//
//nolint:unparam
func (c *Client) connectSSE() (mcp.Transport, error) {
	url := fmt.Sprintf("http://%s:%d/sse", c.config.Address, c.config.Port)
	return mcp.NewSSEClientTransport(url, nil), nil
}

// connectStreamableHTTP creates a streamable HTTP transport connection
//
//nolint:unparam
func (c *Client) connectStreamableHTTP() (mcp.Transport, error) {
	url := fmt.Sprintf("http://%s:%d/mcp", c.config.Address, c.config.Port)
	return mcp.NewStreamableClientTransport(url, nil), nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if c.session != nil {
		return c.session.Close()
	}
	return nil
}

// ListTools lists all available tools from the server
func (c *Client) ListTools(ctx context.Context) error {
	tools, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return fmt.Errorf("failed to list tools: %w", err)
	}

	fmt.Printf("Available tools (%d):\n", len(tools.Tools))
	for _, tool := range tools.Tools {
		fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
	}
	return nil
}

// CallTool calls a tool on the server
func (c *Client) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) error {
	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return fmt.Errorf("failed to call tool %s: %w", toolName, err)
	}

	contentJSON, err := json.Marshal(result.Content)
	if err != nil {
		fmt.Printf("Failed to marshal content to JSON: %v\n", err)
	} else {
		fmt.Printf("%v\n", string(contentJSON))
	}

	return nil
}

// ListResources lists all available resources from the server
func (c *Client) ListResources(ctx context.Context) error {
	resources, err := c.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		return fmt.Errorf("failed to list resources: %w", err)
	}

	fmt.Printf("Available resources (%d):\n", len(resources.Resources))
	for _, resource := range resources.Resources {
		fmt.Printf("  - %s: %s\n", resource.URI, resource.Description)
	}
	return nil
}

// GetServerInfo gets information about the server
func (c *Client) GetServerInfo(ctx context.Context) error {
	// Try to ping the server to confirm connection
	err := c.session.Ping(ctx, &mcp.PingParams{})
	if err != nil {
		return fmt.Errorf("failed to ping server: %w", err)
	}

	fmt.Printf("Server Info:\n")
	fmt.Printf("  Session ID: %s\n", c.session.ID())
	fmt.Printf("  Connection: Active\n")

	// Try to get basic server capabilities by listing tools
	tools, err := c.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err == nil {
		fmt.Printf("  Tools Available: %d\n", len(tools.Tools))
	}

	// Try to get resources
	resources, err := c.session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err == nil {
		fmt.Printf("  Resources Available: %d\n", len(resources.Resources))
	}

	return nil
}

// parseConfig parses command line flags and environment variables
func parseConfig() Config {
	config := Config{
		Transport: "stdio",
		Address:   "localhost",
		Port:      8080,
		Timeout:   30 * time.Second,
	}

	var toolName string
	var toolArgs string
	var action string

	flag.StringVar(&config.Transport, "transport", config.Transport, "Transport type: stdio, sse, or streamable-http")
	flag.StringVar(&config.Address, "address", config.Address, "Server address (for HTTP-based transports)")
	flag.IntVar(&config.Port, "port", config.Port, "Server port (for HTTP-based transports)")
	flag.StringVar(&config.Command, "command", "", "Command to run for stdio transport")
	flag.DurationVar(&config.Timeout, "timeout", config.Timeout, "Connection timeout")
	flag.StringVar(&action, "action", "info", "Action to perform: info, list-tools, list-resources, call-tool")
	flag.StringVar(&toolName, "tool", "", "Tool name to call (for call-tool action)")
	flag.StringVar(&toolArgs, "args", "{}", "Tool arguments as JSON (for call-tool action)")

	flag.Parse()

	// Parse remaining args as command args for stdio transport
	if config.Command != "" {
		config.Args = flag.Args()
	}

	// Use environment variables if provided
	if t, ok := os.LookupEnv("TRANSPORT"); ok {
		config.Transport = t
	}
	if a, ok := os.LookupEnv("ADDRESS"); ok {
		config.Address = a
	}
	if p, ok := os.LookupEnv("PORT"); ok {
		if intValue, err := strconv.Atoi(p); err == nil {
			config.Port = intValue
		}
	}
	if c, ok := os.LookupEnv("COMMAND"); ok {
		config.Command = c
	}

	// Store action and tool info in a way we can access them
	_ = os.Setenv("CLIENT_ACTION", action)
	_ = os.Setenv("CLIENT_TOOL_NAME", toolName)
	_ = os.Setenv("CLIENT_TOOL_ARGS", toolArgs)

	return config
}

func main() {
	config := parseConfig()

	client := NewClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	// Connect to the server
	if err := client.Connect(ctx); err != nil {
		log.Fatalf("Failed to connect to MCP server: %v", err)
	}
	defer client.Close()

	// Get action from environment (set during parseConfig)
	action := os.Getenv("CLIENT_ACTION")
	toolName := os.Getenv("CLIENT_TOOL_NAME")
	toolArgs := os.Getenv("CLIENT_TOOL_ARGS")

	// Perform the requested action
	switch action {
	case "info":
		if err := client.GetServerInfo(ctx); err != nil {
			log.Fatalf("Failed to get server info: %v", err)
		}

	case "list-tools":
		if err := client.ListTools(ctx); err != nil {
			log.Fatalf("Failed to list tools: %v", err)
		}

	case "list-resources":
		if err := client.ListResources(ctx); err != nil {
			log.Fatalf("Failed to list resources: %v", err)
		}

	case "call-tool":
		if toolName == "" {
			log.Fatal("Tool name is required for call-tool action")
		}

		var arguments map[string]interface{}
		if err := json.Unmarshal([]byte(toolArgs), &arguments); err != nil {
			log.Fatalf("Failed to parse tool arguments: %v", err)
		}

		if err := client.CallTool(ctx, toolName, arguments); err != nil {
			log.Fatalf("Failed to call tool: %v", err)
		}

	default:
		log.Fatalf("Unknown action: %s", action)
	}
}
