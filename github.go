package main

import (
    "context"
    "crypto/rsa"
    "strconv"
    "strings"
    "time"
	"os"

    "github.com/golang-jwt/jwt/v5"
    "github.com/google/go-github/v57/github"
)

// Parse private key from environment variable
func getPrivateKey() (*rsa.PrivateKey, error) {
    keyData := os.Getenv("GITHUB_APP_PRIVATE_KEY")
    keyData = strings.ReplaceAll(keyData, "\\n", "\n")
    
    return jwt.ParseRSAPrivateKeyFromPEM([]byte(keyData))
}

// Create JWT for GitHub App authentication
func createAppJWT() (string, error) {
    privateKey, err := getPrivateKey()
    if err != nil {
        return "", err
    }

    appID := os.Getenv("GITHUB_APP_ID")
    appIDInt, err := strconv.Atoi(appID)
    if err != nil {
        return "", err
    }

    token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
        "iat": time.Now().Unix() - 60,
        "exp": time.Now().Add(10 * time.Minute).Unix(),
        "iss": appIDInt,
    })

    return token.SignedString(privateKey)
}

// Get installation token to access repositories
func getInstallationToken(installationID int64) (string, error) {
    jwtToken, err := createAppJWT()
    if err != nil {
        return "", err
    }

    client := github.NewTokenClient(context.Background(), jwtToken)
    
    token, _, err := client.Apps.CreateInstallationToken(
        context.Background(), 
        installationID, 
        &github.InstallationTokenOptions{},
    )
    if err != nil {
        return "", err
    }

    return token.GetToken(), nil
}