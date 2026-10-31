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
	PeerDeviceID string `json:"peer_device_id"`
	Name         string
	Host         string
	Address      string
	Port         int
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

	return newClientOn(iface)
}

func newClientOn(iface *net.Interface) (*Client, error) {
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

// Find returns the first fully assembled device whose advertised peer device ID matches targetID.
func (c *Client) Find(ctx context.Context, targetID string) (Device, error) {
	devices, err := c.discover(ctx, targetID)
	if err != nil {
		return Device{}, err
	}

	if len(devices) == 0 {
		return Device{}, fmt.Errorf("discovery: peer device %q not found", targetID)
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

	records := newCollector()
	buf := make([]byte, 4096)

	// A single multicast query is easily missed (device just woke, Wi-Fi power
	// save, lost packet) and responders only answer queries, so ask again
	// periodically instead of waiting indefinitely for an unsolicited announcement.
	lastQuery := time.Now()
	for {
		if ctx.Err() != nil {
			return records.list(), ctx.Err()
		}

		if time.Since(lastQuery) >= 2*time.Second {
			if _, err := c.conn.WriteToUDP(query, c.multicast); err != nil {
				return records.list(), err
			}
			lastQuery = time.Now()
		}

		if err := c.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			return records.list(), err
		}

		n, _, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}

			if ctx.Err() != nil {
				return records.list(), ctx.Err()
			}

			return records.list(), err
		}

		for _, device := range records.ingest(buf[:n]) {
			if targetName != "" && device.PeerDeviceID == targetName && device.Address != "" {
				return []Device{device}, nil
			}
		}
	}
}

// collector assembles PTR, SRV, TXT and A records, which usually arrive in
// separate packets, into complete devices.
type collector struct {
	ptrTargets    map[string]struct{}
	srvByName     map[string]models.SRVRecord
	addressByHost map[string]string
	devicesByKey  map[string]Device
	idByName      map[string]string
}

func newCollector() *collector {
	return &collector{
		ptrTargets:    map[string]struct{}{},
		srvByName:     map[string]models.SRVRecord{},
		addressByHost: map[string]string{},
		devicesByKey:  map[string]Device{},
		idByName:      map[string]string{},
	}
}

func (c *collector) list() []Device {
	return deviceListFromMaps(c.devicesByKey)
}

// ingest parses one packet and returns the devices it added or completed.
func (c *collector) ingest(raw []byte) []Device {
	packet, err := ParsePacket(raw)
	if err != nil {
		return nil
	}

	var updated []Device

	for _, rr := range append(packet.Answers, packet.Additional...) {
		switch rr.Type {
		case models.TypeA:
			address, err := ParseARecord(rr)
			if err != nil {
				continue
			}

			c.addressByHost[address.Name] = address.Address

			for _, srv := range c.srvByName {
				if srv.Target != address.Name {
					continue
				}
				if _, ok := c.ptrTargets[srv.Name]; ok {
					if device, added := addDevice(c.devicesByKey, srv.Name, srv, address.Address, c.idByName[srv.Name]); added {
						updated = append(updated, device)
					}
				}
			}

		case models.TypePTR:
			ptr, err := ParsePTRRecord(raw, rr)
			if err != nil || !strings.HasSuffix(ptr.Target, "._quava._udp.local.") {
				continue
			}

			c.ptrTargets[ptr.Target] = struct{}{}
			if srv, ok := c.srvByName[ptr.Target]; ok {
				if device, added := addDevice(c.devicesByKey, ptr.Target, srv, c.addressByHost[srv.Target], c.idByName[ptr.Target]); added {
					updated = append(updated, device)
				}
			}

		case models.TypeSRV:
			srv, err := ParseSRVRecord(raw, rr)
			if err != nil || !strings.HasSuffix(srv.Name, "._quava._udp.local.") {
				continue
			}

			c.srvByName[srv.Name] = srv
			if _, ok := c.ptrTargets[srv.Name]; ok {
				if device, added := addDevice(c.devicesByKey, srv.Name, srv, c.addressByHost[srv.Target], c.idByName[srv.Name]); added {
					updated = append(updated, device)
				}
			}

		case models.TypeTXT:
			txt, err := ParseTXTRecord(raw, rr)
			if err != nil {
				continue
			}

			c.idByName[txt.Name] = strings.ToLower(txt.Attributes["peer_device_id"])

			for key, device := range c.devicesByKey {
				if strings.HasPrefix(key, strings.TrimSuffix(txt.Name, "._quava._udp.local.")+"|") {
					device.PeerDeviceID = c.idByName[txt.Name]
					c.devicesByKey[key] = device
					updated = append(updated, device)
				}
			}
		}
	}

	return updated
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

	return nil, errors.New("discovery: no suitable IPv4 multicast interface found")
}

func addDevice(mapByKey map[string]Device, instanceName string, srv models.SRVRecord, address, peerDeviceID string) (Device, bool) {
	name := strings.TrimSuffix(instanceName, "._quava._udp.local.")
	if name == "" {
		name = strings.TrimSuffix(srv.Name, "._quava._udp.local.")
	}

	if name == "" {
		name = instanceName
	}

	device := Device{PeerDeviceID: peerDeviceID, Name: name, Host: srv.Target, Address: address, Port: srv.Port}
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
