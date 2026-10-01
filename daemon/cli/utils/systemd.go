package utils

import (
	"context"
	"encoding/json"
	"libquava/config"
	"net"
	"os"
	"path/filepath"
	"quava-cli/models"
	"time"
)

func DaemonConnection() net.Conn {
	store, err := config.Open("")

	if err != nil {
		Fail(err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(store.Dir(), models.ControlSocketName))

	if err != nil {
		Fail("cannot reach quavad: " + err.Error())
	}

	return connection
}

func Call(req models.Request, res *models.Response) {
	connection := DaemonConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			Fail(err.Error())
		}
	}()

	if err := json.NewEncoder(connection).Encode(req); err != nil {
		Fail(err.Error())
	}

	if err := json.NewDecoder(connection).Decode(res); err != nil {
		Fail(err.Error())
	}
}

func PrintReply(res models.Response) {
	if res.Error != "" {
		Fail(res.Error)
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	switch {
	case res.Device != nil:
		_ = encoder.Encode(res.Device)

	case res.Devices != nil:
		_ = encoder.Encode(res.Devices)

	default:
		_ = encoder.Encode(res.Discovered)
	}
}
