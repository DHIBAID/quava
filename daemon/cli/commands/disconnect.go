package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Disconnect = &cobra.Command{
	Use:   "disconnect <peer-device-id>",
	Short: "Disconnect from a peer device.",
	Args:  cobra.ExactArgs(1),
	RunE: func(command *cobra.Command, args []string) error {
		var response models.Response
		utils.Call(models.Request{Command: "disconnect", PeerDeviceID: args[0]}, &response)
		if response.Error != "" {
			return fmt.Errorf("daemon returned error: %s", response.Error)
		}
		fmt.Printf("disconnect requested for %s\n", args[0])
		return nil
	},
}
