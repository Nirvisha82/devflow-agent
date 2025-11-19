package main

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"devflow-agent/packages/config"
	"devflow-agent/packages/handlers"

	"github.com/joho/godotenv"
	"github.com/swinton/go-probot/probot"
)

func main() {
	// Configure logging to reduce verbosity
	baseHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	filteredHandler := &FilteredHandler{handler: baseHandler}
	slog.SetDefault(slog.New(filteredHandler))

	// Load .env file
	if err := godotenv.Load(); err != nil {
		slog.Error("No .env file found")
	}

	// Load configuration
	_, err := config.LoadConfig("")
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}
	slog.Info("Configuration loaded successfully")

	// Load private key
	loadPrivateKey()

	// Log app ID
	appID := os.Getenv("GITHUB_APP_ID")
	slog.Info("App ID: ", "appID", appID)

	// Register event handlers
	probot.HandleEvent("issues", handlers.HandleIssues)
	probot.HandleEvent("installation_repositories", handlers.HandleInstallations)

	probot.HandleEvent("pull_request", handlers.HandlePullRequest)

	// Start the bot
	probot.Start()
}

func loadPrivateKey() {
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH")
	if keyPath != "" {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			slog.Error("Failed to read private key", "error", err)
		} else {
			os.Setenv("GITHUB_APP_PRIVATE_KEY", string(keyData))
			slog.Info("Private key loaded from", "keyPath", keyPath)
			slog.Info("Private key starts with", "keyData", string(keyData)[:50])
		}
	}
}

type FilteredHandler struct {
	handler slog.Handler
}

func (h *FilteredHandler) Handle(ctx context.Context, r slog.Record) error {
	// Only filter out headers
	if strings.Contains(r.Message, "Headers:") {
		return nil
	}
	return h.handler.Handle(ctx, r)
}

func (h *FilteredHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *FilteredHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &FilteredHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *FilteredHandler) WithGroup(name string) slog.Handler {
	return &FilteredHandler{handler: h.handler.WithGroup(name)}
}
