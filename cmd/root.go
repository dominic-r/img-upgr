package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/config"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/logger"
	"gitlab.com/sdko-core/appli/img-upgr/pkg/version"
)

const (
	// ExitCodeSuccess indicates successful execution
	ExitCodeSuccess = 0
	// ExitCodeError indicates an error occurred
	ExitCodeError = 1
)

var (
	// rootCfg holds the application configuration
	rootCfg *config.Config

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "img-upgr",
		Short: "Docker image upgrade checker",
		Long: `A CLI tool to check for newer versions of Docker images in docker-compose files.
It parses semver-like tags and checks Docker Hub for newer versions.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Configure logger based on flags
			rootCfg.ConfigureLogger()

			if rootCfg.Verbose {
				PrintVerbose("Running with verbose logging")
				PrintVerbose("Version information: %s", version.GetInfo())
			}
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// It returns an exit code that can be used with os.Exit.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeError
	}
	return ExitCodeSuccess
}

// init initializes the root command and sets up configuration and flags
func init() {
	rootCfg = config.New()
	rootCfg.LoadFromEnv()

	// Define persistent flags that are global to the application
	rootCmd.PersistentFlags().BoolVarP(&rootCfg.Verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&rootCfg.Quiet, "quiet", "q", false, "Suppress all output except errors and updates")
	rootCmd.PersistentFlags().StringVar(&rootCfg.LogLevel, "log-level", rootCfg.LogLevel,
		"Set log level (DEBUG, INFO, WARN, ERROR, FATAL)")

	// Create a custom version command that uses our detailed version output
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  "Display the current version of the application and build information",
		Run: func(cmd *cobra.Command, args []string) {
			version.Print()
		},
	}

	// Add the version command
	rootCmd.AddCommand(versionCmd)
}

// GetConfig returns the root configuration
func GetConfig() *config.Config {
	return rootCfg
}

// IsVerbose returns true if the verbose flag is set
func IsVerbose() bool {
	return rootCfg.Verbose
}

// IsQuiet returns true if the quiet flag is set
func IsQuiet() bool {
	return rootCfg.Quiet
}

// PrintVerbose prints a message if verbose mode is enabled
func PrintVerbose(format string, args ...interface{}) {
	logger.Debug(format, args...)
}

// PrintInfo prints a message if not in quiet mode
func PrintInfo(format string, args ...interface{}) {
	logger.Info(format, args...)
}

// PrintError prints an error message
func PrintError(format string, args ...interface{}) {
	logger.Error(format, args...)
}

// PrintWarning prints a warning message
func PrintWarning(format string, args ...interface{}) {
	logger.Warn(format, args...)
}

// GetVersionInfo returns the version information
func GetVersionInfo() string {
	return version.GetInfo()
}
