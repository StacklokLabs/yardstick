package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EchoRequest represents the request for the echo tool
type EchoRequest struct {
	Input string `json:"input"`
}

// EchoResponse represents the response from the echo tool
type EchoResponse struct {
	Output string `json:"output"`
}

// modeBarrier is a BACKEND_MODE value handled entirely by the barrier
// middleware branch; it has no counterState decision since every call
// (other than initialize/ping) just waits at the barrier.
const modeBarrier = "barrier"

var alphanumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
var transport string
var port int
var stateless bool
var authHeader string
var authValue string
var backendMode string
var barrierN int
var hangAfterN int
var crashAfterN int
var barrierTimeout time.Duration

func validateAlphanumeric(input string) bool {
	return alphanumericRegex.MatchString(input)
}

func checkAuth(r *http.Request) error {
	if authHeader == "" {
		return nil
	}
	if r.Header.Get(authHeader) != authValue {
		return errors.New("unauthorized")
	}
	return nil
}

func authWrapper(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := checkAuth(r); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func echoHandler(_ context.Context, req *mcp.CallToolRequest, params EchoRequest) (*mcp.CallToolResult, EchoResponse, error) {
	// Extract metadata from request to echo back in response
	var metadata mcp.Meta
	if req.Params != nil && len(req.Params.Meta) > 0 {
		log.Printf("echo tool called with metadata: %+v", req.Params.Meta)
		metadata = req.Params.Meta
	}

	if !validateAlphanumeric(params.Input) {
		// Echo back metadata even in error cases
		result := &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "input must be alphanumeric only"}},
			IsError: true,
		}
		if len(metadata) > 0 {
			result.Meta = metadata
		}
		return result, EchoResponse{}, nil
	}

	response := EchoResponse{
		Output: params.Input,
	}

	// Return result with metadata echoed back only if metadata is present
	// When result is nil, SDK auto-populates Content from response
	// When result is non-nil with empty Content, SDK should still auto-populate Content
	if len(metadata) > 0 {
		result := &mcp.CallToolResult{
			Meta: metadata,
		}
		return result, response, nil
	}

	// No metadata, return nil result (original behavior)
	return nil, response, nil
}

// newFaultMiddleware builds the receiving middleware that drives the
// server's fault-injection behavior, uniformly across every transport.
//
// In "barrier" mode, every non-lifecycle call (see isLifecycleMethod) blocks
// on br.join() before being handled. Otherwise cs.decide reports whether the
// call should hang, crash, or proceed normally; in the default "echo" mode,
// decide always reports decisionNormal, making this a pure passthrough.
func newFaultMiddleware(mode string, cs *counterState, br *barrier) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if mode == modeBarrier {
				if !isLifecycleMethod(method) {
					select {
					case <-br.join():
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return next(ctx, method, req)
			}

			switch cs.decide(method) { //nolint:exhaustive // decisionNormal falls through to the return below
			case decisionHang:
				time.Sleep(1<<63 - 1)
				return nil, ctx.Err()
			case decisionCrash:
				os.Exit(1)
				return nil, nil
			}
			return next(ctx, method, req)
		}
	}
}

func main() {
	// Parse command line flags
	parseConfig()

	// Create MCP server
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "echo-server",
		Version: "1.0.0",
	}, nil)

	// Create custom schema for input validation
	inputSchema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"input": {
				Type:        "string",
				Pattern:     "^[a-zA-Z0-9]+$",
				Description: "Alphanumeric string to echo back",
			},
		},
		Required: []string{"input"},
	}

	// Add echo tool to server using the new API
	mcp.AddTool(server, &mcp.Tool{
		Name: "echo",
		Description: "Echo back an alphanumeric string for deterministic testing. " +
			"Also echoes back any _meta field from the request for testing metadata propagation.",
		InputSchema: inputSchema,
	}, echoHandler)

	cs := &counterState{mode: backendMode, hangAfter: hangAfterN, crashAfter: crashAfterN}
	br := &barrier{n: barrierN, timeout: barrierTimeout}
	server.AddReceivingMiddleware(newFaultMiddleware(backendMode, cs, br))

	ctx := context.Background()

	switch transport {
	case "stdio":
		log.Println("Starting MCP server with stdio transport")
		stdioTransport := &mcp.StdioTransport{}
		if err := server.Run(ctx, stdioTransport); err != nil {
			log.Fatal("Failed to run server:", err)
		}

	case "sse":
		log.Printf("Starting MCP server with SSE transport on port %d", port)
		log.Printf("SSE endpoint: http://localhost:%d/sse", port)
		log.Printf("Messages will be handled automatically by the SSE handler")

		handler := mcp.NewSSEHandler(func(_ *http.Request) *mcp.Server {
			return server
		}, nil)

		// Mount the SSE handler at /sse - it will handle both GET (SSE stream) and POST (messages) requests
		http.Handle("/sse", authWrapper(handler))

		// Create server with timeouts to address G114 gosec issue
		srv := &http.Server{
			Addr:         ":" + strconv.Itoa(port),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())

	case "streamable-http":
		log.Printf("Starting MCP server with streamable HTTP transport on port %d", port)

		handler := mcp.NewStreamableHTTPHandler(func(_ *http.Request) *mcp.Server {
			return server
		}, &mcp.StreamableHTTPOptions{Stateless: stateless})

		http.Handle("/mcp", authWrapper(handler))

		// Create server with timeouts to address G114 gosec issue
		srv := &http.Server{
			Addr:         ":" + strconv.Itoa(port),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		log.Fatal(srv.ListenAndServe())

	default:
		fmt.Fprintf(os.Stderr, "Unknown transport type: %s\n", transport)
		fmt.Fprintf(os.Stderr, "Supported transports: stdio, sse, streamable-http\n")
		os.Exit(1)
	}
}

// parseConfig parses the command line flags and environment variables
// to set the transport and port for the MCP server
func parseConfig() {
	flag.StringVar(&transport, "transport", "stdio", "Transport type: stdio, sse, or streamable-http")
	flag.IntVar(&port, "port", 8080, "Port number for HTTP-based transports")
	flag.BoolVar(&stateless, "stateless", false, "Run the streamable-http transport in stateless mode (ignored by stdio and sse)")
	flag.Parse()

	// Use environment variables if provided, otherwise use flag values
	if t, ok := os.LookupEnv("MCP_TRANSPORT"); ok {
		transport = t
	}
	if p, ok := os.LookupEnv("PORT"); ok {
		if intValue, err := strconv.Atoi(p); err == nil {
			port = intValue
		}
	}
	if s, ok := os.LookupEnv("STATELESS"); ok {
		if boolValue, err := strconv.ParseBool(s); err == nil {
			stateless = boolValue
		}
	}

	authHeader = os.Getenv("AUTH_HEADER")
	authValue = os.Getenv("AUTH_VALUE")

	backendMode = os.Getenv("BACKEND_MODE")
	if backendMode == "" {
		backendMode = "echo"
	}
	barrierN = envIntOr("BARRIER_N", 2)
	hangAfterN = envIntOr("HANG_AFTER_N", 1)
	crashAfterN = envIntOr("CRASH_AFTER_N", 1)
	barrierTimeout = time.Duration(envIntOr("BARRIER_TIMEOUT_SECONDS", 10)) * time.Second

	if err := validateFaultConfig(backendMode, barrierN, hangAfterN, crashAfterN); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}

// validateFaultConfig checks that the fault-injection knob relevant to mode
// has a usable value. counterState.decide and barrier.join both treat a
// non-positive threshold as "never triggers"/"release immediately" rather
// than erroring, so a misconfigured value (e.g. a typo'd 0) would otherwise
// silently make the requested fault never fire.
func validateFaultConfig(mode string, barrierN, hangAfterN, crashAfterN int) error {
	switch mode {
	case modeBarrier:
		if barrierN < 1 {
			return fmt.Errorf("BARRIER_N must be >= 1 (got %d): barrier mode requires at least one request per window", barrierN)
		}
	case modeHang:
		if hangAfterN < 1 {
			return fmt.Errorf("HANG_AFTER_N must be >= 1 (got %d): hang mode requires a positive call count to trigger on", hangAfterN)
		}
	case modeCrash:
		if crashAfterN < 1 {
			return fmt.Errorf("CRASH_AFTER_N must be >= 1 (got %d): crash mode requires a positive call count to trigger on", crashAfterN)
		}
	}
	return nil
}
