

## Prerequisites

- Go 1.24 or later
- [Task](https://taskfile.dev/) for running tasks

## Installation

1. Clone the repository:

   ```bash
   git clone https://github.com/StacklokLabs/yardstick.git
   cd yardstick
   ```

2. Install dependencies:

   ```bash
   task install
   ```

3. Build the server:

   ```bash
   task build
   ```

## Usage

### Command Line Options

```bash
./yardstick [options]
```

**Options:**
- `--transport`: Transport type (`stdio`, `sse`, or `streamable-http`) - default: `stdio`
- `--port`: Port number for HTTP-based transports - default: `8080`

### Examples

**Stdio Transport:**
```bash
yardstick --transport stdio
```

**SSE Transport:**
```bash
yardstick --transport sse --port 8080
```

**Streamable HTTP Transport:**
```bash
yardstick --transport streamable-http --port 8080
```

### Running with Docker

**Stdio Transport (default):**
```bash
docker run -it ghcr.io/stackloklabs/yardstick/server
```

**SSE Transport:**
```bash
docker run -p 8080:8080 -e MCP_TRANSPORT=sse -e PORT=8080 ghcr.io/stackloklabs/yardstick/server
```

**Streamable HTTP Transport:**
```bash
docker run -p 8080:8080 -e MCP_TRANSPORT=streamable-http -e PORT=8080 ghcr.io/stackloklabs/yardstick/server
```

## Tools

### `echo` Tool

A deterministic echo tool for basic testing and validation. This tool also supports metadata echoing for testing metadata propagation through MCP implementations.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "input": {
      "type": "string",
      "description": "Alphanumeric string to echo back",
      "pattern": "^[a-zA-Z0-9]+$"
    }
  },
  "required": ["input"]
}
```

**Output (StructuredContent):**
```json
{
  "output": "input_string"
}
```

**Metadata Support:**
The `echo` tool accepts and echoes back the optional `_meta` field from tool call requests. Any metadata provided in the request's `_meta` field will be returned in the response's `_meta` field, enabling validation that:
- MCP clients correctly pass metadata in tool calls
- Servers properly return metadata in responses
- Proxies and routers preserve metadata throughout the request/response lifecycle
- Metadata can be used for request tracking, debugging, and observability

When metadata is present, it is also logged to the server's output for debugging purposes.

**Example Request with Metadata:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": {
      "input": "test123"
    },
    "_meta": {
      "progressToken": "task123",
      "requestId": "req-456",
      "clientInfo": "test-client-v1"
    }
  }
}
```

**Example Response with Metadata:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "{\"output\":\"test123\"}"
      }
    ],
    "_meta": {
      "progressToken": "task123",
      "requestId": "req-456",
      "clientInfo": "test-client-v1"
    }
  }
}
```

## Metadata Field Support

The `echo` tool supports the optional `_meta` field as specified in the [MCP specification (2025-11-25)](https://modelcontextprotocol.io). The `_meta` field allows clients and servers to attach additional metadata to their interactions without exposing it to the LLM.

**Common use cases:**
- **Request tracking**: Using `progressToken` for progress notifications
- **Client context**: Passing client version, user information, or session data
- **Debugging**: Including trace IDs, debug levels, or diagnostic information
- **Testing**: Validating metadata propagation through complex MCP architectures

**Standard Fields:**
While the `_meta` field accepts any key-value pairs, the MCP specification defines some standard fields:
- `progressToken`: An opaque token for associating progress notifications with requests

## Development

### Running tests

```bash
task test
```

### Formatting code

```bash
task fmt
```

### Linting code

```bash
task lint
```

### Updating dependencies

```bash
task deps
```

## Performance Testing Use Case

This server is specifically designed for performance testing MCP implementations. The deterministic nature of the echo tool ensures that:

1. **No Response Caching**: Each request with a unique input produces a unique response
2. **Predictable Behavior**: Response time and content are consistent for the same input
3. **Load Testing**: Can handle thousands of concurrent requests with unique inputs
4. **Transport Comparison**: Allows testing performance across different transport types

Example performance test inputs:
- `test1`, `test2`, `test3`, ... for sequential testing
- `load001`, `load002`, `load003`, ... for load testing  
- `perf${timestamp}` for timestamp-based uniqueness

## Transport Details

### Stdio Transport
- Uses standard input/output for communication
- Ideal for subprocess-based MCP clients
- JSON-RPC messages via stdin/stdout

### SSE Transport  
- Server-Sent Events over HTTP
- Primary endpoint: `/sse` for establishing SSE connections (GET requests)
- Message handling: Same `/sse` endpoint with session ID query parameter for POST requests
- The SSE handler automatically creates session-specific endpoints for bidirectional communication
- Supports CORS for web clients
- Real-time streaming capabilities

**SSE Transport Flow:**
1. Client sends GET request to `/sse` to establish SSE connection
2. Server responds with SSE stream and sends an `endpoint` event with session-specific URL
3. Client sends messages via POST requests to the session endpoint (e.g., `/sse?sessionid=abc123`)
4. Server streams responses back via the SSE connection

### Streamable HTTP Transport
- HTTP POST requests to `/mcp` endpoint
- JSON-RPC over HTTP
- Supports CORS
- Request/response pattern

## Error Handling

The server validates input and returns appropriate errors for:
- Non-alphanumeric characters in input
- Malformed JSON requests
- Invalid tool parameters
- Transport-specific errors