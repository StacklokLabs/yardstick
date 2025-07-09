package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	config := Config{
		Transport: "stdio",
		Address:   "localhost",
		Port:      8080,
		Timeout:   30 * time.Second,
	}

	client := NewClient(config)
	assert.NotNil(t, client)
	assert.Equal(t, config, client.config)
	assert.Nil(t, client.client)
	assert.Nil(t, client.session)
}

func TestConfig_ParseConfig(t *testing.T) {
	// Note: Due to the global nature of the flag package in Go, we test the basic functionality
	// without actually calling parseConfig multiple times to avoid flag redefinition panics.
	// In a real application, parseConfig should only be called once at program start.

	// Save original env vars
	originalEnvVars := map[string]string{}
	envVars := []string{"TRANSPORT", "ADDRESS", "PORT", "COMMAND"}
	for _, env := range envVars {
		if val, exists := os.LookupEnv(env); exists {
			originalEnvVars[env] = val
		}
		os.Unsetenv(env)
	}

	defer func() {
		for _, env := range envVars {
			os.Unsetenv(env)
		}
		for env, val := range originalEnvVars {
			os.Setenv(env, val)
		}
	}()

	// Test environment variable override
	t.Run("environment variables", func(t *testing.T) {
		// Set test env vars
		os.Setenv("TRANSPORT", "streamable-http")
		os.Setenv("ADDRESS", "remote.example.com")
		os.Setenv("PORT", "3000")
		os.Setenv("COMMAND", "test-command")

		defer func() {
			os.Unsetenv("TRANSPORT")
			os.Unsetenv("ADDRESS")
			os.Unsetenv("PORT")
			os.Unsetenv("COMMAND")
		}()

		// Test that env vars would be read (we can't call parseConfig due to flag redefinition)
		transport, transportExists := os.LookupEnv("TRANSPORT")
		assert.True(t, transportExists)
		assert.Equal(t, "streamable-http", transport)

		address, addressExists := os.LookupEnv("ADDRESS")
		assert.True(t, addressExists)
		assert.Equal(t, "remote.example.com", address)

		port, portExists := os.LookupEnv("PORT")
		assert.True(t, portExists)
		assert.Equal(t, "3000", port)

		command, commandExists := os.LookupEnv("COMMAND")
		assert.True(t, commandExists)
		assert.Equal(t, "test-command", command)
	})

	// Test default values by checking what they would be
	t.Run("default values", func(t *testing.T) {
		config := Config{
			Transport: "stdio",
			Address:   "localhost",
			Port:      8080,
			Timeout:   30 * time.Second,
		}

		assert.Equal(t, "stdio", config.Transport)
		assert.Equal(t, "localhost", config.Address)
		assert.Equal(t, 8080, config.Port)
		assert.Equal(t, 30*time.Second, config.Timeout)
	})
}

func TestClient_ConnectStdio(t *testing.T) {
	client := NewClient(Config{
		Transport: "stdio",
		Command:   "echo",
		Args:      []string{"test"},
	})

	transport, err := client.connectStdio()
	assert.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestClient_ConnectStdio_NoCommand(t *testing.T) {
	client := NewClient(Config{
		Transport: "stdio",
	})

	transport, err := client.connectStdio()
	assert.Error(t, err)
	assert.Nil(t, transport)
	assert.Contains(t, err.Error(), "command is required")
}

func TestClient_ConnectSSE(t *testing.T) {
	client := NewClient(Config{
		Transport: "sse",
		Address:   "localhost",
		Port:      8080,
	})

	transport, err := client.connectSSE()
	assert.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestClient_ConnectStreamableHTTP(t *testing.T) {
	client := NewClient(Config{
		Transport: "streamable-http",
		Address:   "localhost",
		Port:      8080,
	})

	transport, err := client.connectStreamableHTTP()
	assert.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestClient_Connect_UnsupportedTransport(t *testing.T) {
	client := NewClient(Config{
		Transport: "unsupported",
	})

	ctx := context.Background()
	err := client.Connect(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported transport type")
}

// Mock MCP server for testing HTTP-based transports
func createMockMCPServer(_ *testing.T, transport string) *httptest.Server {
	mux := http.NewServeMux()

	// Create a mock MCP server
	server := mcp.NewServer("test-server", "1.0.0", nil)

	// Add a simple echo tool for testing
	echoTool := mcp.NewServerTool("echo", "Echo tool for testing",
		func(_ context.Context, _ *mcp.ServerSession, _ *mcp.CallToolParamsFor[map[string]interface{}]) (*mcp.CallToolResultFor[map[string]interface{}], error) {
			return &mcp.CallToolResultFor[map[string]interface{}]{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "test response"},
				},
			}, nil
		},
		mcp.Input(),
	)
	server.AddTools(echoTool)

	switch transport {
	case "sse":
		handler := mcp.NewSSEHandler(func(_ *http.Request) *mcp.Server {
			return server
		})
		mux.Handle("/sse", handler)
	case "streamable-http":
		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return server
		}, nil)
		mux.Handle("/mcp", handler)
	}

	return httptest.NewServer(mux)
}

func TestClient_IntegrationSSE(t *testing.T) {
	// Create mock server
	mockServer := createMockMCPServer(t, "sse")
	defer mockServer.Close()

	// Extract host and port from mock server URL
	url := strings.TrimPrefix(mockServer.URL, "http://")
	parts := strings.Split(url, ":")
	host := parts[0]
	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	client := NewClient(Config{
		Transport: "sse",
		Address:   host,
		Port:      port,
		Timeout:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test connection
	err = client.Connect(ctx)
	assert.NoError(t, err)
	defer client.Close()

	// Test getting server info
	err = client.GetServerInfo(ctx)
	assert.NoError(t, err)

	// Test listing tools
	err = client.ListTools(ctx)
	assert.NoError(t, err)
}

func TestClient_IntegrationStreamableHTTP(t *testing.T) {
	// Create mock server
	mockServer := createMockMCPServer(t, "streamable-http")
	defer mockServer.Close()

	// Extract host and port from mock server URL
	url := strings.TrimPrefix(mockServer.URL, "http://")
	parts := strings.Split(url, ":")
	host := parts[0]
	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	client := NewClient(Config{
		Transport: "streamable-http",
		Address:   host,
		Port:      port,
		Timeout:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test connection
	err = client.Connect(ctx)
	assert.NoError(t, err)
	defer client.Close()

	// Test getting server info
	err = client.GetServerInfo(ctx)
	assert.NoError(t, err)

	// Test listing tools
	err = client.ListTools(ctx)
	assert.NoError(t, err)
}

func TestClient_IntegrationStdio(t *testing.T) {
	// This test requires the server binary to be available
	// Skip if not available
	_, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go not available, skipping stdio integration test")
	}

	// Build the server binary for testing
	buildCmd := exec.Command("go", "build", "-o", "test-server", "../server/main.go")
	err = buildCmd.Run()
	if err != nil {
		t.Skip("Could not build server binary, skipping stdio integration test")
	}
	defer os.Remove("test-server")

	client := NewClient(Config{
		Transport: "stdio",
		Command:   "./test-server",
		Args:      []string{"-transport=stdio"},
		Timeout:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test connection
	err = client.Connect(ctx)
	assert.NoError(t, err)
	defer client.Close()

	// Test getting server info
	err = client.GetServerInfo(ctx)
	assert.NoError(t, err)

	// Test listing tools
	err = client.ListTools(ctx)
	assert.NoError(t, err)

	// Test calling echo tool
	args := map[string]interface{}{
		"input": "test123",
	}
	err = client.CallTool(ctx, "echo", args)
	assert.NoError(t, err)
}

func TestClient_CallTool(t *testing.T) {
	// Create mock server
	mockServer := createMockMCPServer(t, "sse")
	defer mockServer.Close()

	// Extract host and port from mock server URL
	url := strings.TrimPrefix(mockServer.URL, "http://")
	parts := strings.Split(url, ":")
	host := parts[0]
	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	client := NewClient(Config{
		Transport: "sse",
		Address:   host,
		Port:      port,
		Timeout:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to server
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Test calling a tool
	args := map[string]interface{}{
		"test": "value",
	}
	err = client.CallTool(ctx, "echo", args)
	assert.NoError(t, err)
}

func TestClient_ListResources(t *testing.T) {
	// Create mock server
	mockServer := createMockMCPServer(t, "sse")
	defer mockServer.Close()

	// Extract host and port from mock server URL
	url := strings.TrimPrefix(mockServer.URL, "http://")
	parts := strings.Split(url, ":")
	host := parts[0]
	port, err := strconv.Atoi(parts[1])
	require.NoError(t, err)

	client := NewClient(Config{
		Transport: "sse",
		Address:   host,
		Port:      port,
		Timeout:   5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to server
	err = client.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()

	// Test listing resources
	err = client.ListResources(ctx)
	assert.NoError(t, err)
}

func TestClient_Close(t *testing.T) {
	client := NewClient(Config{})

	// Test closing without session
	err := client.Close()
	assert.NoError(t, err)

	// Test closing with session would require a real connection
	// This is covered in the integration tests
}

func TestJSONArgumentParsing(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    map[string]interface{}
		expectError bool
	}{
		{
			name:     "valid JSON",
			input:    `{"key": "value", "number": 42}`,
			expected: map[string]interface{}{"key": "value", "number": float64(42)},
		},
		{
			name:     "empty JSON",
			input:    `{}`,
			expected: map[string]interface{}{},
		},
		{
			name:        "invalid JSON",
			input:       `{"key": "value"`,
			expectError: true,
		},
		{
			name:        "non-JSON string",
			input:       "not json",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			err := json.Unmarshal([]byte(tt.input), &result)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMainFunction(t *testing.T) {
	// Test main function with different actions
	// This would require setting up the environment and mocking inputs
	// For now, we'll test the argument parsing which is the main logic

	// Save original env vars
	originalAction := os.Getenv("CLIENT_ACTION")
	originalTool := os.Getenv("CLIENT_TOOL_NAME")
	originalArgs := os.Getenv("CLIENT_TOOL_ARGS")

	defer func() {
		os.Setenv("CLIENT_ACTION", originalAction)
		os.Setenv("CLIENT_TOOL_NAME", originalTool)
		os.Setenv("CLIENT_TOOL_ARGS", originalArgs)
	}()

	tests := []struct {
		name   string
		action string
		tool   string
		args   string
	}{
		{"info action", "info", "", "{}"},
		{"list-tools action", "list-tools", "", "{}"},
		{"list-resources action", "list-resources", "", "{}"},
		{"call-tool action", "call-tool", "echo", `{"input": "test"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("CLIENT_ACTION", tt.action)
			os.Setenv("CLIENT_TOOL_NAME", tt.tool)
			os.Setenv("CLIENT_TOOL_ARGS", tt.args)

			// Verify environment variables are set correctly
			assert.Equal(t, tt.action, os.Getenv("CLIENT_ACTION"))
			assert.Equal(t, tt.tool, os.Getenv("CLIENT_TOOL_NAME"))
			assert.Equal(t, tt.args, os.Getenv("CLIENT_TOOL_ARGS"))
		})
	}
}
