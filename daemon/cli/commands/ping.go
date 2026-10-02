package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"
	"time"

	"github.com/spf13/cobra"
)

var Ping = &cobra.Command{
	Use:   "ping <peer-device-id> [--timeout 10s]",
	Short: "Ping a device on the network.",
	Long:  `Ping a device on the network using its peer device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return ping(args)
	},
}

func ping(args []string) error {
	timeout := time.Duration(0)

	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}

	var res models.Response
	utils.Call(models.Request{
		Command:      "ping",
		PeerDeviceID: args[0],
		TimeoutMS:    timeout.Milliseconds(),
	}, &res)

	if res.Error != "" {
		return fmt.Errorf("error from daemon: %s", res.Error)
	}

	fmt.Printf("%s requested for %s\n", "ping", args[0])
	return nil
}
