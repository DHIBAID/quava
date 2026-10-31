package discovery

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// A paired phone that is not on the network is asked for about once a
	// minute at most, after a short burst at the start.
	queryBackoffStart = 3 * time.Second
	queryBackoffMax   = 60 * time.Second

	// Never send two multicast queries closer together than this, however many
	// sessions are waiting.
	minQueryGap = 2 * time.Second

	// How often the watcher checks that its socket still matches the machine's
	// current network (Wi-Fi reconnects, docking, VPNs).
	interfaceCheckEvery = 5 * time.Second
)

// Watcher keeps one multicast socket open for the life of the daemon and
// remembers every Quava device it hears. Like KDE Connect, it mostly listens:
// a phone announces itself when it joins the network, and that announcement
// is enough to connect. A query is only sent while some paired device is still
// missing, and then with exponential backoff, so a phone that is switched off
// costs about one packet a minute instead of constant discovery traffic.
type Watcher struct {
	mu        sync.Mutex
	client    *Client
	seen      map[string]Device
	changed   chan struct{} // closed and replaced whenever something new is learned
	lastQuery time.Time
	query     []byte
}

// NewWatcher builds a Watcher. Call Start to begin listening.
func NewWatcher() (*Watcher, error) {
	query, err := BuildPTRQuery("_quava._udp.local.")
	if err != nil {
		return nil, err
	}

	return &Watcher{
		seen:    make(map[string]Device),
		changed: make(chan struct{}),
		query:   query,
	}, nil
}

// Start listens in the background until ctx is cancelled. It never fails: if
// there is no usable network yet, it keeps checking and starts listening once
// there is.
func (w *Watcher) Start(ctx context.Context) {
	go w.run(ctx)
}

func (w *Watcher) run(ctx context.Context) {
	for ctx.Err() == nil {
		iface, key, err := currentInterface()
		if err != nil {
			sleep(ctx, interfaceCheckEvery)
			continue
		}

		client, err := newClientOn(iface)
		if err != nil {
			log.Printf("discovery: listen on %s: %v", iface.Name, err)
			sleep(ctx, interfaceCheckEvery)
			continue
		}

		w.setClient(client)
		w.broadcast()
		w.ask() // one query per network, so devices already online answer at once
		w.listen(ctx, client, key)
		w.setClient(nil)
		_ = client.Close()
	}
}

// listen reads announcements until the context ends, the socket fails, or the
// machine moves to a different network.
func (w *Watcher) listen(ctx context.Context, client *Client, key string) {
	records := newCollector()
	buf := make([]byte, 4096)
	nextCheck := time.Now().Add(interfaceCheckEvery)

	for ctx.Err() == nil {
		if time.Now().After(nextCheck) {
			if _, current, err := currentInterface(); err != nil || current != key {
				return
			}
			nextCheck = time.Now().Add(interfaceCheckEvery)
		}

		if err := client.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return
		}

		n, _, err := client.conn.ReadFromUDP(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return
		}

		for _, device := range records.ingest(buf[:n]) {
			if device.PeerDeviceID != "" && device.Address != "" {
				w.remember(device)
			}
		}
	}
}

func (w *Watcher) remember(device Device) {
	id := strings.ToLower(device.PeerDeviceID)

	w.mu.Lock()
	previous, known := w.seen[id]
	w.seen[id] = device
	w.mu.Unlock()

	if !known || previous.Address != device.Address || previous.Port != device.Port {
		w.broadcast()
	}
}

func (w *Watcher) broadcast() {
	w.mu.Lock()
	close(w.changed)
	w.changed = make(chan struct{})
	w.mu.Unlock()
}

func (w *Watcher) setClient(client *Client) {
	w.mu.Lock()
	w.client = client
	w.mu.Unlock()
}

// ask sends one multicast query, unless one went out very recently.
func (w *Watcher) ask() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client == nil || time.Since(w.lastQuery) < minQueryGap {
		return
	}

	if _, err := w.client.conn.WriteToUDP(w.query, w.client.multicast); err == nil {
		w.lastQuery = time.Now()
	}
}

// Find returns the device with the given ID as soon as it has been heard. It
// answers immediately from what is already known; otherwise it waits, asking
// now and then with growing gaps, until the device shows up or ctx ends.
func (w *Watcher) Find(ctx context.Context, id string) (Device, error) {
	id = strings.ToLower(strings.TrimSpace(id))

	backoff := queryBackoffStart
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		w.mu.Lock()
		device, ok := w.seen[id]
		changed := w.changed
		w.mu.Unlock()

		if ok {
			return device, nil
		}

		select {
		case <-ctx.Done():
			return Device{}, ctx.Err()

		case <-changed:
			// Something new was heard; look again without asking.

		case <-timer.C:
			w.ask()
			timer.Reset(backoff)
			backoff = min(backoff*2, queryBackoffMax)
		}
	}
}

// Forget drops what is known about a device, for example after a connection to
// its last address failed. The next Find waits for a fresh announcement.
func (w *Watcher) Forget(id string) {
	w.mu.Lock()
	delete(w.seen, strings.ToLower(strings.TrimSpace(id)))
	w.mu.Unlock()
}

// currentInterface picks the interface mDNS should use and returns a key that
// changes whenever that interface or its address does.
func currentInterface() (*net.Interface, string, error) {
	iface, err := findMDNSInterface()
	if err != nil {
		return nil, "", err
	}

	var key strings.Builder
	key.WriteString(iface.Name)

	if addrs, err := iface.Addrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				key.WriteString("|" + ipnet.IP.String())
			}
		}
	}

	return iface, key.String(), nil
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
