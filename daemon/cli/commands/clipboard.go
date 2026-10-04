package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Clipboard = &cobra.Command{
	Use:   "clipboard <option> <peer-device-id>",
	Short: "Send the host's clipboard content to the client.",
	Long:  `Send the host's clipboard content to the client using its peer device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return clipboard(args)
	},
}

func clipboard(args []string) error {

	// option has to be oneof: "push", "start", "stop"
	option := args[0]

	switch option {
	case "push":
		var res models.Response
		utils.Call(models.Request{
			Command:      "clipboard-push",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "clipboard-push", args[0])
		return nil
	case "start":
		var res models.Response
		utils.Call(models.Request{
			Command:      "clipboard-service-start",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "clipboard-service-start", args[0])
		return nil

	case "stop":
		var res models.Response
		utils.Call(models.Request{
			Command:      "clipboard-service-stop",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "clipboard-service-stop", args[0])
		return nil
	}

	return nil
}
