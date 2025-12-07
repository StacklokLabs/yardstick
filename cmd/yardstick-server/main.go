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

var alphanumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
var transport string
var port int
var authHeader string
var authValue string

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

func echoHandler(_ context.Context, _ *mcp.CallToolRequest, params EchoRequest) (*mcp.CallToolResult, EchoResponse, error) {
	if !validateAlphanumeric(params.Input) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "input must be alphanumeric only"}},
			IsError: true,
		}, EchoResponse{}, nil
	}

	response := EchoResponse{
		Output: params.Input,
	}

	return nil, response, nil
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
		Name:        "echo",
		Description: "Echo back an alphanumeric string for deterministic testing",
		InputSchema: inputSchema,
	}, echoHandler)

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
		}, nil)

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

	authHeader = os.Getenv("AUTH_HEADER")
	authValue = os.Getenv("AUTH_VALUE")
}
