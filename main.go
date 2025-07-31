package main

import (
	"os"

	"gitlab.com/sdko-core/appli/img-upgr/cmd"
)

func main() {
	// Execute the root command and get the exit code
	exitCode := cmd.Execute()

	// Exit with the appropriate code
	os.Exit(exitCode)
}
