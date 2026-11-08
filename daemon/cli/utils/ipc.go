// Package utils holds the CLI's connection to the daemon and output helpers.
package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"libquava/config"
	"quava-cli/models"
)

const (
	dialTimeout  = 2 * time.Second
	replyTimeout = 15 * time.Second
)

// Dial opens the daemon's control socket.
func Dial() (net.Conn, error) {
	store, err := config.Open("")
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(store.Dir(), models.ControlSocketName))
	if err != nil {
		return nil, fmt.Errorf("cannot reach quavad: %w", err)
	}

	return connection, nil
}

// Call sends one request and returns the daemon's reply. A reply that carries
// an error comes back as a Go error holding the daemon's message unchanged.
func Call(req models.Request) (models.Response, error) {
	var res models.Response

	connection, err := Dial()
	if err != nil {
		return res, err
	}
	defer func() { _ = connection.Close() }()

	// A missing reply means the daemon is stuck, so say so rather than hang.
	if err := connection.SetDeadline(time.Now().Add(replyTimeout)); err != nil {
		return res, err
	}

	if err := json.NewEncoder(connection).Encode(req); err != nil {
		return res, fmt.Errorf("ipc: send request: %w", err)
	}

	if err := json.NewDecoder(connection).Decode(&res); err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return res, fmt.Errorf("quavad did not respond within %s (it may be stuck): see systemctl --user status quavad and journalctl --user -u quavad", replyTimeout)
		}
		return res, fmt.Errorf("ipc: read reply: %w", err)
	}

	if res.Error != "" {
		return res, errors.New(res.Error)
	}

	return res, nil
}

// PrintJSON writes value to standard output as indented JSON.
func PrintJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	return encoder.Encode(value)
}
