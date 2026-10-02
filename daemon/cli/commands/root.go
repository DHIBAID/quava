package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Root = &cobra.Command{
	Use:   "quava",
	Short: "Quava is an Android - Linux bridge.",
	Long:  `Quava is an everyday tool that allows an Android (client) to communicate with a Linux (host) system.`,

	SilenceUsage:  true,
	SilenceErrors: true,

	Run: func(command *cobra.Command, args []string) {
		fmt.Println("Quava works.")
	},
}

func Execute() {
	if err := Root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
