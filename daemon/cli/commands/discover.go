package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"
	"time"

	"github.com/spf13/cobra"
)

var discoverTimeout time.Duration

var Discover = &cobra.Command{
	Use:   "discover",
	Short: "Discover devices on the network.",
	Long:  "Discover devices on the network. Optionally specify a timeout for the discovery process.",

	RunE: func(command *cobra.Command, args []string) error {
		return discover(args)
	},
}

func discover(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("discover does not accept positional arguments")
	}

	var res models.Response

	utils.Call(
		models.Request{
			Command:   "discover",
			TimeoutMS: discoverTimeout.Milliseconds(),
		},
		&res,
	)

	utils.PrintReply(res)
	return nil
}

func init() {
	Discover.Flags().DurationVarP(
		&discoverTimeout,
		"timeout",
		"t",
		10*time.Second,
		"Discovery timeout",
	)
}
