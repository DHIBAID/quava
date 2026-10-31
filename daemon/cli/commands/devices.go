package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"quava-cli/models"
	"quava-cli/utils"
)

// Devices lists paired devices, or shows one.
var Devices = &cobra.Command{
	Use:   "devices [peer-device-id]",
	Short: "List paired devices.",
	Long:  "List all paired devices as JSON, or show one by its device ID.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		req := models.Request{Command: "devices"}
		if len(args) == 1 {
			req.PeerDeviceID = args[0]
		}

		res, err := utils.Call(req)
		if err != nil {
			return err
		}

		if res.Device != nil {
			return utils.PrintJSON(res.Device)
		}

		return utils.PrintJSON(res.Devices)
	},
}

var discoverTimeout time.Duration

// Discover searches the network for devices running Quava.
var Discover = &cobra.Command{
	Use:   "discover",
	Short: "Discover devices on the network.",
	Long:  "Search the local network for devices running Quava. Use --timeout to search longer or shorter.",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		res, err := utils.Call(models.Request{Command: "discover", TimeoutMS: discoverTimeout.Milliseconds()})
		if err != nil {
			return err
		}

		if len(res.Discovered) == 0 {
			fmt.Println("[]")
			return nil
		}

		return utils.PrintJSON(res.Discovered)
	},
}

func init() {
	Discover.Flags().DurationVarP(&discoverTimeout, "timeout", "t", 10*time.Second, "how long to search")
}
