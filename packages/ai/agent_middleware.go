package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/github"
)

// PythonAgentResult represents the result from the Python Strands agent
type PythonAgentResult struct {
	Completed    bool     `json:"completed"`
	Success      bool     `json:"success"`
	ChangesMade  []string `json:"changes_made"`
	Summary      string   `json:"summary"`
	ErrorMessage string   `json:"error_message"`
}

// callPythonStrandsAgent executes the Python Strands agent and returns the result
func CallPythonStrandsAgent(repoPath string, issue *github.Issue) (*PythonAgentResult, error) {
	// Prepare issue data
	issueData := map[string]interface{}{
		"title": issue.GetTitle(),
		"body":  issue.GetBody(),
		"labels": func() []string {
			var labels []string
			for _, label := range issue.Labels {
				if label.Name != nil {
					labels = append(labels, *label.Name)
				}
			}
			return labels
		}(),
	}

	issueJSON, err := json.Marshal(issueData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal issue: %w", err)
	}

	// Execute Python script using venv
	pythonScript := filepath.Join("", "capture_test_data.py")
	pythonBin := filepath.Join("", ".venv-devflow", "bin", "python")

	execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(execCtx, pythonBin, pythonScript, repoPath, string(issueJSON))
	output, err := cmd.CombinedOutput()

	slog.Info("Python agent output", "output", string(output))

	if err != nil {
		return nil, fmt.Errorf("python execution failed: %w\nOutput: %s", err, string(output))
	}

	// Parse JSON result
	outputStr := string(output)
	jsonStart := strings.Index(outputStr, "=== JSON RESULT ===")
	if jsonStart == -1 {
		return nil, fmt.Errorf("no JSON result found in output")
	}

	jsonStr := outputStr[jsonStart+len("=== JSON RESULT ==="):]
	jsonStr = strings.TrimSpace(jsonStr)

	result := &PythonAgentResult{}
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return result, nil
}
