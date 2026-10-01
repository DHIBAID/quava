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
)

func Pair(args []string) {
	timeout, args := utils.ParseTimeout("pair", args, 2*time.Minute)
	if len(args) != 1 {
		utils.Usage()
		os.Exit(2)
	}

	connection := utils.DaemonConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			utils.Fail(err.Error())
		}
	}()

	encoder, decoder := json.NewEncoder(connection), json.NewDecoder(connection)
	if err := encoder.Encode(models.Request{
		Command:      "pair",
		PeerDeviceID: args[0],
		TimeoutMS:    timeout.Milliseconds(),
	}); err != nil {
		utils.Fail(err.Error())
	}

	var res models.Response
	if err := decoder.Decode(&res); err != nil {
		utils.Fail(err.Error())
	}

	if res.Error != "" {
		utils.Fail(res.Error)
	}

	if res.Event != "pairing_code" {
		utils.Fail("daemon did not provide pairing code")
	}

	fmt.Printf("Pairing code for %s: %03d %03d\nConfirm pairing? [y/N]: ", res.PeerName, res.Code/1000, res.Code%1000)

	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	accepted := strings.EqualFold(strings.TrimSpace(line), "y") || strings.EqualFold(strings.TrimSpace(line), "yes")

	if err := encoder.Encode(models.Request{
		Command: "confirm_pairing",
		Confirm: &accepted,
	}); err != nil {
		utils.Fail(err.Error())
	}

	if err := decoder.Decode(&res); err != nil {
		utils.Fail(err.Error())
	}

	utils.PrintReply(res)
}
