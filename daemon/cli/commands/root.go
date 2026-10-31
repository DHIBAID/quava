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

	Run: func(_ *cobra.Command, args []string) {
		fmt.Println("Quava works.")
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
