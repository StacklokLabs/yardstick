package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestValidateAlphanumeric(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"valid alphanumeric", "abc123", true},
		{"only letters", "abcdef", true},
		{"only numbers", "123456", true},
		{"mixed case", "AbC123", true},
		{"empty string", "", false},
		{"with spaces", "abc 123", false},
		{"with special chars", "abc!123", false},
		{"with dash", "abc-123", false},
		{"with underscore", "abc_123", false},
		{"with dots", "abc.123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateAlphanumeric(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEchoHandler(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "valid alphanumeric input",
			input:          "test123",
			expectedOutput: "test123",
			expectError:    false,
		},
		{
			name:           "valid letters only",
			input:          "hello",
			expectedOutput: "hello",
			expectError:    false,
		},
		{
			name:           "valid numbers only",
			input:          "12345",
			expectedOutput: "12345",
			expectError:    false,
		},
		{
			name:        "invalid with special characters",
			input:       "test@123",
			expectError: true,
		},
		{
			name:        "invalid with spaces",
			input:       "test 123",
			expectError: true,
		},
		{
			name:        "invalid empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with the new API
			req := &mcp.CallToolRequest{}
			params := EchoRequest{Input: tt.input}

			// Call handler
			result, response, err := echoHandler(context.Background(), req, params)

			if tt.expectError {
				// For invalid input, we expect an error result, not an error
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.True(t, result.IsError)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, result)
				assert.NotNil(t, response)
				assert.Equal(t, tt.expectedOutput, response.Output)
			}
		})
	}
}

func TestEchoRequestValidation(t *testing.T) {
	// Test that EchoRequest struct works properly
	req := EchoRequest{Input: "test123"}
	assert.Equal(t, "test123", req.Input)
}

func TestEchoResponseCreation(t *testing.T) {
	// Test that EchoResponse struct works properly
	response := EchoResponse{Output: "test123"}
	assert.Equal(t, "test123", response.Output)
}

func TestCheckAuth_HeaderAuth(t *testing.T) {
	// Save original values
	origHeader := authHeader
	origValue := authValue
	defer func() {
		authHeader = origHeader
		authValue = origValue
	}()

	// Set auth config
	authHeader = "X-Auth-Token"
	authValue = "secret123"

	// Create request with correct header
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	assert.NoError(t, err)
	req.Header.Set("X-Auth-Token", "secret123")

	// Should pass authentication
	err = checkAuth(req)
	assert.NoError(t, err)
}

func TestCheckAuth_HeaderAuth_Fail(t *testing.T) {
	// Save original values
	origHeader := authHeader
	origValue := authValue
	defer func() {
		authHeader = origHeader
		authValue = origValue
	}()

	// Set auth config
	authHeader = "X-Auth-Token"
	authValue = "secret123"

	// Test with wrong header value
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	assert.NoError(t, err)
	req.Header.Set("X-Auth-Token", "wrongvalue")

	err = checkAuth(req)
	assert.Error(t, err)
	assert.Equal(t, "unauthorized", err.Error())

	// Test with missing header
	req2, err := http.NewRequest(http.MethodGet, "/test", nil)
	assert.NoError(t, err)

	err = checkAuth(req2)
	assert.Error(t, err)
	assert.Equal(t, "unauthorized", err.Error())
}

func TestCheckAuth_Disabled(t *testing.T) {
	// Save original values
	origHeader := authHeader
	origValue := authValue
	defer func() {
		authHeader = origHeader
		authValue = origValue
	}()

	// Auth disabled when authHeader is empty
	authHeader = ""
	authValue = ""

	// Create request without any auth header
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	assert.NoError(t, err)

	// Should pass since auth is disabled
	err = checkAuth(req)
	assert.NoError(t, err)
}
