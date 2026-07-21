package main

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

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
				// Result is nil when no metadata is present (original behavior)
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

func TestEchoHandler_WithMetadata(t *testing.T) {
	// Create request with metadata
	requestMeta := mcp.Meta{
		"progressToken": "test123",
		"customKey":     "customValue",
	}
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Meta: requestMeta,
		},
	}
	params := EchoRequest{Input: "hello"}

	// Call handler
	result, response, err := echoHandler(context.Background(), req, params)

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, response)
	assert.Equal(t, "hello", response.Output)

	// Verify metadata is echoed back
	assert.NotNil(t, result.Meta)
	assert.Equal(t, requestMeta["progressToken"], result.Meta["progressToken"])
	assert.Equal(t, requestMeta["customKey"], result.Meta["customKey"])
}

func TestEchoHandler_WithMultipleMetadataFields(t *testing.T) {
	tests := []struct {
		name     string
		metadata mcp.Meta
	}{
		{
			name:     "with progressToken",
			metadata: mcp.Meta{"progressToken": "token123"},
		},
		{
			name: "with multiple fields",
			metadata: mcp.Meta{
				"progressToken": "token456",
				"requestId":     "req789",
				"clientInfo":    "test-client",
			},
		},
		{
			name:     "with nested metadata",
			metadata: mcp.Meta{"debug": map[string]any{"level": "verbose"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with metadata
			req := &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{
					Meta: tt.metadata,
				},
			}
			params := EchoRequest{Input: "test123"}

			// Call handler
			result, response, err := echoHandler(context.Background(), req, params)

			// Verify response
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotNil(t, response)
			assert.Equal(t, "test123", response.Output)

			// Verify metadata is echoed back
			assert.NotNil(t, result.Meta)
			for key, expectedValue := range tt.metadata {
				actualValue, exists := result.Meta[key]
				assert.True(t, exists, "Expected metadata key %s to exist", key)
				assert.Equal(t, expectedValue, actualValue, "Metadata value mismatch for key %s", key)
			}
		})
	}
}

func TestEchoHandler_WithoutMetadata(t *testing.T) {
	// Create request without metadata
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{},
	}
	params := EchoRequest{Input: "test123"}

	// Call handler
	result, response, err := echoHandler(context.Background(), req, params)

	// Verify response
	assert.NoError(t, err)
	// Result should be nil when no metadata is present (original behavior)
	assert.Nil(t, result)
	assert.NotNil(t, response)
	assert.Equal(t, "test123", response.Output)
}

func TestEchoHandler_WithMetadata_ValidationError(t *testing.T) {
	// Create request with metadata but invalid input
	requestMeta := mcp.Meta{
		"progressToken": "error-token",
		"requestId":     "error-req-123",
	}
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Meta: requestMeta,
		},
	}
	params := EchoRequest{Input: "invalid@input!"} // Contains special characters

	// Call handler
	result, response, err := echoHandler(context.Background(), req, params)

	// Verify response
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsError, "Expected IsError to be true for invalid input")
	assert.NotEmpty(t, result.Content, "Expected error message in Content")

	// Verify metadata is still echoed back even in error case
	assert.NotNil(t, result.Meta, "Metadata should be echoed back even on validation error")
	assert.Equal(t, requestMeta["progressToken"], result.Meta["progressToken"])
	assert.Equal(t, requestMeta["requestId"], result.Meta["requestId"])

	// Verify response is empty for error case
	assert.Empty(t, response.Output)
}

// noopHandler is an mcp.MethodHandler stub used to observe whether the
// fault-injection middleware calls through to the next handler.
func noopHandler(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return nil, nil
}

func TestFaultMiddleware_EchoModeIsNoop(t *testing.T) {
	// decide() always reports decisionNormal in echo mode, so every method
	// (including initialize/ping) must pass straight through unchanged.
	cs := &counterState{mode: "echo"}
	br := &barrier{n: 2, timeout: time.Second}
	mw := newFaultMiddleware("echo", cs, br)

	wantResult := &mcp.CallToolResult{}
	handler := mw(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return wantResult, nil
	})

	for _, method := range []string{methodInitialize, methodPing, "tools/call", "tools/list"} {
		result, err := handler(context.Background(), method, nil)
		assert.NoError(t, err)
		assert.Same(t, wantResult, result)
	}
}

func TestFaultMiddleware_HangMode_BlocksNonInitPingCalls(t *testing.T) {
	cs := &counterState{mode: modeHang, hangAfter: 1}
	br := &barrier{n: 2, timeout: time.Second}
	mw := newFaultMiddleware(modeHang, cs, br)

	called := make(chan struct{})
	handler := mw(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		close(called)
		return noopHandler(ctx, method, req)
	})

	go func() {
		_, _ = handler(context.Background(), "tools/call", nil)
	}()

	select {
	case <-called:
		t.Fatal("next must not be called once decide() reports decisionHang")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFaultMiddleware_Crash_Subprocess(t *testing.T) {
	if os.Getenv("YARDSTICK_CRASH_HELPER") == "1" {
		cs := &counterState{mode: modeCrash, crashAfter: 1}
		br := &barrier{n: 2, timeout: time.Second}
		mw := newFaultMiddleware(modeCrash, cs, br)
		handler := mw(noopHandler)
		_, _ = handler(context.Background(), "tools/call", nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestFaultMiddleware_Crash_Subprocess")
	cmd.Env = append(os.Environ(), "YARDSTICK_CRASH_HELPER=1")
	err := cmd.Run()

	var exitErr *exec.ExitError
	if assert.ErrorAs(t, err, &exitErr) {
		assert.Equal(t, 1, exitErr.ExitCode())
	}
}

func TestFaultMiddleware_BarrierMode_InitializeAndPingBypassBarrier(t *testing.T) {
	// n=2 with a single caller would hang forever without the bypass, and the
	// barrier's own 1s safety timer would otherwise let this test pass
	// anyway (just slower) even if the bypass regressed. Assert the calls
	// return well under that timeout, so a regression fails fast instead of
	// silently slowing down. Passing a nil counterState also proves this
	// path never touches cs.decide.
	br := &barrier{n: 2, timeout: time.Second}
	mw := newFaultMiddleware(modeBarrier, nil, br)
	handler := mw(noopHandler)

	for _, method := range []string{methodInitialize, methodPing, methodDiscover, notificationInitialized} {
		start := time.Now()
		_, err := handler(context.Background(), method, nil)
		assert.NoError(t, err)
		assert.Less(t, time.Since(start), 500*time.Millisecond)
	}
}

func TestFaultMiddleware_BarrierMode_ReleasesAfterNArrivals(t *testing.T) {
	br := &barrier{n: 2, timeout: time.Second}
	mw := newFaultMiddleware(modeBarrier, nil, br)

	var calls int32
	handler := mw(func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		atomic.AddInt32(&calls, 1)
		return noopHandler(ctx, method, req)
	})

	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, _ = handler(context.Background(), "tools/call", nil)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("barrier did not release both waiters")
		}
	}
	assert.EqualValues(t, 2, atomic.LoadInt32(&calls))
}

func TestParseConfig_BackendModeEnvVars(t *testing.T) {
	origMode, origBarrierN, origHangAfter, origCrashAfter, origTimeout :=
		backendMode, barrierN, hangAfterN, crashAfterN, barrierTimeout
	defer func() {
		backendMode, barrierN, hangAfterN, crashAfterN, barrierTimeout =
			origMode, origBarrierN, origHangAfter, origCrashAfter, origTimeout
	}()

	for k, v := range map[string]string{
		"BACKEND_MODE":            "hang",
		"BARRIER_N":               "5",
		"HANG_AFTER_N":            "3",
		"CRASH_AFTER_N":           "4",
		"BARRIER_TIMEOUT_SECONDS": "7",
	} {
		t.Setenv(k, v)
	}

	parseConfig()

	assert.Equal(t, "hang", backendMode)
	assert.Equal(t, 5, barrierN)
	assert.Equal(t, 3, hangAfterN)
	assert.Equal(t, 4, crashAfterN)
	assert.Equal(t, 7*time.Second, barrierTimeout)
}

func TestValidateFaultConfig(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		barrierN    int
		hangAfterN  int
		crashAfterN int
		wantErr     bool
	}{
		{name: "echo mode ignores all thresholds", mode: "echo", barrierN: 0, hangAfterN: 0, crashAfterN: 0},
		{name: "barrier mode valid", mode: modeBarrier, barrierN: 1, hangAfterN: 0, crashAfterN: 0},
		{name: "barrier mode zero rejected", mode: modeBarrier, barrierN: 0, hangAfterN: 1, crashAfterN: 1, wantErr: true},
		{name: "barrier mode negative rejected", mode: modeBarrier, barrierN: -1, hangAfterN: 1, crashAfterN: 1, wantErr: true},
		{name: "hang mode valid", mode: modeHang, barrierN: 0, hangAfterN: 1, crashAfterN: 0},
		{name: "hang mode zero rejected", mode: modeHang, barrierN: 1, hangAfterN: 0, crashAfterN: 1, wantErr: true},
		{name: "hang mode negative rejected", mode: modeHang, barrierN: 1, hangAfterN: -1, crashAfterN: 1, wantErr: true},
		{name: "crash mode valid", mode: modeCrash, barrierN: 0, hangAfterN: 0, crashAfterN: 1},
		{name: "crash mode zero rejected", mode: modeCrash, barrierN: 1, hangAfterN: 1, crashAfterN: 0, wantErr: true},
		{name: "crash mode negative rejected", mode: modeCrash, barrierN: 1, hangAfterN: 1, crashAfterN: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFaultConfig(tt.mode, tt.barrierN, tt.hangAfterN, tt.crashAfterN)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
