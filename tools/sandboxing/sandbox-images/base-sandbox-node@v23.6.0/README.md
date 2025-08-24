# Node.js v23.6.0 Sandbox Server

A lightweight HTTP API for safely executing JavaScript and TypeScript code using Node.js v23.6.0's native TypeScript support.

## Features

- **Native TypeScript Support**: Leverages Node.js v23.6.0's built-in TypeScript execution
- **Secure Execution**: Runs in a containerized environment with resource limits
- **Simple HTTP API**: POST JavaScript or TypeScript code for execution
- **Timeout Protection**: 30-second execution timeout prevents runaway scripts
- **Health Checks**: Built-in health endpoint for monitoring

## API Endpoints

### POST /eval

Execute JavaScript or TypeScript code.

**Request Body:**
```json
{
  "code": "console.log('Hello, World!');",
  "language": "javascript"
}
```

**Response:**
```json
{
  "stdout": "Hello, World!",
  "stderr": "",
  "exitCode": 0
}
```

**Fields:**
- `code` (required): The JavaScript or TypeScript code to execute
- `language` (optional): "javascript" or "typescript" (defaults to "javascript")

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "node": "v23.6.0"
}
```

## Usage

### Start the service:
```bash
docker-compose up --build
```

### Test JavaScript execution:
```bash
curl -X POST http://localhost:8080/eval \
  -H "Content-Type: application/json" \
  -d '{
    "code": "console.log(\"Hello from JavaScript!\"); console.log(2 + 2);",
    "language": "javascript"
  }'
```

### Test TypeScript execution:
```bash
curl -X POST http://localhost:8080/eval \
  -H "Content-Type: application/json" \
  -d '{
    "code": "const message: string = \"Hello from TypeScript!\"; console.log(message); const sum: number = 2 + 2; console.log(`Sum: ${sum}`);",
    "language": "typescript"
  }'
```

### Test error handling:
```bash
curl -X POST http://localhost:8080/eval \
  -H "Content-Type: application/json" \
  -d '{
    "code": "throw new Error(\"Test error\");",
    "language": "javascript"
  }'
```

### Check health:
```bash
curl http://localhost:8080/health
```

## Security Features

- Runs as non-root user
- Read-only filesystem (except /tmp)
- Limited capabilities (CAP_DROP ALL, minimal CAP_ADD)
- Resource limits (CPU and memory)
- Execution timeout (30 seconds)
- No network privileges in container security options

## Development

### Build manually:
```bash
docker build -t sandbox-node:v23.6.0 .
```

### Run manually:
```bash
docker run -p 8080:8080 sandbox-node:v23.6.0
```

## Limitations

- No external dependencies (Layer 1a - raw code evaluation only)
- 30-second execution timeout
- Limited to built-in Node.js modules
- No file system persistence beyond /tmp
- No network access for executed code