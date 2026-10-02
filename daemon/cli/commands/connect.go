package commands

import (
	"fmt"
	"os"
	"quava-cli/models"
	"quava-cli/utils"
	"time"

	"github.com/spf13/cobra"
)

var Connect = &cobra.Command{
	Use:   "connect <peer-device-id>",
	Short: "Connect to a peer device.",
	Long:  `Connect to a peer device by providing its device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return connect(args)
	},
}

func connect(args []string) error {
	timeout := time.Duration(0)

	if len(args) != 1 {
		utils.Usage()
		os.Exit(2)
	}

	var res models.Response
	utils.Call(models.Request{
		Command:      "connect",
		PeerDeviceID: args[0],
		TimeoutMS:    timeout.Milliseconds(),
	}, &res)

	if res.Error != "" {
		return fmt.Errorf("daemon returned error: %s", res.Error)
	}

	fmt.Printf("%s requested for %s\n", "connect", args[0])
	return nil
}
