package commands

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

// Audio controls the host's system audio outputs (PipeWire or PulseAudio).
var Audio = &cobra.Command{
	Use:   "audio <action>",
	Short: "Control the host's system audio outputs.",
	Long: `Control the host's system audio outputs (PipeWire via wpctl, PulseAudio via pactl).
When a device is omitted, the default output is used.

Actions:
  list                       list output devices
  volume <0.0-1.0> [device]  set volume
  mute [device]              mute
  unmute [device]            unmute
  default <device>           set the default output`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(_ *cobra.Command, args []string) error { return runAudio(args) },
}

func runAudio(args []string) error {
	action, rest := args[0], args[1:]
	var req models.Request

	optionalDevice := func(extra int) error {
		if len(rest) > extra {
			return errors.New("too many arguments for audio " + action)
		}
		if len(rest) == extra {
			req.Device = rest[extra-1]
		}
		return nil
	}

	switch action {
	case "list":
		if len(rest) != 0 {
			return errors.New("usage: quava audio list")
		}
		req.Command = "audio-list"

	case "volume":
		if len(rest) < 1 || len(rest) > 2 {
			return errors.New("usage: quava audio volume <0.0-1.0> [device]")
		}
		volume, err := strconv.ParseFloat(rest[0], 64)
		if err != nil || volume < 0 || volume > 1 {
			return errors.New("volume must be between 0.0 and 1.0")
		}
		req.Command, req.Volume = "audio-volume", volume
		if len(rest) == 2 {
			req.Device = rest[1]
		}

	case "mute", "unmute":
		if err := optionalDevice(1); err != nil {
			return err
		}
		req.Command, req.Enabled = "audio-mute", action == "mute"

	case "default":
		if len(rest) != 1 {
			return errors.New("usage: quava audio default <device>")
		}
		req.Command, req.Device = "audio-default", rest[0]

	default:
		return fmt.Errorf("unknown audio action %q (use list, volume, mute, unmute, or default)", action)
	}

	res, err := utils.Call(req)
	if err != nil {
		return err
	}

	if action == "list" {
		table := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(table, "DEFAULT\tVOLUME\tMUTED\tNAME\tDESCRIPTION")
		for _, device := range res.AudioDevices {
			marker := ""
			if device.IsDefault {
				marker = "*"
			}
			_, _ = fmt.Fprintf(table, "%s\t%.0f%%\t%t\t%s\t%s\n", marker, device.Volume*100, device.Muted, device.Name, device.Description)
		}
		return table.Flush()
	}

	fmt.Printf("Audio command %s completed.\n", action)
	return nil
}
