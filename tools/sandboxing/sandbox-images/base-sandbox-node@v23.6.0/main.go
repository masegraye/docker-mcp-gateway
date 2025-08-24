package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type EvalRequest struct {
	Code     string `json:"code"`
	Language string `json:"language"` // "javascript" or "typescript"
}

type EvalResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

func main() {
	http.HandleFunc("/eval", handleEval)
	http.HandleFunc("/health", handleHealth)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Node.js sandbox server on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"node":   getNodeVersion(),
	})
}

func handleEval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EvalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	// Default to javascript if language not specified
	if req.Language == "" {
		req.Language = "javascript"
	}

	// Validate language
	if req.Language != "javascript" && req.Language != "typescript" {
		http.Error(w, "Language must be 'javascript' or 'typescript'", http.StatusBadRequest)
		return
	}

	response := evalCode(req.Code, req.Language)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func evalCode(code, language string) EvalResponse {
	// Create a temporary file with appropriate extension
	extension := ".js"
	if language == "typescript" {
		extension = ".ts"
	}

	tmpDir := "/tmp"
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("script_%d%s", time.Now().UnixNano(), extension))

	// Write code to temporary file
	if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
		return EvalResponse{
			Error:    fmt.Sprintf("Failed to write temporary file: %v", err),
			ExitCode: 1,
		}
	}
	defer os.Remove(tmpFile)

	// Execute with Node.js with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", tmpFile)
	
	// Capture both stdout and stderr
	stdout, err := cmd.Output()
	var stderr string
	var exitCode int

	if err != nil {
		// Check if it's an exit error to get stderr and exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr = string(exitError.Stderr)
			exitCode = exitError.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			return EvalResponse{
				Error:    "Execution timeout (30s)",
				ExitCode: 124, // Standard timeout exit code
			}
		} else {
			return EvalResponse{
				Error:    fmt.Sprintf("Execution failed: %v", err),
				ExitCode: 1,
			}
		}
	}

	return EvalResponse{
		Stdout:   strings.TrimSpace(string(stdout)),
		Stderr:   strings.TrimSpace(stderr),
		ExitCode: exitCode,
	}
}

func getNodeVersion() string {
	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}