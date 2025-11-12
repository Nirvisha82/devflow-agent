// packages/repository/snapshot.go
package repository

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swinton/go-probot/probot"
)

type Change struct {
	Status string // "A","M","D","R"
	Old    string // for "R"
	New    string // for A/M/D or new path for R
}

type snapshotMeta struct {
	LastSyncedSHA string   `json:"last_synced_sha"`
	ChangedFiles  []string `json:"changed_files"`
	CreatedAt     string   `json:"created_at"`
}

// ---------- tiny git helpers (local to this file) ----------
func git(repoPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoPath
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v failed: %v: %s", args, err, errb.String())
	}
	return out.String(), nil
}

func temporarilyUnignoreDevflow(repoPath string) (restore func(), err error) {
	excludePath := filepath.Join(repoPath, ".git", "info", "exclude")
	data, _ := os.ReadFile(excludePath)
	lines := strings.Split(string(data), "\n")

	// Remove .devflow entries
	filtered := []string{}
	for _, ln := range lines {
		if strings.Contains(ln, ".devflow") {
			continue
		}
		filtered = append(filtered, ln)
	}

	// Write filtered back
	if err := os.WriteFile(excludePath, []byte(strings.Join(filtered, "\n")), 0644); err != nil {
		return nil, err
	}

	// Restore after commit
	return func() {
		_ = appendUniqueLines(excludePath, []string{"/.devflow/", ".devflow/", "/.devflow/*"})
	}, nil
}

// ---------- lock (best-effort) ----------
func acquireWriterLock(repoPath string) (func(), error) {
	lockDir := filepath.Join(repoPath, ".devflow_locks")
	if err := os.MkdirAll(lockDir, 0o755); err != nil {
		return nil, err
	}
	lockFile := filepath.Join(lockDir, "snapshot.write.lock")
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("devflow writer lock busy: %w", err)
	}
	_ = f.Close()
	return func() { _ = os.Remove(lockFile) }, nil
}

// ---------- pointer & meta ----------
func pointerPath(repoPath string) string {
	return filepath.Join(repoPath, ".devflow", "devflow-commit.txt")
}

func readPointerSHA(repoPath string) (string, error) {
	b, err := os.ReadFile(pointerPath(repoPath))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func writePointerSHA(repoPath, sha string) error {
	if err := os.MkdirAll(filepath.Join(repoPath, ".devflow"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pointerPath(repoPath), []byte(sha+"\n"), 0o644)
}

func writeSnapshotMeta(repoPath, headSHA string, changes []Change) error {
	seen := map[string]bool{}
	meta := snapshotMeta{
		LastSyncedSHA: headSHA,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	for _, c := range changes {
		switch c.Status {
		case "A", "M", "D":
			if c.New != "" && !seen[c.New] {
				meta.ChangedFiles = append(meta.ChangedFiles, c.New)
				seen[c.New] = true
			}
		case "R":
			if c.Old != "" && !seen[c.Old] {
				meta.ChangedFiles = append(meta.ChangedFiles, c.Old)
				seen[c.Old] = true
			}
			if c.New != "" && !seen[c.New] {
				meta.ChangedFiles = append(meta.ChangedFiles, c.New)
				seen[c.New] = true
			}
		}
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.MkdirAll(filepath.Join(repoPath, ".devflow"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(repoPath, ".devflow", "snapshot-meta.json"), b, 0o644)
}

// ---------- origin/main helpers ----------
func GetOriginMainSHA(repoPath string) (string, error) {
	if _, err := git(repoPath, "fetch", "origin", "main"); err != nil {
		return "", err
	}
	out, err := git(repoPath, "rev-parse", "origin/main")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func DiffNameStatus(repoPath, base, head string) ([]Change, error) {
	if base == "" {
		out, err := git(repoPath, "ls-tree", "-r", "--name-only", head)
		if err != nil {
			return nil, err
		}
		var cs []Change
		for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			cs = append(cs, Change{Status: "A", New: ln})
		}
		return cs, nil
	}
	out, err := git(repoPath, "diff", "--name-status", base, head)
	if err != nil {
		return nil, err
	}
	var changes []Change
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 3)
		switch parts[0] {
		case "A", "M", "D":
			if len(parts) >= 2 {
				changes = append(changes, Change{Status: parts[0], New: parts[1]})
			}
		default:
			// handle rename (R/ R100/ Rnnn)
			if strings.HasPrefix(parts[0], "R") && len(parts) == 3 {
				changes = append(changes, Change{Status: "R", Old: parts[1], New: parts[2]})
			} else if len(parts) >= 2 {
				changes = append(changes, Change{Status: "M", New: parts[len(parts)-1]})
			}
		}
	}
	return changes, nil
}

// ---------- incremental builders (reuse your existing logic) ----------
func BuildRepoAnalysisIncremental(repoPath string, changes []Change) error {
	// TODO: open .devflow/repo-analysis.md, replace per-file sections for A/M,
	// remove sections for D, rename headers for R. Use your existing analyzers.
	return nil
}

func BuildDepGraphIncremental(repoPath string, changes []Change) error {
	// TODO: load .devflow/dependency-graph.json, re-parse only changed files to
	// update imports; adjust reverse edges; delete/rename nodes on D/R.
	return nil
}

func BuildEmbeddingsIncremental(repoPath string, changes []Change) error {
	// OPTIONAL: only if you keep embeddings/index in .devflow/vector_index/*
	return nil
}

// ---------- commit/publish ----------
func CommitDevflowSync(ctx *probot.Context, repoName, repoPath, headSHA string) error {
	branch := "main"

	// 1) Ensure we’re on a branch that tracks origin/main
	if _, err := git(repoPath, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("fetch origin/%s: %w", branch, err)
	}
	if _, err := git(repoPath, "checkout", "-B", "_devflow_work", "origin/"+branch); err != nil {
		return fmt.Errorf("checkout work branch: %w", err)
	}

	// 2) Configure bot identity
	_, _ = git(repoPath, "config", "user.email", "devflow-bot@local")
	_, _ = git(repoPath, "config", "user.name", "DevFlow Bot")

	// 3) Force-add only .devflow
	if _, err := git(repoPath, "add", "-f", ".devflow"); err != nil {
		return fmt.Errorf("git add .devflow: %w", err)
	}

	// 4) Commit (ignore “nothing to commit” quietly)
	msg := fmt.Sprintf("chore(devflow): sync knowledge base for %.7s", headSHA)
	if _, err := git(repoPath, "commit", "-m", msg); err != nil {
		slog.Info("No .devflow changes to commit (direct mode)")
		return nil
	}

	// 5) Rebase fast-forward on latest origin/main
	if _, err := git(repoPath, "fetch", "origin", branch); err != nil {
		return fmt.Errorf("refetch origin/%s: %w", branch, err)
	}
	if _, err := git(repoPath, "rebase", "origin/"+branch); err != nil {
		_, _ = git(repoPath, "rebase", "--abort")
		return fmt.Errorf("rebase on origin/%s failed: %w", branch, err)
	}

	// 6) Push directly to main
	if _, err := git(repoPath, "push", "origin", "_devflow_work:"+branch); err != nil {
		return fmt.Errorf("push to %s failed: %w", branch, err)
	}

	slog.Info("Directly updated main with .devflow changes", "sha", headSHA)
	return nil
}

// ---------- orchestrator ----------
func RunIncrementalDevflowSync(ctx *probot.Context, repoName, repoPath, headSHA string) error {
	release, err := acquireWriterLock(repoPath)
	if err != nil {
		return err
	}
	defer release()

	last := ""
	if sha, err := readPointerSHA(repoPath); err == nil {
		last = sha
	}

	if _, err := git(repoPath, "fetch", "origin", "main"); err != nil {
		return fmt.Errorf("git fetch origin main: %w", err)
	}
	if err := ensureCommitAvailable(repoPath, headSHA); err != nil {
		return fmt.Errorf("head %s not available: %w", headSHA, err)
	}
	if last != "" {
		if err := ensureCommitAvailable(repoPath, last); err != nil {
			slog.Warn("Base commit missing; falling back to full rebuild", "base", last, "err", err)
			last = ""
		}
	}

	changes, err := DiffNameStatus(repoPath, last, headSHA)
	if err != nil {
		slog.Warn("Diff failed; falling back to full rebuild", "base", last, "head", headSHA, "err", err)
		last = ""
		changes, _ = DiffNameStatus(repoPath, "", headSHA)
	}
	slog.Info("Devflow Sync: diff", "base", last, "head", headSHA, "changes", len(changes))

	if err := BuildRepoAnalysisIncremental(repoPath, changes); err != nil {
		return err
	}
	if err := BuildDepGraphIncremental(repoPath, changes); err != nil {
		return err
	}
	if err := BuildEmbeddingsIncremental(repoPath, changes); err != nil {
		return err
	}

	if err := writePointerSHA(repoPath, headSHA); err != nil {
		return err
	}
	if err := writeSnapshotMeta(repoPath, headSHA, changes); err != nil {
		return err
	}

	if err := CommitDevflowSync(ctx, repoName, repoPath, headSHA); err != nil {
		return err
	}

	slog.Info("Devflow Sync: published", "sha", headSHA)
	return nil
}
