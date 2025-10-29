package handlers

import (
	"log/slog"
	"strings"

	"devflow-agent/packages/config"
	repoActions "devflow-agent/packages/repository"

	"github.com/google/go-github/github"
	"github.com/swinton/go-probot/probot"
)

func HandleInstallations(ctx *probot.Context) error {
	event := ctx.Payload.(*github.InstallationRepositoriesEvent)
	action := event.GetAction()

	slog.Info("Installation Action:", "action", action)

	switch action {
	case "added":
		return handleRepositoriesAdded(ctx, event.RepositoriesAdded)
	case "removed":
		return handleRepositoriesRemoved(ctx, event.RepositoriesRemoved)
	}

	return nil
}

func handleRepositoriesAdded(ctx *probot.Context, repos []*github.Repository) error {
	for _, repo := range repos {
		fullName := repo.GetFullName()

		// Parse owner from full name
		parts := strings.Split(fullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository full name", "fullName", fullName)
			continue
		}

		owner := parts[0]
		name := parts[1]

		slog.Info("Repository details:",
			"fullName", fullName,
			"owner", owner,
			"name", name)

		// Step 1: Add custom labels to newly installed repositories
		if err := repoActions.AddCustomLabels(ctx, owner, name); err != nil {
			slog.Error("Failed to add labels", "repo", repo.GetFullName(), "error", err)
			continue
		}

		// Step 2: Initialize Devflow knowledge base for the repository
		if err := initializeDevflowKnowledgeBase(ctx, fullName); err != nil {
			slog.Error("Failed to initialize Devflow knowledge base", "repo", fullName, "error", err)
			continue
		}
	}
	return nil
}

func handleRepositoriesRemoved(ctx *probot.Context, repos []*github.Repository) error {
	for _, repo := range repos {
		fullName := repo.GetFullName()

		// Parse owner from full name
		parts := strings.Split(fullName, "/")
		if len(parts) != 2 {
			slog.Error("Invalid repository full name", "fullName", fullName)
			continue
		}

		owner := parts[0]
		name := parts[1]

		slog.Info("Repository removed",
			"fullName", fullName,
			"owner", owner,
			"name", name)

		// Labels can't be cleaned up since access to the repo is removed.
		// if err := repoActions.RemoveCustomLabels(ctx, owner, name); err != nil {
		// 	slog.Error("Failed to cleanup repository", "repo", fullName, "error", err)
		// 	continue
		// }
	}
	return nil
}

// initializeDevflowKnowledgeBase creates the complete Devflow knowledge base for a repository
func initializeDevflowKnowledgeBase(ctx *probot.Context, repoName string) error {
	slog.Info("Initializing Devflow knowledge base", "repo", repoName)

	// Clone repository temporarily
	repoPath, repoURL, err := repoActions.CloneRepository(repoName)
	if err != nil {
		slog.Error("Failed to clone repository for knowledge base initialization", "error", err)
		return err
	}
	// defer func() {
	// 	if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
	// 		slog.Error("Failed to cleanup repository", "repoPath", repoPath, "error", cleanupErr)
	// 	}
	// }()

	// Create .devflow directory
	cfg := config.GetConfig()
	devflowDir := cfg.GetDevflowDir(repoPath)
	if err := repoActions.CreateDirectory(devflowDir); err != nil {
		slog.Error("Failed to create .devflow directory", "error", err)
		return err
	}

	// Step 1: Generate repo-structure.md using RepoAnalyzer (flattened structure)
	structureFile := cfg.GetDevflowPath(repoPath, cfg.Files.StructureFile)
	if err := repoActions.AnalyzeRepo(ctx, structureFile, repoPath, repoURL); err != nil {
		slog.Error("Failed to generate repo structure", "error", err)
		return err
	}

	// Step 2: Save debug files (only if debug mode is enabled)
	var metadataFile, promptFile string
	if cfg.Debug.CreateDebugFiles {
		// Save file metadata as JSON
		metadataFile = cfg.GetDevflowPath(repoPath, cfg.Files.MetadataFile)
		if err := repoActions.SaveFileMetadata(repoPath, metadataFile); err != nil {
			slog.Error("Failed to save file metadata", "error", err)
			return err
		}

		// Save analysis prompt (using repo structure content)
		promptFile = cfg.GetDevflowPath(repoPath, cfg.Files.AnalysisPromptFile)
		if err := repoActions.SaveAnalysisPrompt(repoPath, repoURL, structureFile, promptFile); err != nil {
			slog.Error("Failed to save analysis prompt", "error", err)
			return err
		}
		slog.Info("Debug files created", "metadata", metadataFile, "prompt", promptFile)
	}

	// Step 3: Generate LLM analysis
	analysisFile := cfg.GetDevflowPath(repoPath, cfg.Files.AnalysisFile)
	if err := repoActions.GenerateRepoAnalysisWithLLM(repoPath, repoURL, structureFile, analysisFile); err != nil {
		slog.Error("Failed to generate LLM analysis", "error", err)
		return err
	}

	// Step 4: Build dependency graph
	dependencyFile := cfg.GetDevflowPath(repoPath, cfg.Files.DependencyFile)
	if err := repoActions.GenerateDependencyGraph(repoPath, dependencyFile); err != nil {
		slog.Error("Failed to generate dependency graph", "error", err)
		return err
	}

	// Step 5: Create .devflow/README.md
	readmeFile := cfg.GetDevflowPath(repoPath, cfg.Files.ReadmeFile)
	if err := repoActions.CreateDevflowReadme(readmeFile, repoName); err != nil {
		slog.Error("Failed to create Devflow README", "error", err)
		return err
	}

	// Step 6: Commit all files to the repository in a single commit
	branchName := cfg.Installations.KnowledgeBaseBranch
	if err := repoActions.CreateBranch(ctx, repoName, branchName); err != nil {
		slog.Error("Failed to create knowledge base branch", "error", err)
		return err
	}

	// Prepare files to commit (core files always, debug files conditionally)
	devflowFiles := []string{
		structureFile,
		analysisFile,
		dependencyFile,
		readmeFile,
	}

	// Add debug files if they were created
	if cfg.Debug.CreateDebugFiles {
		devflowFiles = append(devflowFiles, metadataFile, promptFile)
	}

	// Commit all files in a single commit
	if err := repoActions.CommitMultipleFiles(ctx, repoName, branchName, cfg.Installations.KnowledgeBaseCommit, devflowFiles, true, ""); err != nil {
		slog.Error("Failed to commit Devflow files", "error", err)
		return err
	}

	// Create pull request
	pr, err := repoActions.CreateInstallationPR(ctx, repoName, branchName)
	if err != nil {
		slog.Error("Failed to create pull request", "error", err)
		return err
	}

	// Cleanup temporary repository (if enabled)
	if cfg.Repository.CleanupTempRepos {
		if cleanupErr := repoActions.CleanupRepo(repoPath); cleanupErr != nil {
			slog.Error("Failed to cleanup temporary repository", "repoPath", repoPath, "error", cleanupErr)
		} else {
			slog.Info("Temporary repository cleaned up", "repoPath", repoPath)
		}
	} else {
		slog.Info("Temporary repository preserved for debugging", "repoPath", repoPath)
	}

	slog.Info("Devflow knowledge base initialized successfully",
		"repo", repoName,
		"branch", branchName,
		"prNumber", pr.GetNumber(),
		"prURL", pr.GetHTMLURL())
	return nil
}
