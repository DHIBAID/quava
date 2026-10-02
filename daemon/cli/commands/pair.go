package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"quava-cli/models"
	"quava-cli/utils"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var pairTimeout time.Duration

var Pair = &cobra.Command{
	Use:   "pair <peer-device-id>",
	Short: "Pair with a peer device.",
	Long:  "Pair with a peer device by providing its device ID.",

	Args: cobra.ExactArgs(1),

	RunE: func(command *cobra.Command, args []string) error {
		return pair(args[0], pairTimeout)
	},
}

func init() {
	Pair.Flags().DurationVarP(
		&pairTimeout,
		"timeout",
		"t",
		2*time.Minute,
		"Pairing timeout",
	)
}

func pair(peerDeviceID string, timeout time.Duration) error {
	connection := utils.DaemonConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close connection: %v\n", err)
			os.Exit(1)
		}
	}()

	encoder := json.NewEncoder(connection)
	decoder := json.NewDecoder(connection)

	if err := encoder.Encode(models.Request{
		Command:      "pair",
		PeerDeviceID: peerDeviceID,
		TimeoutMS:    timeout.Milliseconds(),
	}); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	var res models.Response

	if err := decoder.Decode(&res); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if res.Error != "" {
		return fmt.Errorf("daemon returned error: %s", res.Error)
	}

	if res.Event != "pairing_code" {
		return fmt.Errorf("daemon did not provide pairing code")
	}

	fmt.Printf(
		"Pairing code for %s: %03d %03d\nConfirm pairing? [y/N]: ",
		res.PeerName,
		res.Code/1000,
		res.Code%1000,
	)

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}

	accepted := strings.EqualFold(strings.TrimSpace(line), "y") ||
		strings.EqualFold(strings.TrimSpace(line), "yes")

	if err := encoder.Encode(models.Request{
		Command: "confirm_pairing",
		Confirm: &accepted,
	}); err != nil {
		return fmt.Errorf("failed to encode confirmation: %w", err)
	}

	if err := decoder.Decode(&res); err != nil {
		return fmt.Errorf("failed to decode confirmation response: %w", err)
	}

	if res.Error != "" {
		return fmt.Errorf("daemon returned error: %s", res.Error)
	}

	utils.PrintReply(res)
	return nil
}
