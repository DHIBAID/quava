package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Input = &cobra.Command{
	Use:   "input <enable|disable> <peer-device-id>",
	Short: "Enable or disable remote input for a paired device.",
	Args:  cobra.ExactArgs(2),
	RunE: func(command *cobra.Command, args []string) error {
		var cmd string
		switch args[0] {
		case "enable":
			cmd = "input-enable"
		case "disable":
			cmd = "input-disable"
		default:
			return fmt.Errorf("invalid input action %q (use enable or disable)", args[0])
		}
		var response models.Response
		utils.Call(models.Request{Command: cmd, PeerDeviceID: args[1]}, &response)
		if response.Error != "" {
			return fmt.Errorf("daemon returned error: %s", response.Error)
		}
		fmt.Printf("Remote input %s for %s\n", map[string]string{"enable": "enabled", "disable": "disabled"}[args[0]], args[1])
		return nil
	},
}
