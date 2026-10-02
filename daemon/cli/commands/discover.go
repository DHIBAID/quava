package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"
	"time"

	"github.com/spf13/cobra"
)

var Discover = &cobra.Command{
	Use:   "discover [--timeout 10s]",
	Short: "Discover devices on the network.",
	Long:  `Discover devices on the network. Optionally specify a timeout for the discovery process.`,

	RunE: func(command *cobra.Command, args []string) error {
		return discover(args)
	},
}

func discover(args []string) error {
	timeout, rest := utils.ParseTimeout("discover", args, 10*time.Second)

	if len(rest) != 0 {
		return fmt.Errorf("invalid number of arguments")
	}

	var res models.Response

	utils.Call(models.Request{Command: "discover", TimeoutMS: timeout.Milliseconds()}, &res)
	utils.PrintReply(res)
	return nil
}
