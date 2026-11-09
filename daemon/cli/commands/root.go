package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Root is the `quava` command.
var Root = &cobra.Command{
	Use:   "quava",
	Short: "Quava is an Android - Linux bridge.",
	Long:  `Quava is an everyday tool that allows an Android (client) to communicate with a Linux (host) system.`,

	SilenceUsage:  true,
	SilenceErrors: true,

	// Running `quava` on its own answers with the basic usage line and a pointer
	// to the full command list, which stays behind `quava --help`.
	Run: func(cmd *cobra.Command, _ []string) {
		const usage = "Usage:\n  quava [command]\n\nRun 'quava --help' for the full list of commands.\n"
		_, _ = fmt.Fprint(cmd.OutOrStdout(), usage)
	},
}

// Execute runs the command line. Every failure ends up here, so the format is
// the same everywhere: "quava: <message>" on standard error, exit status 1.
func Execute() {
	if err := Root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "quava:", err)
		os.Exit(1)
	}
}
