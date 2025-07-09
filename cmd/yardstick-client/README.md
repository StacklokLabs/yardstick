# Yardstick MCP Client

A command-line client for interacting with Model Context Protocol (MCP) servers.

## Overview

The client connects to MCP servers using various transport mechanisms and allows you to:
- Get server information and connection status
- List available tools and resources
- Call tools with JSON arguments

## Usage

```bash
./client [flags]
```

## Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-transport` | string | `stdio` | Transport type: `stdio`, `sse`, or `streamable-http` |
| `-address` | string | `localhost` | Server address (for HTTP-based transports) |
| `-port` | int | `8080` | Server port (for HTTP-based transports) |
| `-command` | string | `""` | Command to run for stdio transport (required for stdio) |
| `-timeout` | duration | `30s` | Connection timeout |
| `-action` | string | `info` | Action to perform: `info`, `list-tools`, `list-resources`, `call-tool` |
| `-tool` | string | `""` | Tool name to call (required for `call-tool` action) |
| `-args` | string | `"{}"` | Tool arguments as JSON (for `call-tool` action) |

## Environment Variables

Configuration can also be set via environment variables:
- `TRANSPORT`: Override transport type
- `ADDRESS`: Override server address  
- `PORT`: Override server port
- `COMMAND`: Override command for stdio transport

## Transport Types

### stdio (default)
Uses standard input/output for communication with a subprocess:
```bash
./client -transport=stdio -command="./server" -action=info
```

### sse
Server-Sent Events over HTTP:
```bash
./client -transport=sse -address=localhost -port=8080 -action=list-tools
```

### streamable-http
HTTP POST requests for JSON-RPC communication:
```bash
./client -transport=streamable-http -address=localhost -port=8080 -action=call-tool -tool=echo -args='{"input":"test123"}'
```

## Actions

### info (default)
Get server information including session ID, connection status, and available tools/resources count:
```bash
./client -action=info
```

### list-tools
List all available tools from the server with their descriptions:
```bash
./client -action=list-tools
```

### list-resources
List all available resources from the server with their descriptions:
```bash
./client -action=list-resources
```

### call-tool
Call a specific tool with provided JSON arguments:
```bash
./client -action=call-tool -tool=echo -args='{"input":"hello world"}'
```

## Examples

### Basic server information with stdio transport
```bash
./client -transport=stdio -command="./my-mcp-server"
```

### List tools from HTTP server
```bash
./client -transport=sse -address=api.example.com -port=3000 -action=list-tools
```

### Call a tool with complex arguments
```bash
./client -action=call-tool -tool=process-data -args='{"input": {"data": [1,2,3], "format": "json"}}'
```

### Using environment variables
```bash
export TRANSPORT=sse
export ADDRESS=remote.example.com
export PORT=3000
./client -action=list-resources
```