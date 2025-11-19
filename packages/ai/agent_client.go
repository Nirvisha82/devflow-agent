package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath" // <-- added
	"time"

	"github.com/google/go-github/github"
)

// IssueData represents the issue information to send to the agent
type IssueData struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// ProcessIssueRequest represents the request to the agent server
type ProcessIssueRequest struct {
	RepoPath string    `json:"repo_path"`
	Issue    IssueData `json:"issue"`
	Mode     string    `json:"mode"`
}

// MarshalJSON ensures RepoPath is absolute before sending to the Python server.
func (p ProcessIssueRequest) MarshalJSON() ([]byte, error) {
	abs, err := filepath.Abs(p.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to make repo path absolute: %w", err)
	}
	// Reconstruct the JSON payload with the absolute path
	type payload struct {
		RepoPath string    `json:"repo_path"`
		Issue    IssueData `json:"issue"`
		Mode     string    `json:"mode"`
	}
	return json.Marshal(payload{
		RepoPath: abs,
		Issue:    p.Issue,
		Mode:     p.Mode,
	})
}

// PythonAgentResult represents the result from the Python Strands agent
type PythonAgentResult struct {
	Completed    bool     `json:"completed"`
	Success      bool     `json:"success"`
	ChangesMade  []string `json:"changes_made"` // Array of file paths
	Summary      string   `json:"summary"`
	PRBodyFile   string   `json:"pr_body_file"`
	ErrorMessage string   `json:"error_message"`
}

// AgentServerConfig holds the configuration for the agent server
type AgentServerConfig struct {
	BaseURL string
	Timeout time.Duration
}

// DefaultAgentServerConfig returns the default configuration
func DefaultAgentServerConfig() AgentServerConfig {
	return AgentServerConfig{
		BaseURL: "http://localhost:8094",
		Timeout: 5 * time.Minute,
	}
}

// CallPythonStrandsAgent calls the agent server via HTTP API
func CallPythonStrandsAgent(repoPath string, issue *github.Issue) (*PythonAgentResult, error) {
	config := DefaultAgentServerConfig()
	return CallPythonStrandsAgentWithConfig(repoPath, issue, config)
}

// CallPythonStrandsAgentWithConfig calls the agent server with custom configuration
func CallPythonStrandsAgentWithConfig(repoPath string, issue *github.Issue, config AgentServerConfig) (*PythonAgentResult, error) {
	// Prepare issue data
	labels := make([]string, 0)
	for _, label := range issue.Labels {
		if label.Name != nil {
			labels = append(labels, *label.Name)
		}
	}

	issueData := IssueData{
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
		Labels: labels,
	}

	// Prepare request
	request := ProcessIssueRequest{
		RepoPath: repoPath,
		Issue:    issueData,
		Mode:     "automate", // Default mode, server will auto-detect from labels
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	slog.Info("Calling Python agent server",
		"url", config.BaseURL,
		"repoPath", repoPath,
		"issueTitle", issue.GetTitle(),
		"labels", labels)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: config.Timeout,
	}

	// Make request to agent server
	resp, err := client.Post(
		config.BaseURL+"/api/process",
		"application/json",
		bytes.NewBuffer(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to call agent server: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	slog.Info("Agent server response received",
		"statusCode", resp.StatusCode,
		"contentLength", len(responseBody))

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent server returned error status %d: %s",
			resp.StatusCode, string(responseBody))
	}

	// Parse response
	result := &PythonAgentResult{}
	if err := json.Unmarshal(responseBody, result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w\nBody: %s",
			err, string(responseBody))
	}

	slog.Info("Agent execution completed",
		"success", result.Success,
		"filesChanged", len(result.ChangesMade),
		"hasPRBody", result.PRBodyFile != "")

	return result, nil
}

// HealthCheck checks if the agent server is running and healthy
func HealthCheck(baseURL string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned status %d: %s",
			resp.StatusCode, string(body))
	}

	slog.Info("Agent server health check passed", "url", baseURL)
	return nil
}
