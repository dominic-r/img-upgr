package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current version of the application
	Version = "dev"
	// Commit is the git commit hash of the build
	Commit = "none"
	// BuildDate is the date when the binary was built
	BuildDate = "unknown"
)

// GetVersion returns just the version string
func GetVersion() string {
	return Version
}

// GetInfo returns formatted version information
func GetInfo() string {
	return fmt.Sprintf("Version: %s, Commit: %s, Build Date: %s, Go: %s, OS/Arch: %s/%s",
		Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// Print outputs the version information to stdout
func Print() {
	fmt.Println("img-upgr version:", Version)
	fmt.Println("Commit:", Commit)
	fmt.Println("Build Date:", BuildDate)
	fmt.Println("Go Version:", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
