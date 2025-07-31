package gitlab

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/sdko-core/appli/img-upgr/pkg/config"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
)

const (
	// DefaultGitTimeout is the default timeout for git operations
	DefaultGitTimeout = 60 * time.Second

	// GitCredentialsFile is the default path for git credentials file
	GitCredentialsFile = ".git-credentials"
)

// GitError represents an error that occurred during a git operation
type GitError struct {
	Operation string
	Err       error
	Output    string
}

// Error returns the error message
func (e *GitError) Error() string {
	if e.Output != "" {
		return fmt.Sprintf("%s failed: %v (output: %s)", e.Operation, e.Err, e.Output)
	}
	return fmt.Sprintf("%s failed: %v", e.Operation, e.Err)
}

// Unwrap returns the underlying error
func (e *GitError) Unwrap() error {
	return e.Err
}

// CloneRepository clones a GitLab repository to a temporary directory
func CloneRepository(cfg *config.Config) error {
	logger.Info("Cloning repository %s", cfg.GitLabRepo)

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "img-upgr-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	cfg.TempDir = tempDir
	logger.Debug("Created temporary directory: %s", tempDir)

	// Set up git credentials
	if err := setupGitCredentials(cfg); err != nil {
		return err
	}

	// Clone repository
	logger.Info("Cloning repository %s to %s", cfg.GitLabRepo, tempDir)
	if err := runGitCommand(tempDir, "clone", cfg.GitLabRepo, tempDir); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}
	logger.Debug("Repository cloned successfully")

	// Configure git user in the repository
	if err := configureGitUser(cfg, tempDir); err != nil {
		return err
	}

	// Update scan directory to be inside the cloned repository
	updateScanDirectory(cfg, tempDir)

	cfg.ClonedRepo = true
	logger.Info("Repository setup complete")
	return nil
}

// CleanupRepository removes the temporary directory
func CleanupRepository(cfg *config.Config) {
	if cfg.TempDir == "" {
		return
	}

	logger.Debug("Cleaning up temporary directory: %s", cfg.TempDir)
	if err := os.RemoveAll(cfg.TempDir); err != nil {
		logger.Warn("Failed to clean up temporary directory: %v", err)
	} else {
		logger.Debug("Temporary directory removed successfully")
	}
}

// CreateBranchInRepo creates a new branch in the cloned repository
func CreateBranchInRepo(cfg *config.Config, branchName, baseBranch string) error {
	logger.Debug("Creating branch %s from %s", branchName, baseBranch)
	if err := validateRepoCloned(cfg); err != nil {
		return err
	}

	// Checkout base branch
	logger.Debug("Checking out base branch: %s", baseBranch)
	if err := runGitCommand(cfg.TempDir, "checkout", baseBranch); err != nil {
		return fmt.Errorf("failed to checkout base branch: %w", err)
	}

	// Pull latest changes
	logger.Debug("Pulling latest changes from origin/%s", baseBranch)
	if err := runGitCommand(cfg.TempDir, "pull", "origin", baseBranch); err != nil {
		return fmt.Errorf("failed to pull latest changes: %w", err)
	}

	// Create new branch
	logger.Debug("Creating new branch: %s", branchName)
	if err := runGitCommand(cfg.TempDir, "checkout", "-b", branchName); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	logger.Info("Created branch %s successfully", branchName)
	return nil
}

// CommitAndPushChanges commits and pushes changes to the remote repository
func CommitAndPushChanges(cfg *config.Config, message string) error {
	logger.Debug("Committing and pushing changes with message: %s", message)
	if err := validateRepoCloned(cfg); err != nil {
		return err
	}

	// Add all changes
	logger.Debug("Adding all changes")
	if err := runGitCommand(cfg.TempDir, "add", "."); err != nil {
		return fmt.Errorf("failed to add changes: %w", err)
	}

	// Commit changes
	logger.Debug("Committing changes with message: %s", message)
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = cfg.TempDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		// Check if there are no changes to commit
		if strings.Contains(string(output), "nothing to commit") {
			logger.Warn("No changes to commit")
			return fmt.Errorf("no changes to commit")
		}
		return &GitError{
			Operation: "commit",
			Err:       err,
			Output:    string(output),
		}
	}
	logger.Debug("Changes committed successfully")

	// Push changes
	logger.Debug("Pushing changes to origin")
	if err := runGitCommand(cfg.TempDir, "push", "origin", "HEAD"); err != nil {
		return fmt.Errorf("failed to push changes: %w", err)
	}

	logger.Info("Changes pushed successfully")
	return nil
}

// GetCurrentBranch returns the current branch name
func GetCurrentBranch(cfg *config.Config) (string, error) {
	logger.Debug("Getting current branch name")
	if err := validateRepoCloned(cfg); err != nil {
		return "", err
	}

	// Get current branch
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = cfg.TempDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	branchName := strings.TrimSpace(string(output))
	logger.Debug("Current branch is: %s", branchName)
	return branchName, nil
}

// GetDefaultBranch returns the default branch of the repository
func GetDefaultBranch(cfg *config.Config) (string, error) {
	logger.Debug("Getting default branch for repository")
	if err := validateRepoCloned(cfg); err != nil {
		return "", err
	}

	// First try to get the default branch from git remote show origin
	cmd := exec.Command("git", "remote", "show", "origin")
	cmd.Dir = cfg.TempDir

	output, err := cmd.Output()
	if err == nil {
		// Parse the output to find the default branch
		outputStr := string(output)
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "HEAD branch:") {
				defaultBranch := strings.TrimSpace(strings.TrimPrefix(line, "HEAD branch:"))
				logger.Debug("Found default branch from remote: %s", defaultBranch)
				return defaultBranch, nil
			}
		}
	}

	// If that fails, try to get the symbolic-ref of HEAD
	cmd = exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	cmd.Dir = cfg.TempDir

	output, err = cmd.Output()
	if err == nil {
		defaultBranch := strings.TrimSpace(string(output))
		// Remove the origin/ prefix
		defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")
		logger.Debug("Found default branch from symbolic ref: %s", defaultBranch)
		return defaultBranch, nil
	}

	// If all else fails, assume "main" as the default branch
	logger.Warn("Could not determine default branch, using 'main' as fallback")
	return "main", nil
}

// GetRepoStatus returns the git status of the repository
func GetRepoStatus(cfg *config.Config) (string, error) {
	logger.Debug("Getting repository status")
	if err := validateRepoCloned(cfg); err != nil {
		return "", err
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = cfg.TempDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get repository status: %w", err)
	}

	return string(output), nil
}

// HasChanges checks if there are uncommitted changes in the repository
func HasChanges(cfg *config.Config) (bool, error) {
	status, err := GetRepoStatus(cfg)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(status) != "", nil
}

// setupGitCredentials configures git to use stored credentials
func setupGitCredentials(cfg *config.Config) error {
	logger.Debug("Configuring git credentials")
	if err := runGitCommand("", "config", "--global", "credential.helper", "store"); err != nil {
		return fmt.Errorf("failed to configure git credentials: %w", err)
	}

	// Create credentials file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	credentialsFile := filepath.Join(homeDir, GitCredentialsFile)
	logger.Debug("Writing credentials to %s", credentialsFile)

	credentialURL := fmt.Sprintf("https://%s:%s@%s\n",
		cfg.GitLabUser,
		cfg.GitLabToken,
		extractHostFromURL(cfg.GitLabRepo))

	if err := os.WriteFile(credentialsFile, []byte(credentialURL), 0600); err != nil {
		return fmt.Errorf("failed to write git credentials: %w", err)
	}

	return nil
}

// configureGitUser sets up the git user name and email in the repository
func configureGitUser(cfg *config.Config, repoDir string) error {
	// Set up git user name
	logger.Debug("Setting git user name to %s", cfg.GitLabUser)
	if err := runGitCommand(repoDir, "config", "user.name", cfg.GitLabUser); err != nil {
		return fmt.Errorf("failed to set git user name: %w", err)
	}

	// Set up git email
	logger.Debug("Setting git user email to %s", cfg.GitLabEmail)
	if err := runGitCommand(repoDir, "config", "user.email", cfg.GitLabEmail); err != nil {
		return fmt.Errorf("failed to set git user email: %w", err)
	}

	return nil
}

// updateScanDirectory updates the scan directory to be inside the cloned repository
func updateScanDirectory(cfg *config.Config, tempDir string) {
	if cfg.ScanDir == "" {
		cfg.ScanDir = tempDir
		logger.Debug("Using repository root as scan directory: %s", cfg.ScanDir)
		return
	}

	// If ScanDir is relative, make it relative to the cloned repo
	if !filepath.IsAbs(cfg.ScanDir) {
		oldScanDir := cfg.ScanDir
		cfg.ScanDir = filepath.Join(tempDir, cfg.ScanDir)
		logger.Debug("Updated scan directory from %s to %s", oldScanDir, cfg.ScanDir)
	}
}

// validateRepoCloned checks if the repository is cloned
func validateRepoCloned(cfg *config.Config) error {
	if !cfg.ClonedRepo || cfg.TempDir == "" {
		return fmt.Errorf("repository not cloned")
	}
	return nil
}

// runGitCommand runs a git command with the given arguments
func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return &GitError{
			Operation: "git " + strings.Join(args, " "),
			Err:       err,
			Output:    string(output),
		}
	}

	return nil
}

// extractHostFromURL extracts the host from a URL
func extractHostFromURL(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove path
	if i := strings.Index(url, "/"); i != -1 {
		url = url[:i]
	}

	return url
}
