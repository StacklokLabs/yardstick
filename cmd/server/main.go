package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonschema"
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

func validateAlphanumeric(input string) bool {
	return alphanumericRegex.MatchString(input)
}

func echoHandler(_ context.Context, _ *mcp.ServerSession, params *mcp.CallToolParamsFor[EchoRequest]) (
	*mcp.CallToolResultFor[EchoResponse], error,
) {
	if !validateAlphanumeric(params.Arguments.Input) {
		return nil, fmt.Errorf("input must be alphanumeric only")
	}

	response := EchoResponse{
		Output: params.Arguments.Input,
	}

	return &mcp.CallToolResultFor[EchoResponse]{
		Content: []mcp.Content{
			&mcp.TextContent{Text: response.Output},
		},
	}, nil
}

func main() {
	// Parse command line flags
	parseConfig()

	// Create MCP server
	server := mcp.NewServer("echo-server", "1.0.0", nil)

	// Add echo tool to server
	echoTool := mcp.NewServerTool("echo", "Echo back an alphanumeric string for deterministic testing", echoHandler,
		mcp.Input(
			mcp.Property("input",
				mcp.Description("Alphanumeric string to echo back"),
				mcp.Schema(&jsonschema.Schema{
					Type:    "string",
					Pattern: "^[a-zA-Z0-9]+$",
				}),
			),
		),
	)

	server.AddTools(echoTool)

	ctx := context.Background()

	switch transport {
	case "stdio":
		log.Println("Starting MCP server with stdio transport")
		transport := mcp.NewStdioTransport()
		if err := server.Run(ctx, transport); err != nil {
			log.Fatal("Failed to run server:", err)
		}

	case "sse":
		log.Printf("Starting MCP server with SSE transport on port %d", port)
		log.Printf("SSE endpoint: http://localhost:%d/sse", port)
		log.Printf("Messages will be handled automatically by the SSE handler")

		handler := mcp.NewSSEHandler(func(_ *http.Request) *mcp.Server {
			return server
		})

		// Mount the SSE handler at /sse - it will handle both GET (SSE stream) and POST (messages) requests
		http.Handle("/sse", handler)

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

		http.Handle("/mcp", handler)

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
	if t, ok := os.LookupEnv("TRANSPORT"); ok {
		transport = t
	}
	if p, ok := os.LookupEnv("PORT"); ok {
		if intValue, err := strconv.Atoi(p); err == nil {
			port = intValue
		}
	}
}
