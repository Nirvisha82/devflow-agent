package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Installations InstallationsConfig `yaml:"installations"`
	Issues        IssuesConfig        `yaml:"issues"`
	Labels        []LabelConfig       `yaml:"labels"`
	AI            AIConfig            `yaml:"ai"`
	Repository    RepositoryConfig    `yaml:"repository"`
	Files         FilesConfig         `yaml:"files"`
	PullRequests  PullRequestsConfig  `yaml:"pull_requests"`
	Debug         DebugConfig         `yaml:"debug"`
}

// InstallationsConfig contains installation-related configuration
type InstallationsConfig struct {
	InitBranch          string `yaml:"init_branch"`
	InitCommit          string `yaml:"init_commit"`
	KnowledgeBaseBranch string `yaml:"knowledge_base_branch"`
	KnowledgeBaseCommit string `yaml:"knowledge_base_commit"`
}

// IssuesConfig contains issue handling configuration
type IssuesConfig struct {
	RequiredLabels      []string `yaml:"required_labels"`
	BranchPrefix        string   `yaml:"branch_prefix"`
	BranchNameMaxLength int      `yaml:"branch_name_max_length"`
}

// LabelConfig represents a GitHub label configuration
type LabelConfig struct {
	Name        string `yaml:"name"`
	Color       string `yaml:"color"`
	Description string `yaml:"description"`
}

// AIConfig contains AI-related configuration
type AIConfig struct {
	Model                   string  `yaml:"model"`
	Temperature             float32 `yaml:"temperature"`
	TopK                    int32   `yaml:"top_k"`
	TopP                    float32 `yaml:"top_p"`
	MaxOutputTokens         int32   `yaml:"max_output_tokens"`
	RepoAnalysisTemperature float32 `yaml:"repo_analysis_temperature"`
}

// RepositoryConfig contains repository-related configuration
type RepositoryConfig struct {
	CloneDepth       int    `yaml:"clone_depth"`
	DefaultBranch    string `yaml:"default_branch"`
	DevflowDirectory string `yaml:"devflow_directory"`
	TempRepoPrefix   string `yaml:"temp_repo_prefix"`
	CleanupTempRepos bool   `yaml:"cleanup_temp_repos"`
}

// DebugConfig contains debug-related configuration
type DebugConfig struct {
	Enabled          bool `yaml:"enabled"`
	CreateDebugFiles bool `yaml:"create_debug_files"`
}

// PullRequestsConfig contains PR-related configuration
type PullRequestsConfig struct {
	Installation    PRTemplateConfig `yaml:"installation"`
	IssueResolution PRTemplateConfig `yaml:"issue_resolution"`
}

// PRTemplateConfig contains PR template configuration
type PRTemplateConfig struct {
	TitleFile string `yaml:"title_file"`
	BodyFile  string `yaml:"body_file"`
}

// FilesConfig contains file naming configuration
type FilesConfig struct {
	StructureFile      string `yaml:"structure_file"`
	AnalysisFile       string `yaml:"analysis_file"`
	AnalysisPromptFile string `yaml:"analysis_prompt_file"`
	MetadataFile       string `yaml:"metadata_file"`
	DependencyFile     string `yaml:"dependency_file"`
	ReadmeFile         string `yaml:"readme_file"`
	SummaryFile        string `yaml:"summary_file"`
}

var globalConfig *Config

// LoadConfig loads configuration from the specified file
func LoadConfig(configPath string) (*Config, error) {
	// If no path provided, use default
	if configPath == "" {
		configPath = "config/development.yaml"
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set global config
	globalConfig = &config

	return &config, nil
}

// GetConfig returns the global configuration instance
func GetConfig() *Config {
	if globalConfig == nil {
		// Try to load default config
		config, err := LoadConfig("")
		if err != nil {
			panic(fmt.Sprintf("Failed to load configuration: %v", err))
		}
		return config
	}
	return globalConfig
}

// GetDevflowPath returns the full path to a devflow file
func (c *Config) GetDevflowPath(repoPath, fileName string) string {
	return filepath.Join(repoPath, c.Repository.DevflowDirectory, fileName)
}

// GetDevflowDir returns the full path to the devflow directory
func (c *Config) GetDevflowDir(repoPath string) string {
	return filepath.Join(repoPath, c.Repository.DevflowDirectory)
}
