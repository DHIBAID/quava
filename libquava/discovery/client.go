package discovery

import (
	"context"
	"errors"
	"fmt"
	"libquava/models"
	"net"
	"sort"
	"strings"
	"time"
)

// Device is a discovered Quava service on the local network.
type Device struct {
	Name    string
	Host    string
	Address string
	Port    int
}

// Client performs minimal mDNS/DNS-SD discovery for Quava services.
type Client struct {
	conn      *net.UDPConn
	multicast *net.UDPAddr
}

// NewClient creates a UDP4 multicast socket bound to the mDNS group.
func NewClient() (*Client, error) {
	iface, err := findMDNSInterface()
	if err != nil {
		return nil, err
	}

	mgroup := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	conn, err := net.ListenMulticastUDP("udp4", iface, mgroup)
	if err != nil {
		return nil, err
	}
	if err := conn.SetReadBuffer(64 << 10); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Client{conn: conn, multicast: mgroup}, nil
}

// Discover sends a PTR query and listens until ctx is cancelled.
func (c *Client) Discover(ctx context.Context) ([]Device, error) {
	return c.discover(ctx, "")
}

// Find returns the first fully assembled device whose name matches targetName.
func (c *Client) Find(ctx context.Context, targetName string) (Device, error) {
	devices, err := c.discover(ctx, targetName)
	if err != nil {
		return Device{}, err
	}
	if len(devices) == 0 {
		return Device{}, fmt.Errorf("discovery: device %q not found", targetName)
	}
	return devices[0], nil
}

func (c *Client) discover(ctx context.Context, targetName string) ([]Device, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("discovery: nil client")
	}

	query, err := BuildPTRQuery("_quava._udp.local.")
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.WriteToUDP(query, c.multicast); err != nil {
		return nil, err
	}

	ptrTargets := map[string]struct{}{}
	srvByName := map[string]models.SRVRecord{}
	addressByHost := map[string]string{}
	devicesByKey := map[string]Device{}

	for {
		if ctx.Err() != nil {
			return deviceListFromMaps(devicesByKey), ctx.Err()
		}

		if err := c.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			return deviceListFromMaps(devicesByKey), err
		}

		buf := make([]byte, 4096)
		n, _, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return deviceListFromMaps(devicesByKey), ctx.Err()
			}
			return deviceListFromMaps(devicesByKey), err
		}

		packet, err := ParsePacket(buf[:n])
		if err != nil {
			continue
		}

		for _, rr := range packet.Answers {
			switch rr.Type {
			case models.TypeA:
				address, err := ParseARecord(rr)
				if err != nil {
					continue
				}
				addressByHost[address.Name] = address.Address
				for _, srv := range srvByName {
					if srv.Target == address.Name {
						if _, ok := ptrTargets[srv.Name]; ok {
							if device, added := addDevice(devicesByKey, srv.Name, srv, address.Address); added && targetName != "" && device.Name == targetName && device.Address != "" {
								return []Device{device}, nil
							}
						}
					}
				}
			case models.TypePTR:
				ptr, err := ParsePTRRecord(rr)
				if err != nil || !strings.HasSuffix(ptr.Target, "._quava._udp.local.") {
					continue
				}
				ptrTargets[ptr.Target] = struct{}{}
				if srv, ok := srvByName[ptr.Target]; ok {
					if device, added := addDevice(devicesByKey, ptr.Target, srv, addressByHost[srv.Target]); added && targetName != "" && device.Name == targetName && device.Address != "" {
						return []Device{device}, nil
					}
				}
			case models.TypeSRV:
				srv, err := ParseSRVRecord(rr)
				if err != nil || !strings.HasSuffix(srv.Name, "._quava._udp.local.") {
					continue
				}
				srvByName[srv.Name] = srv
				if _, ok := ptrTargets[srv.Name]; ok {
					if device, added := addDevice(devicesByKey, srv.Name, srv, addressByHost[srv.Target]); added && targetName != "" && device.Name == targetName && device.Address != "" {
						return []Device{device}, nil
					}
				}
			}
		}
	}

}

// Close closes the listening multicast socket.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func findMDNSInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
				continue
			}
			return &iface, nil
		}
	}

	return nil, fmt.Errorf("discovery: no suitable IPv4 multicast interface found")
}

func addDevice(mapByKey map[string]Device, instanceName string, srv models.SRVRecord, address string) (Device, bool) {
	name := strings.TrimSuffix(instanceName, "._quava._udp.local.")
	if name == "" {
		name = strings.TrimSuffix(srv.Name, "._quava._udp.local.")
	}
	if name == "" {
		name = instanceName
	}
	device := Device{Name: name, Host: srv.Target, Address: address, Port: srv.Port}
	key := fmt.Sprintf("%s|%s|%d", device.Name, device.Host, device.Port)
	if existing, ok := mapByKey[key]; ok {
		if existing.Address == "" && device.Address != "" {
			mapByKey[key] = device
			return device, true
		}
		return existing, false
	}
	mapByKey[key] = device
	return device, true
}

func deviceListFromMaps(devicesByKey map[string]Device) []Device {
	devices := make([]Device, 0, len(devicesByKey))
	for _, device := range devicesByKey {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name == devices[j].Name {
			if devices[i].Host == devices[j].Host {
				return devices[i].Port < devices[j].Port
			}
			return devices[i].Host < devices[j].Host
		}
		return devices[i].Name < devices[j].Name
	})
	return devices
}

// TODO: This prototype intentionally does not resolve hostnames to IPv4 addresses,
// and does not implement a full mDNS service cache or packet retry loop.
