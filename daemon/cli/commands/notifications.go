package commands

import (
	"fmt"
	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Notification = &cobra.Command{
	Use:   "notification <direction> <peer-device-id>",
	Short: "Toggle notification sync directions.",
	Long:  `Allow bidirectional, one-directional or no notification sync.`,
	Args:  cobra.ExactArgs(2),

	RunE: func(command *cobra.Command, args []string) error {
		return notifications(args)
	},
}

func notifications(args []string) error {

	// toClient: Linux -> Android. toHost: Android -> Linux.
	direction := args[0]

	switch direction {
	case "both":
		var res models.Response
		utils.Call(models.Request{
			Command:      "notif-sync-both",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "notification sync, bidirectional", args[1])
		return nil

	case "none":
		var res models.Response
		utils.Call(models.Request{
			Command:      "notif-sync-stop",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "notification sync, stop", args[1])
		return nil

	case "toClient":
		var res models.Response
		utils.Call(models.Request{
			Command:      "notif-sync-toclient",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "notification sync, toClient", args[1])
		return nil

	case "toHost":
		var res models.Response
		utils.Call(models.Request{
			Command:      "notif-sync-tohost",
			PeerDeviceID: args[1],
		}, &res)

		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}

		fmt.Printf("%s sent for %s\n", "notification sync, toHost", args[1])
		return nil

	}

	return fmt.Errorf("invalid notification direction %q (use both, none, toClient, or toHost)", direction)
}
