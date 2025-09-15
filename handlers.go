package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"

    "github.com/google/go-github/v57/github"
    "golang.org/x/oauth2"
    oauth2github "golang.org/x/oauth2/github"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
    html := `
    <h1>GitHub App</h1>
    <p>To install this app, go to: <a href="https://github.com/apps/devflow-agent">GitHub App Page</a></p>
    <a href="https://github.com/login/oauth/authorize?client_id=%s&scope=repo">Authorize as User</a>
    `
    fmt.Fprintf(w, html, os.Getenv("GITHUB_CLIENT_ID"))
}

func handleInstall(w http.ResponseWriter, r *http.Request) {
    installationID := r.URL.Query().Get("installation_id")
    if installationID == "" {
        http.Error(w, "No installation_id provided", http.StatusBadRequest)
        return
    }

    id, err := strconv.ParseInt(installationID, 10, 64)
    if err != nil {
        http.Error(w, "Invalid installation_id", http.StatusBadRequest)
        return
    }

    // Get installation token
    token, err := getInstallationToken(id)
    if err != nil {
        log.Printf("Error getting installation token: %v", err)
        http.Error(w, "Failed to get installation token", http.StatusInternalServerError)
        return
    }

    // Test the token by getting installation info
    client := github.NewTokenClient(context.Background(), token)
    installation, _, err := client.Apps.GetInstallation(context.Background(), id)
    if err != nil {
        log.Printf("Error getting installation: %v", err)
        http.Error(w, "Failed to get installation", http.StatusInternalServerError)
        return
    }

    fmt.Fprintf(w, "✅ App installed successfully!\nInstallation ID: %d\nAccount: %s\n", 
        installation.GetID(), installation.GetAccount().GetLogin())
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")
    if code == "" {
        http.Error(w, "No code provided", http.StatusBadRequest)
        return
    }

    // Exchange code for token
    conf := &oauth2.Config{
        ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
        ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
        Endpoint:     oauth2github.Endpoint,
    }

    token, err := conf.Exchange(context.Background(), code)
    if err != nil {
        log.Printf("Error exchanging code: %v", err)
        http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
        return
    }

    // Test the token by getting user info
    client := github.NewTokenClient(context.Background(), token.AccessToken)
    user, _, err := client.Users.Get(context.Background(), "")
    if err != nil {
        log.Printf("Error getting user: %v", err)
        http.Error(w, "Failed to get user", http.StatusInternalServerError)
        return
    }

    fmt.Fprintf(w, "✅ User authorized successfully!\nUser: %s\nToken: %s\n", 
        user.GetLogin(), token.AccessToken)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
    payload, err := github.ValidatePayload(r, []byte(os.Getenv("WEBHOOK_SECRET")))
    if err != nil {
        log.Printf("Error validating webhook: %v", err)
        // For now, continue without validation
    }

    event, err := github.ParseWebHook(github.WebHookType(r), payload)
    if err != nil {
        log.Printf("Error parsing webhook: %v", err)
        http.Error(w, "Error parsing webhook", http.StatusBadRequest)
        return
    }

    switch e := event.(type) {
    case *github.InstallationEvent:
        log.Printf("Installation event: %s for installation %d", 
            e.GetAction(), e.GetInstallation().GetID())
    case *github.IssuesEvent:
        log.Printf("Issue event: %s for issue #%d", 
            e.GetAction(), e.GetIssue().GetNumber())
    default:
        log.Printf("Unhandled event type: %T", event)
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}