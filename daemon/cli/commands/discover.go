package commands

import (
	"os"
	"quava-cli/models"
	"quava-cli/utils"
	"time"
)

func Discover(args []string) {
	timeout, rest := utils.ParseTimeout("discover", args, 10*time.Second)

	if len(rest) != 0 {
		utils.Usage()
		os.Exit(2)
	}

	var res models.Response

	utils.Call(models.Request{Command: "discover", TimeoutMS: timeout.Milliseconds()}, &res)
	utils.PrintReply(res)
}
