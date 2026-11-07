package commands

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"quava-cli/models"
	"quava-cli/utils"
)

var pairTimeout time.Duration

// Pair pairs with a device found by discover.
var Pair = &cobra.Command{
	Use:   "pair <peer-device-id>",
	Short: "Pair with a device.",
	Long:  "Pair with a device by its device ID. Both screens show a code; confirm only if they match.",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return pair(args[0], pairTimeout)
	},
}

func init() {
	Pair.Flags().DurationVarP(&pairTimeout, "timeout", "t", 2*time.Minute, "how long to wait for pairing")
}

// pair talks to the daemon over one connection: it asks, the daemon replies
// with a code to verify, and we answer with the user's decision.
func pair(peerDeviceID string, timeout time.Duration) error {
	connection, err := utils.Dial()
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()

	encoder := json.NewEncoder(connection)
	decoder := json.NewDecoder(connection)

	if err := encoder.Encode(models.Request{Command: "pair", PeerDeviceID: peerDeviceID, TimeoutMS: timeout.Milliseconds()}); err != nil {
		return fmt.Errorf("pair: send request: %w", err)
	}

	var res models.Response
	if err := decoder.Decode(&res); err != nil {
		return fmt.Errorf("pair: read reply: %w", err)
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	if res.Event != "pairing_code" {
		return errors.New("the daemon did not send a pairing code")
	}

	fmt.Printf("Pairing code for %s: %03d %03d\nConfirm pairing? [y/N]: ", res.PeerName, res.Code/1000, res.Code%1000)

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("pair: read confirmation: %w", err)
	}

	answer := strings.ToLower(strings.TrimSpace(line))
	accepted := answer == "y" || answer == "yes"

	if err := encoder.Encode(models.Request{Command: "confirm_pairing", Confirm: &accepted}); err != nil {
		return fmt.Errorf("pair: send confirmation: %w", err)
	}

	res = models.Response{}
	if err := decoder.Decode(&res); err != nil {
		return fmt.Errorf("pair: read confirmation reply: %w", err)
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}

	return utils.PrintJSON(res.Device)
}
