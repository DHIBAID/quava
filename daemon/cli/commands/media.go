package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"quava-cli/models"
	"quava-cli/utils"

	"github.com/spf13/cobra"
)

var Media = &cobra.Command{
	Use:   "media <action>",
	Short: "Discover and control host media players through MPRIS.",
	Long:  "Control MPRIS-compatible media players on the host.\nWhen a player bus name is omitted, the active player is used.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  func(_ *cobra.Command, args []string) error { return runMedia(args) },
}

func runMedia(args []string) error {
	action, req := args[0], models.Request{}
	switch action {
	case "list", "active":
		if len(args) != 1 {
			return fmt.Errorf("usage: quava media %s", action)
		}

		if action == "list" {
			req.Command = "media-list"
		} else {
			req.Command = "media-active"
		}

	case "set-active":
		if len(args) != 2 {
			return fmt.Errorf("usage: quava media set-active <bus-name>")
		}

		req.Command, req.BusName = "media-set-active", args[1]

	case "play-pause", "play", "pause", "stop", "next", "previous":
		if len(args) > 2 {
			return fmt.Errorf("usage: quava media %s [bus-name]", action)
		}

		req.Command = "media-" + action
		if len(args) == 2 {
			req.BusName = args[1]
		}

	case "seek":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: quava media seek <offset-ms> [bus-name]")
		}

		offset, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid seek offset %q: %w", args[1], err)
		}

		req.Command, req.OffsetMS = "media-seek", offset
		if len(args) == 3 {
			req.BusName = args[2]
		}

	case "shuffle":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: quava media shuffle <on|off> [bus-name]")
		}

		switch strings.ToLower(args[1]) {
		case "on":
			req.Enabled = true

		case "off":

		default:
			return fmt.Errorf("shuffle must be on or off")
		}

		req.Command = "media-shuffle"
		if len(args) == 3 {
			req.BusName = args[2]
		}

	case "loop":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: quava media loop <none|track|playlist> [bus-name]")
		}

		statuses := map[string]string{
			"none":     "None",
			"track":    "Track",
			"playlist": "Playlist",
		}

		status, ok := statuses[strings.ToLower(args[1])]
		if !ok {
			return fmt.Errorf("loop must be none, track, or playlist")
		}

		req.Command, req.LoopStatus = "media-loop", status
		if len(args) == 3 {
			req.BusName = args[2]
		}

	case "volume":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: quava media volume <0.0-1.0> [bus-name]")
		}

		volume, err := strconv.ParseFloat(args[1], 64)
		if err != nil || volume < 0 || volume > 1 {
			return fmt.Errorf("volume must be between 0.0 and 1.0")
		}

		req.Command, req.Volume = "media-volume", volume
		if len(args) == 3 {
			req.BusName = args[2]
		}

	default:
		return fmt.Errorf("unknown media action %q (use list, active, set-active, play-pause, play, pause, stop, next, previous, seek, shuffle, loop, or volume)", action)
	}

	var res models.Response

	utils.Call(req, &res)
	if res.Error != "" {
		return fmt.Errorf("daemon returned error: %s", res.Error)
	}

	if res.Players != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(res.Players)
	}

	if res.Player != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(res.Player)
	}

	fmt.Printf("Media command %s completed.\n", action)
	return nil
}
