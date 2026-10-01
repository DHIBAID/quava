package commands

import (
	"os"
	"quava-cli/models"
	"quava-cli/utils"
)

func Devices(args []string) {
	if len(args) > 1 {
		utils.Usage()
		os.Exit(2)
	}

	req := models.Request{Command: "devices"}

	if len(args) == 1 {
		req.PeerDeviceID = args[0]
	}

	var res models.Response
	utils.Call(req, &res)
	utils.PrintReply(res)
}
