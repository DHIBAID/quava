package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"quava-cli/models"
	"quava-cli/utils"
)

// peerAction is one daemon command applied to one paired device.
type peerAction struct {
	command string // daemon command to send
	done    string // printed on success; %s is the device ID
}

// peerCommand builds `quava <name> <peer-device-id>`, which sends a single
// daemon command for that device.
func peerCommand(name, short, long string, action peerAction) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <peer-device-id>",
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runPeerAction(action, args[0])
		},
	}
}

// actionCommand builds `quava <name> <action> <peer-device-id>`, where the
// action picks one of several daemon commands.
func actionCommand(name, short, long string, actions map[string]peerAction) *cobra.Command {
	names := make([]string, 0, len(actions))
	for action := range actions {
		names = append(names, action)
	}
	sort.Strings(names)
	choices := strings.Join(names, "|")

	return &cobra.Command{
		Use:   name + " <" + choices + "> <peer-device-id>",
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			action, ok := actions[args[0]]
			if !ok {
				return fmt.Errorf("unknown %s action %q (use %s)", name, args[0], strings.Join(names, ", "))
			}
			return runPeerAction(action, args[1])
		},
	}
}

func runPeerAction(action peerAction, peerID string) error {
	if _, err := utils.Call(models.Request{Command: action.command, PeerDeviceID: peerID}); err != nil {
		return err
	}

	fmt.Printf(action.done+"\n", peerID)
	return nil
}
