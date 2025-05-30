package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tjun/tfsort/internal/commands" // Import the new package
	"github.com/urfave/cli/v3"
)

var (
	// These variables are set by goreleaser at build time
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// NewApp creates and configures the cli.Command instance.
func NewApp() *cli.Command {
	cmd := &cli.Command{
		Name:    "tfsort",
		Usage:   "A fast, opinionated sorter for Terraform configuration files.",
		Version: fmt.Sprintf("%s, commit %s, built at %s", version, commit, date),
		Flags:   commands.GetFlags(),   // Get flags from the commands package
		Action:  commands.TfsortAction, // Use the action from the commands package
		// Hide the default 'help' command generated by urfave/cli
		// because we are not using subcommands for the main functionality.
		// The default help message (triggered by -h or --help) is still shown.
		HideHelpCommand: true,
	}
	return cmd
}

func main() {
	// Create the app using NewApp and run it.
	// If Run returns an error, log it fatally.
	if err := NewApp().Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
