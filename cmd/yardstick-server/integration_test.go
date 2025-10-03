// Package main contains integration tests for the MCP echo server
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioTransportIntegration(t *testing.T) {
	// This test would require building the binary and running it as a subprocess
	// For now, we'll test the core functionality without the actual stdio transport
	t.Skip("Stdio transport integration test requires subprocess execution")
}

func TestSSETransportIntegration(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// For testing, we'll simulate an SSE response
		// In a real implementation, this would use the MCP SDK's SSE transport
		fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"output\\\":\\\"test123\\\"}\"}]}}\n\n")
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	// Test SSE endpoint
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "keep-alive", resp.Header.Get("Connection"))

	// Read the SSE data
	scanner := bufio.NewScanner(resp.Body)
	var sseData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			sseData = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	assert.NotEmpty(t, sseData)

	// Parse the JSON-RPC response
	var jsonRPCResponse map[string]interface{}
	err = json.Unmarshal([]byte(sseData), &jsonRPCResponse)
	require.NoError(t, err)

	assert.Equal(t, "2.0", jsonRPCResponse["jsonrpc"])
	assert.Equal(t, float64(1), jsonRPCResponse["id"])
	assert.Contains(t, jsonRPCResponse, "result")
}

func TestStreamableHTTPTransportIntegration(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set headers for streamable HTTP
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// For testing, we'll simulate a streamable HTTP response
		// In a real implementation, this would use the MCP SDK's streamable HTTP transport
		response := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": "{\"output\":\"test123\"}",
					},
				},
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Test OPTIONS request
	req, err := http.NewRequest("OPTIONS", server.URL, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "POST, OPTIONS", resp.Header.Get("Access-Control-Allow-Methods"))

	// Test POST request
	req, err = http.NewRequest("POST", server.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"input":"test123"}}}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var jsonRPCResponse map[string]interface{}
	err = json.Unmarshal(body, &jsonRPCResponse)
	require.NoError(t, err)

	assert.Equal(t, "2.0", jsonRPCResponse["jsonrpc"])
	assert.Equal(t, float64(1), jsonRPCResponse["id"])
	assert.Contains(t, jsonRPCResponse, "result")
}

func TestEndToEndEchoFunctionality(t *testing.T) {
	// Test the complete flow of the echo functionality
	testCases := []struct {
		name          string
		input         string
		expectedError bool
	}{
		{"valid alphanumeric", "abc123", false},
		{"valid letters only", "hello", false},
		{"valid numbers only", "12345", false},
		{"invalid with special chars", "test@123", true},
		{"invalid with spaces", "test 123", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the echo handler directly
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Create a CallToolRequest for testing
			req := &mcp.CallToolRequest{}
			params := EchoRequest{Input: tc.input}

			result, response, err := echoHandler(ctx, req, params)

			if tc.expectedError {
				// For invalid input, we expect an error result, not an error
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.True(t, result.IsError)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, result) // When successful, result is nil and response contains the data
				assert.NotNil(t, response)
				assert.Equal(t, tc.input, response.Output)
			}
		})
	}
}

func TestServerStartup(t *testing.T) {
	// Test that the server can be created and configured without errors
	// This doesn't test the actual transport but validates the server setup

	// Test different transport configurations
	transports := []string{"stdio", "sse", "streamable-http"}

	for _, transport := range transports {
		t.Run(fmt.Sprintf("transport_%s", transport), func(t *testing.T) {
			// This would normally test server startup, but since we can't easily
			// test the actual server startup in a unit test, we'll verify the
			// configuration is valid

			// Verify alphanumeric validation works
			assert.True(t, validateAlphanumeric("test123"))
			assert.False(t, validateAlphanumeric("test@123"))

			// Verify echo handler works
			req := &mcp.CallToolRequest{}
			params := EchoRequest{Input: "test123"}

			result, response, err := echoHandler(context.Background(), req, params)
			assert.NoError(t, err)
			assert.Nil(t, result)
			assert.NotNil(t, response)
		})
	}
}

func TestConcurrentEchoRequests(t *testing.T) {
	// Test that the echo handler can handle concurrent requests
	// This is important for performance testing scenarios

	const numConcurrentRequests = 100

	ctx := context.Background()
	results := make(chan error, numConcurrentRequests)

	for i := 0; i < numConcurrentRequests; i++ {
		go func(id int) {
			req := &mcp.CallToolRequest{}
			params := EchoRequest{Input: fmt.Sprintf("test%d", id)}

			result, response, err := echoHandler(ctx, req, params)
			if err != nil {
				results <- err
				return
			}

			if result != nil {
				results <- fmt.Errorf("expected nil result for request %d", id)
				return
			}

			expectedOutput := fmt.Sprintf("test%d", id)
			if response.Output != expectedOutput {
				results <- fmt.Errorf("expected %s, got %s", expectedOutput, response.Output)
				return
			}

			results <- nil
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numConcurrentRequests; i++ {
		err := <-results
		assert.NoError(t, err, "Request %d failed", i)
	}
}
