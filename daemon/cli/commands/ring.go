package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Ring = &cobra.Command{
	Use:   "ring <peer-device-id>",
	Short: "Ring a device on the network.",
	Long:  `Ring a device on the network using its peer device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return ring(args)
	},
}

func ring(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}

	var res models.Response
	utils.Call(models.Request{
		Command:      "ring",
		PeerDeviceID: args[0],
	}, &res)

	if res.Error != "" {
		return fmt.Errorf("error from daemon: %s", res.Error)
	}

	fmt.Printf("%s requested for %s\n", "ring", args[0])
	return nil
}
