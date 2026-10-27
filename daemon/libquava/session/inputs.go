package session

import (
	"context"
	"fmt"
	"log"
	"math"

	"libquava/models"
	"libquava/protocol"
)

// SetRemoteInputHandler installs the host-side input emulator. The handler is
// called for each unsolicited InputEvent received from the trusted peer.
func (s *Session) SetRemoteInputHandler(handler func(context.Context, models.RemoteInput) error) {
	s.remoteInput = handler
}

func (s *Session) handleRemoteInput(ctx context.Context, msg protocol.Message) error {
	input, err := remoteInputFromPayload(msg.Payload)
	if err == nil {
		err = s.applyRemoteInput(ctx, input)
	}
	if writeErr := s.ackInput(ctx, msg, err == nil); writeErr != nil {
		return fmt.Errorf("send input acknowledgement: %w", writeErr)
	}
	return err
}

func (s *Session) applyRemoteInput(ctx context.Context, input models.RemoteInput) error {
	if s.remoteInput == nil {
		return fmt.Errorf("remote input is unavailable")
	}
	return s.remoteInput(ctx, input)
}

func (s *Session) ackInput(ctx context.Context, msg protocol.Message, accepted bool) error {
	ack := protocol.NewMessage(protocol.MessageTypeInputEventAck, msg.TransactionID, map[uint64]any{0: accepted})
	s.pingMutex.Lock()
	err := s.conn.WriteMessage(ctx, ack)
	s.pingMutex.Unlock()
	return err
}

func (s *Session) runInputWorker(ctx context.Context) {
	var pending *inputJob
	for {
		var job inputJob
		if pending != nil {
			job = *pending
			pending = nil
		} else {
			select {
			case <-ctx.Done():
				return
			case job = <-s.inputQueue:
			}
		}

		input, err := remoteInputFromPayload(job.msg.Payload)
		if err != nil || input.Type != models.InputPointerMove {
			if err := s.handleRemoteInput(job.ctx, job.msg); err != nil {
				log.Printf("session: handle remote input failed: %v", err)
			}
			continue
		}

		// Coalesce queued pointer movements to avoid launching an OS input
		// operation for every touch sample. Stop at the first non-movement event
		// so button and keyboard ordering is preserved.
		jobs := []inputJob{job}
		dx, dy := input.X, input.Y
	coalesce:
		for {
			select {
			case next := <-s.inputQueue:
				nextInput, parseErr := remoteInputFromPayload(next.msg.Payload)
				if parseErr != nil || nextInput.Type != models.InputPointerMove {
					pending = &next
					break coalesce
				}
				jobs = append(jobs, next)
				dx += nextInput.X
				dy += nextInput.Y
			default:
				break coalesce
			}
		}
		combined := input
		combined.X = math.Max(-10000, math.Min(10000, dx))
		combined.Y = math.Max(-10000, math.Min(10000, dy))
		var applyErr error
		if combined.X != 0 || combined.Y != 0 {
			applyErr = s.applyRemoteInput(job.ctx, combined)
		}
		for _, item := range jobs {
			if ackErr := s.ackInput(item.ctx, item.msg, applyErr == nil); ackErr != nil {
				log.Printf("session: send coalesced input acknowledgement failed: %v", ackErr)
			}
		}
		if applyErr != nil {
			log.Printf("session: handle coalesced pointer movement failed: %v", applyErr)
		}
	}
}

// InputEvent payload schema: 0 type, 1 action, 2 Android key code, 3 pointer
// button, 4 relative X, 5 relative Y, 6 UTF-8 text. Unused fields are omitted.
func remoteInputFromPayload(payload map[uint64]any) (models.RemoteInput, error) {
	typeValue, err := protocol.PayloadUint(payload, 0)
	if err != nil {
		return models.RemoteInput{}, err
	}
	actionValue, err := optionalUint(payload, 1)
	if err != nil {
		return models.RemoteInput{}, err
	}
	keyCode, err := optionalUint(payload, 2)
	if err != nil {
		return models.RemoteInput{}, err
	}
	button, err := optionalUint(payload, 3)
	if err != nil {
		return models.RemoteInput{}, err
	}
	textValue := ""
	if rawText, ok := payload[6]; ok {
		var valid bool
		textValue, valid = rawText.(string)
		if !valid {
			return models.RemoteInput{}, fmt.Errorf("invalid text field")
		}
	}
	x, err := optionalFloat(payload, 4)
	if err != nil {
		return models.RemoteInput{}, err
	}
	y, err := optionalFloat(payload, 5)
	if err != nil {
		return models.RemoteInput{}, err
	}
	if typeValue > math.MaxUint8 || actionValue > math.MaxUint8 || keyCode > math.MaxUint32 || button > math.MaxUint8 {
		return models.RemoteInput{}, fmt.Errorf("input field is out of range")
	}
	input := models.RemoteInput{Type: models.InputType(typeValue), Action: models.InputAction(actionValue), KeyCode: uint32(keyCode), Button: uint8(button), X: x, Y: y, Text: textValue}
	if err := input.Validate(); err != nil {
		return models.RemoteInput{}, err
	}
	return input, nil
}

func optionalUint(payload map[uint64]any, key uint64) (uint64, error) {
	if _, ok := payload[key]; !ok {
		return 0, nil
	}
	return protocol.PayloadUint(payload, key)
}

func optionalFloat(payload map[uint64]any, key uint64) (float64, error) {
	v, ok := payload[key]
	if !ok {
		return 0, nil
	}
	switch value := v.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			break
		}
		return value, nil
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			break
		}
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case uint32:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case int:
		return float64(value), nil
	}
	return 0, fmt.Errorf("invalid floating-point field %d", key)
}
