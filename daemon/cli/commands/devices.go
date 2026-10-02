package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Devices = &cobra.Command{
	Use:   "devices [peer-device-id]",
	Short: "List connected devices.",
	Long:  `List all connected devices or details of a specific device by providing its device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return devices(args)
	},
}

func devices(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("invalid number of arguments")
	}

	req := models.Request{Command: "devices"}

	if len(args) == 1 {
		req.PeerDeviceID = args[0]
	}

	var res models.Response
	utils.Call(req, &res)
	utils.PrintReply(res)

	return nil
}
