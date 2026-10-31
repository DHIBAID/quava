package commands

import (
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

// Status lists paired devices with their live connection state.
var Status = &cobra.Command{
	Use:   "status",
	Short: "Show paired devices and whether each is connected.",
	Args:  cobra.NoArgs,
	RunE:  func(_ *cobra.Command, _ []string) error { return status() },
}

func status() error {
	res, err := utils.Call(models.Request{Command: "devices"})
	if err != nil {
		return err
	}

	if len(res.Devices) == 0 {
		fmt.Println("No paired devices.")
		return nil
	}

	table := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "DEVICE ID\tNAME\tSTATE")
	for _, device := range res.Devices {
		state := "disconnected"
		if slices.Contains(res.ConnectedDeviceIDs, device.PeerDeviceID) {
			state = "connected"
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\n", device.PeerDeviceID, device.PeerDeviceName, state)
	}
	return table.Flush()
}
