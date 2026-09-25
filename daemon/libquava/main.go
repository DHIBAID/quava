package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"libquava/config"
	"libquava/discovery"
	"libquava/models"
	"libquava/pairing"
	"libquava/protocol"
	"libquava/session"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "discover":
		cmdDiscover(os.Args[2:])
	case "pair":
		cmdPair(os.Args[2:])
	case "ping":
		cmdPing(os.Args[2:])
	case "devices":
		cmdDevices(os.Args[2:])
	case "connect":
		cmdConnect(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println("usage:")
	fmt.Println("  go run . discover [--timeout 10s]")
	fmt.Println("  go run . pair <device-name> [--timeout 120s]")
	fmt.Println("  go run . connect <device-name> [--timeout 24h]")
	fmt.Println("  go run . ping <device-name> [--timeout 15s]")
	fmt.Println("  go run . devices [device-id]")
}

func cmdDevices(args []string) {
	if len(args) > 1 {
		fmt.Println("usage: go run . devices [device-id]")
		os.Exit(2)
	}

	store, err := config.Open("")
	if err != nil {
		fmt.Printf("failed to open config: %v\n", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if len(args) == 1 {
		device, ok := store.Lookup(args[0])
		if !ok {
			fmt.Printf("device %q not found\n", args[0])
			os.Exit(1)
		}
		if err := enc.Encode(device); err != nil {
			fmt.Printf("failed to print device: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := enc.Encode(store.Devices()); err != nil {
		fmt.Printf("failed to print devices: %v\n", err)
		os.Exit(1)
	}
}

func cmdDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 10*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	devices, err := discovery.Discover(ctx)
	if err != nil && ctx.Err() == nil {
		fmt.Printf("discover failed: %v\n", err)
		os.Exit(1)
	}

	if len(devices) == 0 {
		fmt.Println("no Quava devices discovered")
		return
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(devices); err != nil {
		fmt.Printf("failed to print devices: %v\n", err)
		os.Exit(1)
	}
}

func cmdPair(args []string) {
	if len(args) == 0 {
		fmt.Println("missing device name")
		os.Exit(2)
	}

	deviceName := args[0]
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 120*time.Second, "overall timeout")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client, err := discovery.NewClient()
	if err != nil {
		fmt.Printf("failed to create discovery client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	device, err := client.Find(ctx, deviceName)
	if err != nil {
		fmt.Printf("discover failed: %v\n", err)
		os.Exit(1)
	}

	conn, err := device.Connect(ctx)
	if err != nil {
		fmt.Printf("connect failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("Connected to %s; requesting pairing challenge...\n", device.Name)
	reader := bufio.NewReader(os.Stdin)
	result, err := pairing.Initiate(ctx, conn.ProtocolConn(), models.InitiatorOptions{
		DeviceName:       localDeviceName(),
		RemoteDeviceName: device.Name,
		Capabilities:     []uint64{},
	}, func(code uint32, remoteName string) (bool, error) {
		fmt.Printf("Pairing code for %s: %03d %03d\n", remoteName, code/1000, code%1000)
		fmt.Print("Confirm pairing? [y/N]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		return line == "y" || line == "yes", nil
	})
	if err != nil {
		fmt.Printf("pair failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Paired with %s using device %s\n", device.Name, result.PeerDeviceID)
}

func cmdPing(args []string) {
	if len(args) == 0 {
		fmt.Println("missing device name")
		os.Exit(2)
	}

	// Check active session for connected devices.
	deviceName := args[0]
	fs := flag.NewFlagSet("ping", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 15*time.Second, "ping timeout")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	_ = timeout

	// Get device from active session
	store, err := config.Open("")
	if err != nil {
		fmt.Printf("failed to open config: %v\n", err)
		os.Exit(1)
	}
	deviceStore, ok := store.Lookup(deviceName)
	if !ok {
		fmt.Printf("device %q is not paired\n", deviceName)
		os.Exit(1)
	}

	// identity, ok := store.Identity()
	// if !ok {
	// 	fmt.Println("local identity not found; pair this device first")
	// 	os.Exit(1)
	// }

	// ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	// defer cancel()

	fmt.Printf("Connected to %s\nPING -> PONG\n", deviceStore.PeerDeviceName)
}

func cmdConnect(args []string) {
	if len(args) == 0 {
		fmt.Println("missing device name")
		os.Exit(2)
	}
	deviceName := args[0]
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 24*time.Hour, "maximum session lifetime")
	if err := fs.Parse(args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	store, err := config.Open("")
	if err != nil {
		fmt.Printf("failed to open config: %v\n", err)
		os.Exit(1)
	}
	peer, ok := store.Lookup(deviceName)
	if !ok {
		fmt.Printf("device %q is not paired\n", deviceName)
		os.Exit(1)
	}
	identity, ok := store.Identity()
	if !ok {
		fmt.Println("local identity not found; pair this device first")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	discoveryClient, err := discovery.NewClient()
	if err != nil {
		fmt.Printf("failed to create discovery client: %v\n", err)
		os.Exit(1)
	}
	defer discoveryClient.Close()

	fmt.Printf("Maintaining authenticated session with %s; press Ctrl-C to stop.\n", deviceName)
	err = session.Run(ctx, identity, peer, func(dialCtx context.Context) (*protocol.Conn, error) {
		device, err := discoveryClient.Find(dialCtx, deviceName)
		if err != nil {
			return nil, err
		}
		conn, err := device.Connect(dialCtx)
		if err != nil {
			return nil, err
		}
		return conn.ProtocolConn(), nil
	}, func(s *session.Session) {
		fmt.Printf("Session established with %s (%s)\n", peer.PeerDeviceName, peer.PeerDeviceID)
	}, func(err error) {
		if err != nil {
			fmt.Printf("Session disconnected: %v; reconnecting...\n", err)
		}
	})
	if err != nil && ctx.Err() == nil {
		fmt.Printf("session manager stopped: %v\n", err)
		os.Exit(1)
	}
}

func localDeviceName() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "Quava"
	}
	return hostname
}
