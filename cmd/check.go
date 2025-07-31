package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/compose"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/config"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/docker"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/gitlab"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/update"
)

var (
	// checkCfg holds the configuration for the check command
	checkCfg *config.Config
)

// UpdateInfo represents information about an image update
type UpdateInfo struct {
	FilePath    string
	ServiceName string
	OldImage    string
	NewImage    string
	Repository  string
	OldTag      string
	NewTag      string
}

var checkCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Check docker-compose file for image updates",
	Long: `Check docker-compose files in a GitLab repository for image updates.
The repository is cloned using the IMG_UPGR_GL_REPO environment variable.
The scan directory is specified using the IMG_UPGR_SCANDIR environment variable.

Examples:
  img-upgr check            Check compose files using environment variables
  img-upgr check --dry-run  Check for updates without creating merge requests`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Create a context that can be cancelled
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			logger.Info("Received interrupt signal, shutting down gracefully...")
			cancel()
		}()

		// Run the check command with context
		if err := runCheckCommand(ctx, args); err != nil {
			logger.Error("Check command failed: %v", err)
			os.Exit(1)
		}
	},
}

// runCheckCommand is the main function for the check command
func runCheckCommand(ctx context.Context, args []string) error {
	// Initialize and validate configuration
	if err := initializeAndValidate(); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	// Clean up repository when done
	defer gitlab.CleanupRepository(checkCfg)

	// Determine the files to scan
	composeFiles, err := determineFilesToScan(args)
	if err != nil {
		return fmt.Errorf("failed to determine files to scan: %w", err)
	}

	// Create Docker client
	dockerClient := docker.NewClient()

	// Process files and collect updates
	updates, err := processComposeFilesWithContext(ctx, composeFiles, dockerClient)
	if err != nil {
		return fmt.Errorf("error processing compose files: %w", err)
	}

	// Handle found updates
	return handleUpdates(ctx, updates)
}

// initializeAndValidate initializes and validates the configuration
func initializeAndValidate() error {
	// Comprehensive validation of all configuration
	logger.Debug("Validating configuration...")

	// First validate GitLab configuration if we need to clone the repo
	if checkCfg.GitLabRepo != "" {
		if err := checkCfg.ValidateGitLab(); err != nil {
			return fmt.Errorf("GitLab configuration validation failed: %w", err)
		}

		// Initialize GitLab client
		gitlabClient, err := gitlab.NewClient(checkCfg)
		if err != nil {
			return fmt.Errorf("error initializing GitLab client: %w", err)
		}
		checkCfg.GitLabClient = gitlabClient

		// Clone repository before validating scan directory
		logger.Info("Cloning repository: %s", checkCfg.GitLabRepo)
		if err := gitlab.CloneRepository(checkCfg); err != nil {
			return fmt.Errorf("error cloning repository: %w", err)
		}
	}

	// Now validate all configuration (after repository is cloned if needed)
	if err := checkCfg.ValidateAll(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	logger.Debug("Configuration validated successfully")
	return nil
}

// determineFilesToScan determines which files to scan based on arguments and configuration
func determineFilesToScan(args []string) ([]string, error) {
	// Determine the file or directory to scan
	var scanPath string
	if len(args) > 0 {
		// User specified a path
		scanPath = args[0]
	} else if checkCfg.ScanDir != "" {
		// Use the scan directory from config
		scanPath = checkCfg.ScanDir
	} else {
		// Default to docker-compose.yml in repo root
		scanPath = "docker-compose.yml"
	}

	// If path is not absolute and we have a temp dir, make it relative to temp dir
	if !filepath.IsAbs(scanPath) && checkCfg.TempDir != "" {
		scanPath = filepath.Join(checkCfg.TempDir, scanPath)
		logger.Debug("Using path relative to cloned repo: %s", scanPath)
	}

	// Check if path exists
	fileInfo, err := os.Stat(scanPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", scanPath)
	} else if err != nil {
		return nil, fmt.Errorf("error accessing path: %v", err)
	}

	// Handle directory or file
	var composeFiles []string
	if fileInfo.IsDir() {
		// It's a directory, use FindComposeFiles
		checkCfg.ScanDir = scanPath
		files, err := checkCfg.FindComposeFiles()
		if err != nil {
			return nil, fmt.Errorf("error finding compose files: %w", err)
		}
		composeFiles = files
	} else {
		// It's a file, just use this one file
		composeFiles = []string{scanPath}
	}

	// Check if we found any files
	if len(composeFiles) == 0 {
		return nil, fmt.Errorf("no compose files found in %s", scanPath)
	}

	return composeFiles, nil
}

// processComposeFilesWithContext processes each compose file and returns updates
func processComposeFilesWithContext(ctx context.Context, composeFiles []string, dockerClient *docker.Client) ([]UpdateInfo, error) {
	var updates []UpdateInfo
	var mu sync.Mutex // Mutex for thread-safe updates to the updates slice

	// Process each compose file
	for _, composeFilePath := range composeFiles {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		logger.Info("Processing compose file: %s", composeFilePath)

		// Parse compose file
		composeFile, err := compose.ParseComposeFile(composeFilePath)
		if err != nil {
			logger.Error("Error parsing compose file %s: %v", composeFilePath, err)
			continue
		}

		// Check each image
		images := composeFile.GetImages()
		if len(images) == 0 {
			logger.Info("No images found in compose file %s", composeFilePath)
			continue
		}

		PrintInfo("Found %d services with images in %s", len(images), filepath.Base(composeFilePath))

		// Process each image
		fileUpdates, err := processImagesInFile(ctx, composeFilePath, images, dockerClient)
		if err != nil {
			logger.Error("Error processing images in %s: %v", composeFilePath, err)
			continue
		}

		// Add file updates to overall updates
		mu.Lock()
		updates = append(updates, fileUpdates...)
		mu.Unlock()
	}

	return updates, nil
}

// processImagesInFile processes all images in a single compose file
func processImagesInFile(ctx context.Context, filePath string, images map[string]string, dockerClient *docker.Client) ([]UpdateInfo, error) {
	var updates []UpdateInfo

	for serviceName, imageName := range images {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		PrintInfo("Checking image for service %s: %s", serviceName, imageName)

		info, err := update.CheckImage(imageName, dockerClient)
		if err != nil {
			if strings.Contains(err.Error(), "no tag found") ||
				strings.Contains(err.Error(), "tag not semver-like") {
				PrintInfo("  Skipping %s: %v", serviceName, err)
				continue
			}
			logger.Error("  Error checking %s: %v", serviceName, err)
			continue
		}

		// Print version info
		PrintVerbose("  Parsed version: prefix='%s', version=%s", info.Prefix, info.Version)

		if info.LatestVersion == nil {
			PrintInfo("  No matching versions found for %s", serviceName)
			continue
		}

		if info.HasUpdate {
			// Add to updates list for merge request creation
			updates = append(updates, UpdateInfo{
				FilePath:    filePath,
				ServiceName: serviceName,
				OldImage:    imageName,
				NewImage:    fmt.Sprintf("%s:%s", info.Repository, info.LatestTag),
				Repository:  info.Repository,
				OldTag:      info.Tag,
				NewTag:      info.LatestTag,
			})
			green := color.New(color.FgGreen).SprintFunc()
			PrintInfo("  %s Update available: %s → %s", green("✓"), info.Tag, info.LatestTag)
			PrintInfo("     Suggested image: %s:%s", info.Repository, info.LatestTag)
		} else {
			PrintInfo("  ✓ Image is up to date")
		}
	}

	return updates, nil
}

// handleUpdates processes any updates that were found
func handleUpdates(ctx context.Context, updates []UpdateInfo) error {
	// Process updates if any were found
	if len(updates) > 0 {
		logger.Info("Found %d updates across all files", len(updates))

		// Create merge requests for updates if not in dry run mode
		if !checkCfg.DryRun {
			if err := createMergeRequestsForUpdates(ctx, checkCfg, updates); err != nil {
				return fmt.Errorf("failed to create merge requests: %w", err)
			}
		} else {
			logger.Info("Dry run mode: skipping merge request creation")
		}
	} else {
		logger.Info("No updates found across all files")
	}

	return nil
}

// createMergeRequestsWithContext creates merge requests for the found updates
func createMergeRequestsForUpdates(ctx context.Context, cfg *config.Config, updates []UpdateInfo) error {
	// Process each image update individually
	for _, update := range updates {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Create a unique branch name for each image update
		timestamp := time.Now().Format("20060102-150405")
		serviceSanitized := strings.ReplaceAll(update.ServiceName, "/", "-")
		branchName := fmt.Sprintf("img-upgr/%s-%s", serviceSanitized, timestamp)

		// Get default branch from repository
		defaultBranch, err := gitlab.GetDefaultBranch(cfg)
		if err != nil {
			logger.Error("Error getting default branch: %v", err)
			continue
		}

		// Create branch in local repository
		logger.Info("Creating branch %s for updating %s from default branch %s", branchName, update.ServiceName, defaultBranch)
		if err := gitlab.CreateBranchInRepo(cfg, branchName, defaultBranch); err != nil {
			logger.Error("Error creating branch: %v", err)
			continue
		}

		// Read file content
		filePath := update.FilePath
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Error reading file %s: %v", filePath, err)
			continue
		}

		// Update content with only this specific image
		logger.Info("Updating %s: %s → %s", update.ServiceName, update.OldImage, update.NewImage)
		newContent := strings.ReplaceAll(string(content), update.OldImage, update.NewImage)

		// Write updated content back to file
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			logger.Error("Error writing file %s: %v", filePath, err)
			continue
		}

		// Commit changes
		relPath := cfg.GetRelativePath(filePath)
		commitMsg := fmt.Sprintf("Update Docker image for %s in %s", update.ServiceName, filepath.Base(filePath))
		logger.Info("Committing changes to %s", relPath)
		if err := gitlab.CommitAndPushChanges(cfg, commitMsg); err != nil {
			logger.Error("Error committing changes: %v", err)
			continue
		}

		// Get current branch name
		currentBranch, err := gitlab.GetCurrentBranch(cfg)
		if err != nil {
			logger.Error("Error getting current branch: %v", err)
			continue
		}

		// Get default branch for merge request target
		defaultBranch, err = gitlab.GetDefaultBranch(cfg)
		if err != nil {
			logger.Error("Error getting default branch: %v", err)
			continue
		}

		// Create merge request with specific title and description for this image
		title := fmt.Sprintf("Update %s from %s to %s", update.ServiceName, update.OldTag, update.NewTag)
		description := formatMergeRequestDescription(update)

		logger.Info("Creating merge request for %s targeting %s", update.ServiceName, defaultBranch)
		gitlabClient, err := gitlab.NewClient(cfg)
		if err != nil {
			logger.Error("Error creating GitLab client: %v", err)
			continue
		}

		_, err = gitlabClient.CreateMergeRequest(currentBranch, defaultBranch, title, description)
		if err != nil {
			logger.Error("Error creating merge request: %v", err)
			continue
		}

		logger.Info("Created merge request successfully for %s", update.ServiceName)
	}

	return nil
}

// formatMergeRequestDescription builds a detailed description for the merge request
func formatMergeRequestDescription(update UpdateInfo) string {
	description := "Automated update of Docker image by img-upgr\n\n"
	description += fmt.Sprintf("Service: `%s`\n", update.ServiceName)
	description += fmt.Sprintf("File: `%s`\n", filepath.Base(update.FilePath))
	description += fmt.Sprintf("Update: `%s` → `%s`\n", update.OldTag, update.NewTag)
	description += fmt.Sprintf("Repository: `%s`\n", update.Repository)
	description += fmt.Sprintf("\nGenerated: %s", time.Now().Format(time.RFC3339))

	return description
}

func init() {
	checkCfg = config.New()
	checkCfg.LoadFromEnv()

	rootCmd.AddCommand(checkCmd)

	// Output format flag
	checkCmd.Flags().StringVarP(&checkCfg.OutputFormat, "output", "o", "text", "Output format (text, json)")

	// Behavior flags
	checkCmd.Flags().BoolVar(&checkCfg.DryRun, "dry-run", false, "Check for updates but don't create merge requests")
}
