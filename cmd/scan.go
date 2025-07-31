package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/compose"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/config"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/docker"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/gitlab"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/update"
)

// UpdatedImage represents an image that has an update available
type UpdatedImage struct {
	ServiceName string // Name of the service in docker-compose
	FilePath    string // Path to the docker-compose file
	OldImage    string // Full old image name with tag
	NewImage    string // Full new image name with tag
	Repository  string // Image repository name
	OldTag      string // Old image tag
	NewTag      string // New image tag
}

// scanCmd represents the scan command
var scanCmd = &cobra.Command{
	Use:   "scan [directory]",
	Short: "Scan directory for docker-compose files and check for updates",
	Long: `Scan a directory for docker-compose files and check for image updates.
If no directory is specified, it will use the value of IMG_UPGR_SCANDIR environment variable.
The directory is relative to the repository root after cloning.
Can optionally create merge requests for updates.`,
	Run: runScanCmd,
}

// runScanCmd is the main function for the scan command
func runScanCmd(cmd *cobra.Command, args []string) {
	// Get directory to scan from args if provided
	if len(args) > 0 {
		cfg.ScanDir = args[0]
	}

	// Setup GitLab and clone repository
	if err := setupGitLab(); err != nil {
		logger.Fatal("GitLab setup failed: %v", err)
	}
	defer gitlab.CleanupRepository(cfg)

	// Find and process compose files
	updatedImages, err := processComposeFiles()
	if err != nil {
		logger.Error("Error processing compose files: %v", err)
		os.Exit(1)
	}

	// Handle updates if found
	if len(updatedImages) == 0 {
		PrintInfo("No updates found")
		return
	}

	PrintInfo("Found %d images to update", len(updatedImages))

	// Create merge requests if requested
	if cfg.CreateMR {
		createMergeRequests(updatedImages)
	}
}

// setupGitLab validates GitLab configuration, initializes the client and clones the repository
func setupGitLab() error {
	// Comprehensive validation of all configuration
	logger.Debug("Validating configuration...")

	// First validate GitLab configuration (required for cloning)
	if err := cfg.ValidateGitLab(); err != nil {
		return fmt.Errorf("GitLab configuration validation failed: %w", err)
	}

	// Initialize GitLab client
	gitlabClient, err := gitlab.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("error initializing GitLab client: %w", err)
	}
	cfg.GitLabClient = gitlabClient

	// Clone repository before validating scan directory
	logger.Info("Cloning repository: %s", cfg.GitLabRepo)
	if err := gitlab.CloneRepository(cfg); err != nil {
		return fmt.Errorf("error cloning repository: %w", err)
	}

	// Now validate all configuration (after repository is cloned)
	if err := cfg.ValidateAll(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Debug("Configuration validated successfully")
	return nil
}

// processComposeFiles finds and processes all docker-compose files in the scan directory
func processComposeFiles() ([]UpdatedImage, error) {
	// Find all docker-compose files
	composeFiles, err := cfg.FindComposeFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find compose files: %w", err)
	}

	if len(composeFiles) == 0 {
		fmt.Println("No docker-compose files found in", cfg.ScanDir)
		return nil, nil
	}

	PrintInfo("Found %d docker-compose files in %s", len(composeFiles), cfg.ScanDir)

	// Create Docker client
	dockerClient := docker.NewClient()

	// Track updates
	var updatedImages []UpdatedImage

	// Process each compose file
	for _, filePath := range composeFiles {
		images, err := processComposeFile(filePath, dockerClient)
		if err != nil {
			logger.Warn("Error processing %s: %v", filePath, err)
			continue
		}
		updatedImages = append(updatedImages, images...)
	}

	return updatedImages, nil
}

// processComposeFile processes a single docker-compose file and returns any images that need updates
func processComposeFile(filePath string, dockerClient *docker.Client) ([]UpdatedImage, error) {
	PrintInfo("Checking file: %s", filePath)

	// Parse compose file
	composeFile, err := compose.ParseComposeFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing file: %w", err)
	}

	// Check each image
	images := composeFile.GetImages()
	if len(images) == 0 {
		PrintInfo("  No images found in %s", filePath)
		return nil, nil
	}

	PrintInfo("  Found %d services with images", len(images))

	var updatedImages []UpdatedImage

	// Process each image
	for serviceName, imageName := range images {
		image, err := checkImageForUpdates(serviceName, imageName, filePath, dockerClient)
		if err != nil {
			logger.Debug("    Error checking %s: %v", serviceName, err)
			continue
		}

		if image != nil {
			updatedImages = append(updatedImages, *image)
		}
	}

	return updatedImages, nil
}

// checkImageForUpdates checks if an image has updates available
func checkImageForUpdates(serviceName, imageName, filePath string, dockerClient *docker.Client) (*UpdatedImage, error) {
	PrintInfo("  Checking image for service %s: %s", serviceName, imageName)

	info, err := update.CheckImage(imageName, dockerClient)
	if err != nil {
		if strings.Contains(err.Error(), "no tag found") ||
			strings.Contains(err.Error(), "tag not semver-like") {
			PrintVerbose("    Skipping %s: %v", serviceName, err)
			return nil, nil
		}
		return nil, fmt.Errorf("error checking image: %w", err)
	}

	// Print version info
	PrintVerbose("    Parsed version: prefix='%s', version=%s", info.Prefix, info.Version)

	if info.LatestVersion == nil {
		PrintVerbose("    No matching versions found for %s", serviceName)
		return nil, nil
	}

	if !info.HasUpdate {
		PrintVerbose("    ✓ Image is up to date")
		return nil, nil
	}

	PrintInfo("    ✓ Update available: %s → %s", info.Tag, info.LatestTag)
	PrintInfo("      Suggested image: %s:%s", info.Repository, info.LatestTag)

	return &UpdatedImage{
		ServiceName: serviceName,
		FilePath:    filePath,
		OldImage:    imageName,
		NewImage:    fmt.Sprintf("%s:%s", info.Repository, info.LatestTag),
		Repository:  info.Repository,
		OldTag:      info.Tag,
		NewTag:      info.LatestTag,
	}, nil
}

// createMergeRequests creates merge requests for each updated image
func createMergeRequests(updates []UpdatedImage) {
	// Verify GitLab client exists
	if cfg.GitLabClient == nil {
		logger.Error("GitLab client not initialized")
		return
	}

	// Verify repository was cloned
	if !cfg.ClonedRepo {
		logger.Error("Repository not cloned")
		return
	}

	// Process each image update individually
	for _, update := range updates {
		if err := createMergeRequestForUpdate(update); err != nil {
			logger.Error("Failed to create merge request for %s: %v",
				update.ServiceName, err)
			continue
		}

		PrintInfo("Created merge request successfully for %s", update.ServiceName)
	}
}

// createMergeRequestForUpdate creates a merge request for a single image update
func createMergeRequestForUpdate(update UpdatedImage) error {
	// Create a unique branch name
	branchName := generateBranchName(update.ServiceName)

	// Create branch in local repository
	PrintInfo("Creating branch %s for updating %s", branchName, update.ServiceName)
	if err := gitlab.CreateBranchInRepo(cfg, branchName, cfg.TargetBranch); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	// Update file content
	if err := updateFileContent(update); err != nil {
		return fmt.Errorf("failed to update file content: %w", err)
	}

	// Commit and push changes
	relPath := cfg.GetRelativePath(update.FilePath)
	PrintInfo("Committing changes to %s", relPath)

	commitMsg := fmt.Sprintf("Update Docker image for %s in %s",
		update.ServiceName, filepath.Base(update.FilePath))

	if err := gitlab.CommitAndPushChanges(cfg, commitMsg); err != nil {
		return fmt.Errorf("failed to commit changes: %w", err)
	}

	// Create merge request
	if err := submitMergeRequest(update); err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

// generateBranchName creates a unique branch name for an update
func generateBranchName(serviceName string) string {
	timestamp := time.Now().Format("20060102-150405")
	sanitizedName := sanitizeBranchName(serviceName)
	return fmt.Sprintf("img-upgr/%s-%s", sanitizedName, timestamp)
}

// sanitizeBranchName removes characters that are not allowed in branch names
func sanitizeBranchName(name string) string {
	// Replace slashes with hyphens
	name = strings.ReplaceAll(name, "/", "-")

	// Replace other potentially problematic characters
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, ".", "-")

	return name
}

// updateFileContent updates the image reference in the file
func updateFileContent(update UpdatedImage) error {
	// Read file content
	content, err := os.ReadFile(update.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Update content with only this specific image
	newContent := strings.Replace(string(content), update.OldImage, update.NewImage, -1)

	// Write updated content back to file
	if err := os.WriteFile(update.FilePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// submitMergeRequest creates and submits a merge request for the changes
func submitMergeRequest(update UpdatedImage) error {
	// Get current branch name
	currentBranch, err := gitlab.GetCurrentBranch(cfg)
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// Create merge request title and description
	title := fmt.Sprintf("Update %s from %s to %s",
		update.ServiceName, update.OldTag, update.NewTag)

	description := buildMergeRequestDescription(update)

	PrintInfo("Creating merge request for %s", update.ServiceName)

	// Get GitLab client
	gitlabClient, ok := cfg.GitLabClient.(*gitlab.Client)
	if !ok {
		return fmt.Errorf("invalid GitLab client type")
	}

	// Create the merge request
	_, err = gitlabClient.CreateMergeRequest(
		currentBranch, cfg.TargetBranch, title, description)
	if err != nil {
		return fmt.Errorf("failed to create merge request: %w", err)
	}

	return nil
}

// buildMergeRequestDescription creates a description for the merge request
func buildMergeRequestDescription(update UpdatedImage) string {
	description := "Automated update of Docker image by img-upgr\n\n"
	description += fmt.Sprintf("Service: `%s`\n", update.ServiceName)
	description += fmt.Sprintf("File: `%s`\n", filepath.Base(update.FilePath))
	description += fmt.Sprintf("Update: `%s` → `%s`\n", update.OldTag, update.NewTag)

	return description
}

var cfg *config.Config

// init initializes the scan command
func init() {
	cfg = config.New()
	cfg.LoadFromEnv()

	rootCmd.AddCommand(scanCmd)

	// Add command-specific flags
	scanCmd.Flags().BoolVar(&cfg.CreateMR, "create-mr", false, "Create merge requests for updates")
	scanCmd.Flags().StringVar(&cfg.TargetBranch, "target-branch", cfg.TargetBranch, "Target branch for merge requests")
}
