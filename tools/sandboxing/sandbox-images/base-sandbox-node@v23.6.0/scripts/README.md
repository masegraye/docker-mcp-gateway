# Base Sandbox Testing Scripts

This directory contains helper scripts for testing the base Node.js sandbox service.

## Quick Start

1. Start the sandbox service:
   ```bash
   docker compose up -d
   ```

2. Run test scripts:
   ```bash
   ./scripts/eval-in-sandbox http://localhost:8081/eval ./scripts/examples/hello.js
   ./scripts/eval-in-sandbox http://localhost:8081/eval ./scripts/examples/typescript-demo.ts
   ```

## Scripts

### `eval-in-sandbox`
Helper script that simplifies API testing by:
- Automatically detecting language from file extension (`.js` → javascript, `.ts` → typescript)
- Creating the proper JSON payload format
- Formatting the response with success/error indicators
- Displaying stdout, stderr, and exit codes

**Usage:**
```bash
./eval-in-sandbox <url> <script-path>
```

**Examples:**
```bash
./eval-in-sandbox http://localhost:8081/eval examples/hello.js
./eval-in-sandbox http://localhost:8081/eval examples/typescript-demo.ts
```

## Test Examples

### `examples/hello.js`
Basic JavaScript example demonstrating:
- Console output
- Array operations
- Date handling

### `examples/typescript-demo.ts` 
TypeScript example showcasing:
- TypeScript interfaces
- Type annotations
- Functions and loops
- Native TypeScript execution (no compilation step needed)

### `examples/error-test.js`
Error handling demonstration:
- Caught exceptions
- Uncaught async errors
- Non-zero exit codes

### `examples/timeout-test.js`
Timeout testing:
- Long-running script that exceeds the 30-second timeout
- Demonstrates the sandbox's safety mechanisms

## API Response Format

The sandbox API returns JSON with the following fields:
- `stdout`: String output from console.log, etc.
- `stderr`: Error output and stack traces
- `exitCode`: Process exit code (0 = success, non-zero = error)
- `error`: Error message for execution failures (optional)

## Requirements

The helper script requires:
- `curl` - for HTTP requests
- `jq` - for JSON processing and formatting