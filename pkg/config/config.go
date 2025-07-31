package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/validation"
)

const (
	// DefaultLogLevel is the default logging level
	DefaultLogLevel = "INFO"

	// DefaultOutputFormat is the default output format
	DefaultOutputFormat = "text"

	// DefaultTargetBranch is the default target branch for merge requests
	DefaultTargetBranch = "main"

	// EnvPrefix is the prefix for all environment variables
	EnvPrefix = "IMG_UPGR_"
)

// Environment variable names
const (
	EnvScanDir       = EnvPrefix + "SCANDIR"
	EnvLogLevel      = EnvPrefix + "LOG_LEVEL"
	EnvGitLabUser    = EnvPrefix + "GL_USER"
	EnvGitLabToken   = EnvPrefix + "GL_TOKEN"
	EnvGitLabRepo    = EnvPrefix + "GL_REPO"
	EnvGitLabProject = EnvPrefix + "GL_PROJECT_ID"
	EnvGitLabEmail   = EnvPrefix + "GL_EMAIL"
	EnvOutputFormat  = EnvPrefix + "OUTPUT_FORMAT"
)

// ValidLogLevels contains the list of valid log levels
var ValidLogLevels = []string{"DEBUG", "INFO", "WARN", "WARNING", "ERROR", "FATAL"}

// ValidOutputFormats contains the list of valid output formats
var ValidOutputFormats = []string{"text", "json", "yaml"}

// GitLabClient is an interface for GitLab API client to avoid import cycle
type GitLabClient interface {
	CreateMergeRequest(sourceBranch, targetBranch, title, description string) (interface{}, error)
}

// Config holds all configuration for the application
type Config struct {
	// General settings
	Verbose  bool
	Quiet    bool
	LogLevel string

	// Check command settings
	OutputFormat string
	DryRun       bool

	// Scan command settings
	ScanDir      string
	CreateMR     bool
	TargetBranch string
	TempDir      string
	ClonedRepo   bool

	// GitLab settings
	GitLabUser      string
	GitLabToken     string
	GitLabRepo      string
	GitLabProjectID string
	GitLabEmail     string

	// GitLab client (set after initialization)
	GitLabClient interface{}
}

// New creates a new Config with default values
func New() *Config {
	return &Config{
		Verbose:      false,
		Quiet:        false,
		LogLevel:     DefaultLogLevel,
		OutputFormat: DefaultOutputFormat,
		DryRun:       false,
		ScanDir:      "",
		CreateMR:     false,
		TargetBranch: DefaultTargetBranch,
		TempDir:      "",
		ClonedRepo:   false,
	}
}

// LoadFromEnv loads configuration from environment variables
func (c *Config) LoadFromEnv() {
	// Scan settings
	c.ScanDir = getEnvOrDefault(EnvScanDir, c.ScanDir)

	// GitLab settings
	c.GitLabUser = getEnvOrDefault(EnvGitLabUser, c.GitLabUser)
	c.GitLabToken = getEnvOrDefault(EnvGitLabToken, c.GitLabToken)
	c.GitLabRepo = getEnvOrDefault(EnvGitLabRepo, c.GitLabRepo)
	c.GitLabProjectID = getEnvOrDefault(EnvGitLabProject, c.GitLabProjectID)
	c.GitLabEmail = getEnvOrDefault(EnvGitLabEmail, c.GitLabEmail)

	// Logging settings
	c.LogLevel = getEnvOrDefault(EnvLogLevel, c.LogLevel)

	// Output format
	c.OutputFormat = getEnvOrDefault(EnvOutputFormat, c.OutputFormat)

	// Configure logger based on settings
	c.ConfigureLogger()
}

// getEnvOrDefault returns the environment variable value or the default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Validate performs comprehensive validation of all configuration settings
func (c *Config) Validate() error {
	// Create a validation errors collection
	validationErrors := &validation.ValidationErrors{}

	// Validate log level
	if err := validation.ValidateLogLevel(c.LogLevel, ValidLogLevels); err != nil {
		validationErrors.Add("LogLevel", err.Error())
	}

	// Validate output format
	if !validation.IsValidOutputFormat(c.OutputFormat, ValidOutputFormats) {
		validationErrors.Add("OutputFormat", fmt.Sprintf("invalid output format: %s (valid formats: %s)",
			c.OutputFormat, strings.Join(ValidOutputFormats, ", ")))
	}

	// Validate scan directory if set
	if c.ScanDir != "" {
		scanPath := c.GetScanPath()
		if err := validation.ValidateFileOrDir(scanPath); err != nil {
			validationErrors.Add("ScanDir", err.Error())
		}
	}

	// Validate target branch if creating merge requests
	if c.CreateMR && c.TargetBranch == "" {
		validationErrors.Add("TargetBranch", "target branch must be specified when creating merge requests")
	}

	// Check for validation errors
	if validationErrors.HasErrors() {
		return validationErrors
	}

	logger.Debug("Configuration validated successfully")
	return nil
}

// ValidateGitLab validates GitLab configuration
func (c *Config) ValidateGitLab() error {
	// Create a validation errors collection
	validationErrors := &validation.ValidationErrors{}

	// Check required GitLab variables
	requiredVars := map[string]string{
		EnvGitLabUser:  c.GitLabUser,
		EnvGitLabToken: c.GitLabToken,
		EnvGitLabRepo:  c.GitLabRepo,
		EnvGitLabEmail: c.GitLabEmail,
	}

	// Only validate these if we're creating merge requests
	if c.CreateMR {
		missingVars := validation.GetMissingVars(requiredVars)
		if len(missingVars) > 0 {
			validationErrors.Add("GitLab", fmt.Sprintf("missing required environment variables: %s",
				strings.Join(missingVars, ", ")))
		}

		// Validate GitLab repo URL
		if err := validation.ValidateURL(c.GitLabRepo); err != nil {
			validationErrors.Add("GitLabRepo", err.Error())
		}
	}

	// Check for validation errors
	if validationErrors.HasErrors() {
		return validationErrors
	}

	logger.Debug("GitLab configuration validated successfully")
	return nil
}

// GetScanPath returns the full path to the scan directory
func (c *Config) GetScanPath() string {
	if c.ScanDir == "" {
		return ""
	}

	if c.TempDir != "" && !filepath.IsAbs(c.ScanDir) {
		return filepath.Join(c.TempDir, c.ScanDir)
	}

	return c.ScanDir
}

// ComposeFilePatterns contains patterns for Docker Compose files
var ComposeFilePatterns = struct {
	Names      []string
	Extensions []string
}{
	Names:      []string{"docker-compose", "compose"},
	Extensions: []string{".yml", ".yaml"},
}

// DirectoriesToSkip contains directories to skip when scanning
var DirectoriesToSkip = []string{".git", "node_modules", "vendor"}

// FindComposeFiles finds all docker-compose files in the given directory
func (c *Config) FindComposeFiles() ([]string, error) {
	if c.ScanDir == "" {
		return nil, fmt.Errorf("scan directory not specified")
	}

	// Get the full scan path
	scanPath := c.GetScanPath()

	// Check if directory exists
	if err := validation.ValidateDirectory(scanPath); err != nil {
		return nil, err
	}

	logger.Debug("Scanning directory: %s", scanPath)

	// Find all docker-compose files recursively
	var composeFiles []string
	err := c.walkDirectory(scanPath, func(path string, info os.FileInfo) bool {
		if isComposeFile(info.Name()) {
			logger.Debug("Found compose file: %s", path)
			composeFiles = append(composeFiles, path)
			return true
		}
		return false
	})

	if err != nil {
		return nil, fmt.Errorf("error scanning directory: %w", err)
	}

	logger.Info("Found %d compose files in %s", len(composeFiles), scanPath)
	return composeFiles, nil
}

// walkDirectory walks through a directory and applies a filter function to each file
func (c *Config) walkDirectory(root string, filter func(path string, info os.FileInfo) bool) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories that should be ignored
		if info.IsDir() {
			for _, skipDir := range DirectoriesToSkip {
				if info.Name() == skipDir {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Apply filter to files
		filter(path, info)
		return nil
	})
}

// isComposeFile returns true if the filename is a docker-compose file
func isComposeFile(filename string) bool {
	// Check if the filename contains any of the compose patterns
	hasComposeInName := false
	for _, pattern := range ComposeFilePatterns.Names {
		if strings.Contains(filename, pattern) {
			hasComposeInName = true
			break
		}
	}

	// Check if the file has any of the yaml extensions
	hasYamlExtension := false
	for _, ext := range ComposeFilePatterns.Extensions {
		if strings.HasSuffix(filename, ext) {
			hasYamlExtension = true
			break
		}
	}

	// Return true if both conditions are met
	return hasComposeInName && hasYamlExtension
}

// GetRelativePath returns a path relative to the scan directory
func (c *Config) GetRelativePath(path string) string {
	if c.ScanDir == "" {
		return path
	}

	// Get the base scan path
	basePath := c.GetScanPath()

	// Calculate relative path
	relPath, err := filepath.Rel(basePath, path)
	if err != nil {
		// If there's an error, return the original path
		return path
	}

	return relPath
}

// ConfigureLogger configures the logger based on the current settings
func (c *Config) ConfigureLogger() {
	// Set default log level if not specified
	logLevel := logger.INFO
	if c.LogLevel != "" {
		logLevel = logger.ParseLevel(c.LogLevel)
	}

	// If verbose is enabled, override with DEBUG level
	if c.Verbose {
		logLevel = logger.DEBUG
	}

	// Configure the logger
	logger.SetLevel(logLevel)
	logger.SetQuiet(c.Quiet)

	// Log the configuration if not in quiet mode
	if !c.Quiet {
		logger.Debug("Logger configured with level: %s, quiet: %v", logger.GetLevel(), c.Quiet)
	}
}

// String returns a string representation of the configuration
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{LogLevel: %s, Verbose: %v, Quiet: %v, ScanDir: %s, CreateMR: %v, TargetBranch: %s}",
		c.LogLevel, c.Verbose, c.Quiet, c.ScanDir, c.CreateMR, c.TargetBranch,
	)
}

// ValidateAll performs a comprehensive validation of all configuration
// This should be called before running any command
func (c *Config) ValidateAll() error {
	// Validate general configuration
	if err := c.Validate(); err != nil {
		return err
	}

	// Validate GitLab configuration if creating merge requests
	if c.CreateMR {
		if err := c.ValidateGitLab(); err != nil {
			return err
		}
	}

	return nil
}
