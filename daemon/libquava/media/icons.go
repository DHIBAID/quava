package media

import (
	"bufio"
	"bytes"
	"context"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
)

const (
	appIconSize     = 64
	appIconMaxBytes = 8 * 1024
)

// PNG icons are decoded directly; SVG-only apps are rasterized through an
// external tool when one is installed, otherwise they yield no icon (nil).
var appIconCache = struct {
	sync.Mutex
	items map[string][]byte // nil value caches a failed lookup
}{items: make(map[string][]byte)}

// AppIconPNG returns a 64x64 PNG for a player, resolved from its desktop
// entry (or, failing that, its identity) via the freedesktop icon lookup.
// Results, including misses, are cached for the process lifetime.
func AppIconPNG(desktopEntry, identity string) []byte {
	key := desktopEntry + "\x00" + identity
	appIconCache.Lock()
	if data, ok := appIconCache.items[key]; ok {
		appIconCache.Unlock()
		return data
	}
	appIconCache.Unlock()

	data := loadAppIcon(desktopEntry, identity)
	if data == nil {
		log.Printf("media: no icon found for player entry=%q identity=%q", desktopEntry, identity)
	} else {
		log.Printf("media: loaded %d-byte icon for player entry=%q identity=%q", len(data), desktopEntry, identity)
	}

	appIconCache.Lock()
	appIconCache.items[key] = data
	appIconCache.Unlock()
	return data
}

func loadAppIcon(desktopEntry, identity string) []byte {
	var names []string
	if desktopEntry != "" {
		desktopEntry = strings.TrimSuffix(desktopEntry, ".desktop")
		if icon := desktopFileIcon(desktopEntry, identity); icon != "" {
			names = append(names, icon)
		}
		names = append(names, desktopEntry)
	}
	if identity != "" {
		names = append(names, strings.ToLower(identity), strings.ReplaceAll(strings.ToLower(identity), " ", "-"))
	}

	for _, name := range names {
		if path := findIconFile(name); path != "" {
			if data := renderIcon(path); data != nil {
				return data
			}
		}
	}
	return nil
}

func dataDirs() []string {
	var dirs []string
	home, _ := os.UserHomeDir()
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		dirs = append(dirs, v)
	} else if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local/share"))
	}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local/share/flatpak/exports/share"))
	}
	dirs = append(dirs, "/var/lib/flatpak/exports/share")

	system := os.Getenv("XDG_DATA_DIRS")
	if system == "" {
		system = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(system, ":") {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// desktopFileIcon finds the player's desktop file and returns its Icon=.
// MPRIS DesktopEntry is often a short name ("spotify") while the file is a
// reverse-DNS ID ("com.spotify.Client.desktop"), so match exactly first, then
// by ID suffix, then by the file's Name= equal to the player identity.
func desktopFileIcon(entry, identity string) string {
	entry = strings.ToLower(entry)
	identity = strings.ToLower(strings.TrimSpace(identity))

	type candidate struct{ path, base string }
	var all []candidate
	for _, dir := range dataDirs() {
		files, _ := filepath.Glob(filepath.Join(dir, "applications", "*.desktop"))
		for _, f := range files {
			all = append(all, candidate{f, strings.ToLower(strings.TrimSuffix(filepath.Base(f), ".desktop"))})
		}
	}

	for _, match := range []func(c candidate) bool{
		func(c candidate) bool { return entry != "" && c.base == entry },
		func(c candidate) bool { return entry != "" && strings.HasSuffix(c.base, "."+entry) },
		func(c candidate) bool { return entry != "" && strings.Contains(c.base, "."+entry+".") },
		func(c candidate) bool { return identity != "" && readDesktopKey(c.path, "Name") == identity },
	} {
		for _, c := range all {
			if !match(c) {
				continue
			}
			if icon := readDesktopKey(c.path, "Icon"); icon != "" {
				return icon
			}
		}
	}
	return ""
}

// readDesktopKey returns a [Desktop Entry] value (lowercased for Name).
func readDesktopKey(path, key string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	inEntry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if inEntry && strings.HasPrefix(line, key+"=") {
			value := strings.TrimSpace(strings.TrimPrefix(line, key+"="))
			if key == "Name" {
				value = strings.ToLower(value)
			}
			return value
		}
	}
	// Scan stops early on a read error or an over-long line; report it rather
	// than silently treating a half-read file as "key not found".
	if err := scanner.Err(); err != nil {
		log.Printf("media: read desktop file %s: %v", path, err)
	}
	return ""
}

// findIconFile picks the PNG whose size is closest to appIconSize, preferring
// equal-or-larger so the downscale stays sharp.
func findIconFile(name string) string {
	if filepath.IsAbs(name) {
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".svg") {
			return name
		}
		return ""
	}
	if strings.ContainsAny(name, "/\\") {
		return ""
	}

	best, bestScore := "", int(^uint(0)>>1)
	consider := func(path string, size int) {
		score := size - appIconSize
		if score < 0 {
			score = -score * 4 // penalize upscaling
		}
		if score < bestScore {
			best, bestScore = path, score
		}
	}

	for _, dir := range dataDirs() {
		matches, _ := filepath.Glob(filepath.Join(dir, "icons", "*", "*", "apps", name+".png"))
		for _, path := range matches {
			// .../<theme>/<size>/apps/<name>.png where size is e.g. "64x64".
			sizeDir := filepath.Base(filepath.Dir(filepath.Dir(path)))
			if w, _, ok := strings.Cut(sizeDir, "x"); ok {
				if n, err := strconv.Atoi(w); err == nil && n > 0 {
					consider(path, n)
				}
			}
		}
		pixmap := filepath.Join(dir, "pixmaps", name+".png")
		if _, err := os.Stat(pixmap); err == nil {
			consider(pixmap, 48)
		}
	}
	if best == "" {
		// No PNG anywhere: fall back to a scalable SVG (rasterized later).
		for _, dir := range dataDirs() {
			matches, _ := filepath.Glob(filepath.Join(dir, "icons", "*", "scalable", "apps", name+".svg"))
			if len(matches) > 0 {
				return matches[0]
			}
			svg := filepath.Join(dir, "pixmaps", name+".svg")
			if _, err := os.Stat(svg); err == nil {
				return svg
			}
		}
		pixmap := filepath.Join("/usr/share/pixmaps", name+".png")
		if _, err := os.Stat(pixmap); err == nil {
			return pixmap
		}
	}
	return best
}

// loadIconImage decodes a PNG, or rasterizes an SVG to 64x64 with an
// installed rsvg-convert/magick (Go's standard library cannot render SVG).
func loadIconImage(path string) image.Image {
	if strings.HasSuffix(strings.ToLower(path), ".svg") {
		return rasterizeSVG(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	cfg, err := png.DecodeConfig(file)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > 4096*4096 {
		return nil
	}
	if _, err := file.Seek(0, 0); err != nil {
		return nil
	}
	img, err := png.Decode(file)
	if err != nil {
		return nil
	}
	return img
}

var svgRasterizers = [][]string{
	{"rsvg-convert", "-w", "64", "-h", "64", "-a", "-f", "png"},
	{"magick", "-background", "none", "-density", "384", "{in}", "-resize", "64x64", "png:-"},
	{"convert", "-background", "none", "-density", "384", "{in}", "-resize", "64x64", "png:-"},
}

func rasterizeSVG(path string) image.Image {
	for _, tool := range svgRasterizers {
		if _, err := exec.LookPath(tool[0]); err != nil {
			continue
		}
		args := make([]string, 0, len(tool))
		usedPlaceholder := false
		for _, a := range tool[1:] {
			if a == "{in}" {
				a, usedPlaceholder = path, true
			}
			args = append(args, a)
		}
		if !usedPlaceholder {
			args = append(args, path)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		out, err := exec.CommandContext(ctx, tool[0], args...).Output()
		cancel()
		if err != nil {
			log.Printf("media: %s failed on %s: %v", tool[0], path, err)
			continue
		}
		if img, err := png.Decode(bytes.NewReader(out)); err == nil {
			return img
		}
	}
	log.Printf("media: no SVG rasterizer (rsvg-convert/magick) usable for %s", path)
	return nil
}

// renderIcon scales an icon to fit 64x64 (aspect preserved, centered on a
// transparent canvas) and encodes it as PNG.
func renderIcon(path string) []byte {
	src := loadIconImage(path)
	if src == nil {
		return nil
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	scale := float64(appIconSize) / float64(max(w, h))
	dw, dh := max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))

	dst := image.NewNRGBA(image.Rect(0, 0, appIconSize, appIconSize))
	offset := image.Rect((appIconSize-dw)/2, (appIconSize-dh)/2, (appIconSize-dw)/2+dw, (appIconSize-dh)/2+dh)
	draw.CatmullRom.Scale(dst, offset, src, b, draw.Over, nil)

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, dst); err != nil || buf.Len() > appIconMaxBytes {
		return nil
	}
	return buf.Bytes()
}
