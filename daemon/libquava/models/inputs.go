package models

import (
	"fmt"
	"math"
	"unicode"
	"unicode/utf8"
)

// InputType identifies the device action sent by the Android client.
type InputType uint8

const (
	InputKey InputType = iota + 1
	InputPointerMove
	InputPointerButton
	InputScroll
	InputText
)

// InputAction is used by key and pointer-button input. Scroll and pointer move
// events use InputActionNone.
type InputAction uint8

const (
	InputActionNone InputAction = iota
	InputActionDown
	InputActionUp
)

// RemoteInput is one input event. KeyCode uses Android's KeyEvent key-code
// values; the daemon translates those values to the active compositor backend.
// X and Y are relative pointer/scroll deltas, never absolute screen positions.
type RemoteInput struct {
	Type    InputType
	Action  InputAction
	KeyCode uint32
	Button  uint8
	X       float64
	Y       float64
	Text    string
}

// Validate rejects malformed or potentially surprising remote input before it
// reaches an input-emulation program.
func (i RemoteInput) Validate() error {
	if math.IsNaN(i.X) || math.IsNaN(i.Y) || math.IsInf(i.X, 0) || math.IsInf(i.Y, 0) || math.Abs(i.X) > 10000 || math.Abs(i.Y) > 10000 {
		return fmt.Errorf("invalid input delta")
	}
	switch i.Type {
	case InputKey:
		if !supportedAndroidKey(i.KeyCode) || (i.Action != InputActionDown && i.Action != InputActionUp) {
			return fmt.Errorf("unsupported or invalid key input")
		}
	case InputPointerMove:
		if i.Action != InputActionNone || (i.X == 0 && i.Y == 0) {
			return fmt.Errorf("invalid pointer move")
		}
	case InputPointerButton:
		if i.Button < 1 || i.Button > 5 || i.Action != InputActionDown && i.Action != InputActionUp {
			return fmt.Errorf("invalid pointer button")
		}
	case InputScroll:
		if i.Action != InputActionNone || (i.X == 0 && i.Y == 0) {
			return fmt.Errorf("invalid scroll")
		}
	case InputText:
		if i.Action != InputActionNone || i.Text == "" || len(i.Text) > 4096 || !utf8.ValidString(i.Text) {
			return fmt.Errorf("invalid text input")
		}
		for _, r := range i.Text {
			if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
				return fmt.Errorf("text input contains control characters")
			}
		}
	default:
		return fmt.Errorf("unknown input type %d", i.Type)
	}
	return nil
}

// supportedAndroidKey intentionally excludes modifier, system, and power keys.
// Normal Android IME output must use InputText instead of synthesizing physical
// letter key presses, which are layout-dependent.
func supportedAndroidKey(code uint32) bool {
	switch code {
	case 4, 19, 20, 21, 22, 23, 61, 62, 66, 67, 111, 112:
		return true
	default:
		return false
	}
}
