package inputs

// Inspired from the archive: https://github.com/bendahl/uinput

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	evSyn = 0
	evKey = 1
	evRel = 2

	synReport = 0
	relX      = 0
	relY      = 1
	relHWheel = 6
	relWheel  = 8

	uiSetEvBit   = 0x40045564
	uiSetKeyBit  = 0x40045565
	uiSetRelBit  = 0x40045566
	uiDevCreate  = 0x5501
	uiDevDestroy = 0x5502
	uiDevSetup   = 0x405c5503

	busUSB = 0x03
)

type inputEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

type inputID struct {
	BusType uint16
	Vendor  uint16
	Product uint16
	Version uint16
}

type uinputSetup struct {
	Name         [80]byte
	ID           inputID
	FFEffectsMax uint32
}

type uinputDevice struct {
	file *os.File
}

func ioctl(fd uintptr, request uintptr, arg uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, arg)
	if errno != 0 {
		return errno
	}
	return nil
}

func newKeyboardDevice(path, name string) (*uinputDevice, error) {
	d, err := openUinput(path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*uinputDevice, error) { _ = d.file.Close(); return nil, err }
	for _, code := range []uintptr{uintptr(evKey)} {
		if err := ioctl(d.file.Fd(), uiSetEvBit, code); err != nil {
			return fail(err)
		}
	}
	for _, item := range keyMap {
		if err := ioctl(d.file.Fd(), uiSetKeyBit, uintptr(item.evdev)); err != nil {
			return fail(fmt.Errorf("enable key %d: %w", item.evdev, err))
		}
	}
	// Extra keys used by ASCII text and common modifier combinations.
	for _, code := range []uintptr{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58} {
		if err := ioctl(d.file.Fd(), uiSetKeyBit, code); err != nil {
			return fail(err)
		}
	}
	if err := d.create(name); err != nil {
		return fail(err)
	}
	return d, nil
}

func newMouseDevice(path, name string) (*uinputDevice, error) {
	d, err := openUinput(path)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*uinputDevice, error) { _ = d.file.Close(); return nil, err }
	for _, ev := range []uintptr{uintptr(evKey), uintptr(evRel)} {
		if err := ioctl(d.file.Fd(), uiSetEvBit, ev); err != nil {
			return fail(err)
		}
	}
	for _, code := range []uintptr{272, 273, 274, 275, 276} {
		if err := ioctl(d.file.Fd(), uiSetKeyBit, code); err != nil {
			return fail(err)
		}
	}
	for _, code := range []uintptr{relX, relY, relWheel, relHWheel} {
		if err := ioctl(d.file.Fd(), uiSetRelBit, code); err != nil {
			return fail(err)
		}
	}
	if err := d.create(name); err != nil {
		return fail(err)
	}
	return d, nil
}

func openUinput(path string) (*uinputDevice, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s (check uinput permissions/module): %w", path, err)
	}
	return &uinputDevice{file: f}, nil
}

func (d *uinputDevice) create(name string) error {
	var setup uinputSetup
	copy(setup.Name[:], []byte(name))
	setup.ID = inputID{BusType: busUSB, Vendor: 0x1209, Product: 0x5156, Version: 1}
	if err := ioctl(d.file.Fd(), uiDevSetup, uintptr(unsafe.Pointer(&setup))); err != nil {
		// UI_DEV_SETUP is available on modern Linux. Do not silently fall back to
		// the legacy struct because its ABI is different and error-prone.
		return fmt.Errorf("UI_DEV_SETUP: %w", err)
	}
	if err := ioctl(d.file.Fd(), uiDevCreate, 0); err != nil {
		return fmt.Errorf("UI_DEV_CREATE: %w", err)
	}
	return nil
}

func (d *uinputDevice) event(typ uint16, code uint16, value int32) error {
	ev := inputEvent{Type: typ, Code: code, Value: value}
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(ev.Sec))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(ev.Usec))
	binary.LittleEndian.PutUint16(buf[16:18], ev.Type)
	binary.LittleEndian.PutUint16(buf[18:20], ev.Code)
	binary.LittleEndian.PutUint32(buf[20:24], uint32(ev.Value))
	n, err := d.file.Write(buf)
	if err != nil {
		return fmt.Errorf("write input event: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("write input event: short write %d/%d", n, len(buf))
	}
	return d.sync()
}

func (d *uinputDevice) sync() error {
	ev := inputEvent{Type: evSyn, Code: synReport}
	buf := make([]byte, 24)
	binary.LittleEndian.PutUint16(buf[16:18], ev.Type)
	binary.LittleEndian.PutUint16(buf[18:20], ev.Code)
	n, err := d.file.Write(buf)
	if err != nil {
		return fmt.Errorf("write SYN_REPORT: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("write SYN_REPORT: short write %d/%d", n, len(buf))
	}
	return nil
}

func (d *uinputDevice) Close() error {
	if d == nil || d.file == nil {
		return nil
	}
	var errs []error
	if err := ioctl(d.file.Fd(), uiDevDestroy, 0); err != nil && err != syscall.EINVAL {
		errs = append(errs, err)
	}
	if err := d.file.Close(); err != nil {
		errs = append(errs, err)
	}
	d.file = nil
	if len(errs) != 0 {
		return fmt.Errorf("destroy uinput device: %v", errs)
	}
	return nil
}
