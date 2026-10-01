package commands

import (
	"fmt"
	"os"
	"quava-cli/models"
	"quava-cli/utils"
	"time"
)

func Ping(args []string) {
	timeout := time.Duration(0)

	if len(args) != 1 {
		utils.Usage()
		os.Exit(2)
	}

	var res models.Response
	utils.Call(models.Request{
		Command:      "ping",
		PeerDeviceID: args[0],
		TimeoutMS:    timeout.Milliseconds(),
	}, &res)

	if res.Error != "" {
		utils.Fail(res.Error)
	}

	fmt.Printf("%s requested for %s\n", "ping", args[0])
}
