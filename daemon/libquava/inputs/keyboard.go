package inputs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"libquava/models"
)

type Backend string

const (
	BackendX11     Backend = "x11"
	BackendWayland Backend = "wayland"
)

// Emulator applies validated Android input events using Linux uinput.
// The backend constants remain for compatibility with callers; input is now
// injected through kernel virtual devices and is compositor-independent.
type Emulator interface {
	Apply(context.Context, models.RemoteInput) error
	Reset(context.Context) error
	Close() error
}

type emulator struct {
	mu          sync.Mutex
	keyboard    *uinputDevice
	mouse       *uinputDevice
	heldKeys    map[uint32]bool
	heldButtons map[uint8]bool
	closed      bool
}

// New creates virtual keyboard and mouse devices. The daemon needs write
// permission on /dev/uinput (or /dev/input/uinput).
func New() (Emulator, error) {
	path := os.Getenv("QUAVA_UINPUT_PATH")
	if path == "" {
		path = "/dev/uinput"
		if _, err := os.Stat(path); err != nil {
			if _, altErr := os.Stat("/dev/input/uinput"); altErr == nil {
				path = "/dev/input/uinput"
			}
		}
	}

	keyboard, err := newKeyboardDevice(path, "Quava Virtual Keyboard")
	if err != nil {
		return nil, fmt.Errorf("remote input: create uinput keyboard: %w", err)
	}
	mouse, err := newMouseDevice(path, "Quava Virtual Mouse")
	if err != nil {
		_ = keyboard.Close()
		return nil, fmt.Errorf("remote input: create uinput mouse: %w", err)
	}
	return &emulator{
		keyboard: keyboard, mouse: mouse,
		heldKeys: make(map[uint32]bool), heldButtons: make(map[uint8]bool),
	}, nil
}

// DetectBackend is retained for compatibility with existing diagnostics.
func DetectBackend(getenv func(string) string) (Backend, error) {
	switch getenv("XDG_SESSION_TYPE") {
	case "wayland":
		return BackendWayland, nil
	case "x11":
		return BackendX11, nil
	}
	if getenv("WAYLAND_DISPLAY") != "" {
		return BackendWayland, nil
	}
	if getenv("DISPLAY") != "" {
		return BackendX11, nil
	}
	return "", errors.New("remote input: no Wayland or X11 session in environment")
}

func (e *emulator) Apply(ctx context.Context, input models.RemoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := input.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("remote input: uinput devices are closed")
	}

	var err error
	switch input.Type {
	case models.InputKey:
		log.Printf(
			"remote input: Android keycode=%d action=%d",
			input.KeyCode,
			input.Action,
		)
		code, ok := evdevKey(input.KeyCode)
		if !ok {
			return fmt.Errorf("remote input: unsupported Android key code %d", input.KeyCode)
		}
		value := int32(1)
		if input.Action == models.InputActionUp {
			value = 0
		}
		err = e.keyboard.event(evKey, uint16(code), value)
	case models.InputText:
		err = e.typeText(input.Text)
	default:
		err = e.applyMouse(input)
	}
	if err != nil {
		return err
	}
	switch input.Type {
	case models.InputKey:
		if input.Action == models.InputActionDown {
			e.heldKeys[input.KeyCode] = true
		} else {
			delete(e.heldKeys, input.KeyCode)
		}
	case models.InputPointerButton:
		if input.Action == models.InputActionDown {
			e.heldButtons[input.Button] = true
		} else {
			delete(e.heldButtons, input.Button)
		}
	}
	return nil
}

// Reset releases held keys/buttons. Call when the remote-input session ends.
func (e *emulator) Reset(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var errs []error
	for code := range e.heldKeys {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		ev, ok := evdevKey(code)
		if !ok {
			continue
		}
		if err := e.keyboard.event(evKey, uint16(ev), 0); err != nil {
			errs = append(errs, err)
		} else {
			delete(e.heldKeys, code)
		}
	}
	for button := range e.heldButtons {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		ev, ok := evdevButton(button)
		if !ok {
			continue
		}
		if err := e.mouse.event(evKey, uint16(ev), 0); err != nil {
			errs = append(errs, err)
		} else {
			delete(e.heldButtons, button)
		}
	}
	return errors.Join(errs...)
}

func (e *emulator) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	var errs []error
	if err := e.releaseAllLocked(); err != nil {
		errs = append(errs, err)
	}
	if e.mouse != nil {
		if err := e.mouse.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if e.keyboard != nil {
		if err := e.keyboard.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	e.closed = true
	return errors.Join(errs...)
}

func (e *emulator) releaseAllLocked() error {
	var errs []error
	for code := range e.heldKeys {
		if ev, ok := evdevKey(code); ok {
			if err := e.keyboard.event(evKey, uint16(ev), 0); err != nil {
				errs = append(errs, err)
			}
		}
		delete(e.heldKeys, code)
	}
	for button := range e.heldButtons {
		if ev, ok := evdevButton(button); ok {
			if err := e.mouse.event(evKey, uint16(ev), 0); err != nil {
				errs = append(errs, err)
			}
		}
		delete(e.heldButtons, button)
	}
	return errors.Join(errs...)
}

func (e *emulator) typeText(text string) error {
	if len(text) == 0 || len(text) > 4096 {
		return fmt.Errorf("remote input: text length out of range")
	}
	// uinput sends physical key events, not Unicode strings. Map printable ASCII
	// to keycodes and synthesize Shift for uppercase/shifted punctuation.
	for _, r := range text {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("remote input: raw uinput text supports printable ASCII only; Unicode needs a text/IME protocol")
		}
		code, shifted, ok := asciiKey(r)
		if !ok {
			return fmt.Errorf("remote input: cannot map text character %q to a US keyboard key", r)
		}
		if shifted {
			if err := e.keyboard.event(evKey, 42, 1); err != nil {
				return err
			}
		}
		if err := e.keyboard.event(evKey, uint16(code), 1); err != nil {
			return err
		}
		if err := e.keyboard.event(evKey, uint16(code), 0); err != nil {
			return err
		}
		if shifted {
			if err := e.keyboard.event(evKey, 42, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

var letterKeyCodes = map[rune]int{
	'a': 30, 'b': 48, 'c': 46, 'd': 32,
	'e': 18, 'f': 33, 'g': 34, 'h': 35,
	'i': 23, 'j': 36, 'k': 37, 'l': 38,
	'm': 50, 'n': 49, 'o': 24, 'p': 25,
	'q': 16, 'r': 19, 's': 31, 't': 20,
	'u': 22, 'v': 47, 'w': 17, 'x': 45,
	'y': 21, 'z': 44,
}

func asciiKey(r rune) (code int, shifted bool, ok bool) {
	if r >= 'A' && r <= 'Z' {
		code, ok := letterKeyCodes[r+('a'-'A')]
		return code, true, ok
	}
	if code, ok := letterKeyCodes[r]; ok {
		return code, false, true
	}

	if r >= '1' && r <= '9' {
		return int(r-'1') + 2, false, true
	}
	if r == '0' {
		return 11, false, true
	}

	m := map[rune]struct {
		code  int
		shift bool
	}{
		' ':  {57, false},
		'\n': {28, false},
		'\t': {15, false},
		'-':  {12, false}, '_': {12, true},
		'=': {13, false}, '+': {13, true},
		'[': {26, false}, '{': {26, true},
		']': {27, false}, '}': {27, true},
		'\\': {43, false}, '|': {43, true},
		';': {39, false}, ':': {39, true},
		'\'': {40, false}, '"': {40, true},
		'`': {41, false}, '~': {41, true},
		',': {51, false}, '<': {51, true},
		'.': {52, false}, '>': {52, true},
		'/': {53, false}, '?': {53, true},
		'!': {2, true}, '@': {3, true},
		'#': {4, true}, '$': {5, true},
		'%': {6, true}, '^': {7, true},
		'&': {8, true}, '*': {9, true},
		'(': {10, true}, ')': {11, true},
	}

	v, ok := m[r]
	return v.code, v.shift, ok
}

// Android key codes are translated here rather than accepting Linux keycodes
// from the client. The protocol remains independent of the host OS.
func evdevKey(code uint32) (int, bool) {
	v, ok := keyMap[code]
	return v.evdev, ok
}

type key struct {
	x11   string
	evdev int
}

var keyMap = map[uint32]key{
	3: {"Super_L", 125}, 4: {"Escape", 1}, 19: {"Up", 103}, 20: {"Down", 108}, 21: {"Left", 105}, 22: {"Right", 106}, 23: {"Return", 28}, 24: {"XF86AudioRaiseVolume", 115}, 25: {"XF86AudioLowerVolume", 114}, 26: {"XF86PowerOff", 116},
	29: {"a", 30}, 30: {"b", 48}, 31: {"c", 46}, 32: {"d", 32}, 33: {"e", 18}, 34: {"f", 33}, 35: {"g", 34}, 36: {"h", 35}, 37: {"i", 23}, 38: {"j", 36}, 39: {"k", 37}, 40: {"l", 38}, 41: {"m", 50}, 42: {"n", 49}, 43: {"o", 24}, 44: {"p", 25}, 45: {"q", 16}, 46: {"r", 19}, 47: {"s", 31}, 48: {"t", 20}, 49: {"u", 22}, 50: {"v", 47}, 51: {"w", 17}, 52: {"x", 45}, 53: {"y", 21}, 54: {"z", 44},
	55: {"comma", 51}, 56: {"period", 52}, 57: {"Alt_L", 56}, 58: {"Shift_L", 42}, 59: {"Tab", 15}, 61: {"space", 57}, 62: {"space", 57}, 66: {"Return", 28}, 67: {"BackSpace", 14}, 68: {"grave", 41}, 69: {"minus", 12}, 70: {"equal", 13}, 71: {"bracketleft", 26}, 72: {"bracketright", 27}, 73: {"backslash", 43}, 74: {"semicolon", 39}, 75: {"apostrophe", 40}, 76: {"slash", 53}, 82: {"Menu", 139}, 92: {"Page_Up", 104}, 93: {"Page_Down", 109}, 111: {"Escape", 1}, 112: {"Delete", 111}, 113: {"Control_L", 29}, 114: {"Control_R", 97}, 115: {"Caps_Lock", 58}, 116: {"Scroll_Lock", 70}, 120: {"Print", 99}, 121: {"Pause", 119}, 122: {"Home", 102}, 123: {"End", 107}, 124: {"Insert", 110},
	131: {"F1", 59}, 132: {"F2", 60}, 133: {"F3", 61}, 134: {"F4", 62}, 135: {"F5", 63}, 136: {"F6", 64}, 137: {"F7", 65}, 138: {"F8", 66}, 139: {"F9", 67}, 140: {"F10", 68}, 141: {"F11", 87}, 142: {"F12", 88},
	144: {"KP_0", 82}, 145: {"KP_1", 79}, 146: {"KP_2", 80}, 147: {"KP_3", 81}, 148: {"KP_4", 75}, 149: {"KP_5", 76}, 150: {"KP_6", 77}, 151: {"KP_7", 71}, 152: {"KP_8", 72}, 153: {"KP_9", 73}, 154: {"KP_Divide", 98}, 155: {"KP_Multiply", 55}, 156: {"KP_Subtract", 74}, 157: {"KP_Add", 78}, 158: {"KP_Decimal", 83}, 160: {"KP_Enter", 96},
	7: {"0", 11}, 8: {"1", 2}, 9: {"2", 3}, 10: {"3", 4}, 11: {"4", 5}, 12: {"5", 6}, 13: {"6", 7}, 14: {"7", 8}, 15: {"8", 9}, 16: {"9", 10},
}
