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

	Args: cobra.ExactArgs(2),
	RunE: func(command *cobra.Command, args []string) error {
		return clipboard(args)
	},
}

func clipboard(args []string) error {

	// options: push, start, stop, sync-on, sync-off
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

		fmt.Printf("%s sent for %s\n", "clipboard-push", args[1])
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

		fmt.Printf("%s sent\n", "clipboard-service-start")
		return nil

	case "sync-on", "sync-off":
		cmd := "clipboard-sync-on"
		if option == "sync-off" {
			cmd = "clipboard-sync-off"
		}
		var res models.Response
		utils.Call(models.Request{Command: cmd, PeerDeviceID: args[1]}, &res)
		if res.Error != "" {
			return fmt.Errorf("error from daemon: %s", res.Error)
		}
		fmt.Printf("Automatic clipboard sync %s for %s\n", map[string]string{"sync-on": "enabled", "sync-off": "disabled"}[option], args[1])
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

		fmt.Printf("%s sent\n", "clipboard-service-stop")
		return nil
	}

	return fmt.Errorf("invalid clipboard option %q (use push, start, stop, sync-on, or sync-off)", option)
}
