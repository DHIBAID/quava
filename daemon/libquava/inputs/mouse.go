package inputs

import (
	"fmt"
	"math"

	"libquava/models"
)

func (e *emulator) applyMouse(input models.RemoteInput) error {
	switch input.Type {
	case models.InputPointerMove:
		if err := e.mouse.event(evRel, relX, int32(math.Round(input.X))); err != nil {
			return err
		}
		if err := e.mouse.event(evRel, relY, int32(math.Round(input.Y))); err != nil {
			return err
		}
		return e.mouse.sync()
	case models.InputPointerButton:
		code, ok := evdevButton(input.Button)
		if !ok {
			return fmt.Errorf("remote input: unsupported mouse button %d", input.Button)
		}
		value := int32(1)
		if input.Action == models.InputActionUp {
			value = 0
		}
		return e.mouse.event(evKey, uint16(code), value)
	case models.InputScroll:
		// Keep wheel units as discrete detents; REL_WHEEL positive is up and
		// REL_HWHEEL positive is right under Linux input-event conventions.
		if input.Y != 0 {
			if err := e.mouse.event(evRel, relWheel, int32(math.Round(input.Y))); err != nil {
				return err
			}
		}
		if input.X != 0 {
			if err := e.mouse.event(evRel, relHWheel, int32(math.Round(input.X))); err != nil {
				return err
			}
		}
		return e.mouse.sync()
	default:
		return nil
	}
}

func evdevButton(button uint8) (int, bool) {
	switch button {
	case 1:
		return 272, true // BTN_LEFT
	case 2:
		return 274, true // BTN_MIDDLE
	case 3:
		return 273, true // BTN_RIGHT
	case 4:
		return 275, true // BTN_SIDE
	case 5:
		return 276, true // BTN_EXTRA
	default:
		return 0, false
	}
}
