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

var Pair = &cobra.Command{
	Use:   "pair <peer-device-id> [--timeout 2m]",
	Short: "Pair with a peer device.",
	Long:  `Pair with a peer device by providing its device ID.`,

	RunE: func(command *cobra.Command, args []string) error {
		return pair(args)
	},
}

func pair(args []string) error {
	timeout, args := utils.ParseTimeout("pair", args, 2*time.Minute)
	if len(args) != 1 {
		return fmt.Errorf("invalid number of arguments")
	}

	connection := utils.DaemonConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close connection: %v\n", err)
			os.Exit(1)
		}
	}()

	encoder, decoder := json.NewEncoder(connection), json.NewDecoder(connection)
	if err := encoder.Encode(models.Request{
		Command:      "pair",
		PeerDeviceID: args[0],
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

	fmt.Printf("Pairing code for %s: %03d %03d\nConfirm pairing? [y/N]: ", res.PeerName, res.Code/1000, res.Code%1000)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	accepted := strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")

	if err := encoder.Encode(models.Request{
		Command: "confirm_pairing",
		Confirm: &accepted,
	}); err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	if err := decoder.Decode(&res); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	utils.PrintReply(res)
	return nil
}
