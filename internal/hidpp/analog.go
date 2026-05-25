package hidpp

import "fmt"

// FeatAnalogButtons (0x1B0C, ANALOG_BUTTONS) is the Superstrike's analog main
// buttons: adjustable actuation point, rapid trigger, and — the headline — the
// haptic click feedback. Decoded from Solaar's settings_templates.py.
//
// Wire encoding: actuation/rapidTrigger/haptics each pack a logical value in
// bits 7..2, i.e. wire = logical << 2. The rapid-trigger byte's bit 0 is a
// firmware-managed sensitivity flag that MUST be preserved across writes or the
// device answers INVALID_ARGUMENT.
//
//	fn0 (getCapabilities) -> [flags, buttonCount, maxAct<<2, maxRT<<2, maxHaptics<<2, ...]
//	fn2 (getConfig, 0x20)  : param[button] -> [button, act<<2, rt<<2|sensFlag, haptics<<2]
//	fn1 (setConfig, 0x10)  : [button, act<<2, rt<<2|sensFlag, haptics<<2]
const FeatAnalogButtons = 0x1B0C

// AnalogButtonNames labels the user-accessible analog buttons by index.
var AnalogButtonNames = []string{"Left Button", "Right Button"}

// AnalogCaps describes the analog-button capabilities of the device.
type AnalogCaps struct {
	Flags           byte
	Buttons         int // user-accessible count (firmware over-reports; capped at 2)
	MaxActuation    int
	MaxRapidTrigger int
	MaxHaptics      int
}

// AnalogConfig is one button's current logical settings.
type AnalogConfig struct {
	Actuation    int
	RapidTrigger int
	Haptics      int
	rtFlag       byte // preserved sensitivity flag (rapid-trigger byte bit 0)
}

func (d *Device) analogIndex() (byte, error) {
	f, err := d.FeatureIndex(FeatAnalogButtons)
	if err != nil || f.Index == 0 {
		return 0, fmt.Errorf("ANALOG_BUTTONS (0x1B0C) unavailable on this device")
	}
	return f.Index, nil
}

// AnalogCaps reads the analog-button capabilities (button count and the max
// logical value for each axis).
func (d *Device) AnalogCaps() (AnalogCaps, error) {
	idx, err := d.analogIndex()
	if err != nil {
		return AnalogCaps{}, err
	}
	r, err := d.Call(idx, 0x00)
	if err != nil {
		return AnalogCaps{}, err
	}
	if len(r) < 5 {
		return AnalogCaps{}, ErrShortRead
	}
	c := AnalogCaps{
		Flags:           r[0],
		Buttons:         int(r[1]),
		MaxActuation:    int(r[2] >> 2),
		MaxRapidTrigger: int(r[3] >> 2),
		MaxHaptics:      int(r[4] >> 2),
	}
	if c.Buttons > 2 { // firmware reports 3; only L/R are user-accessible
		c.Buttons = 2
	}
	if c.MaxActuation == 0 {
		c.MaxActuation = 10
	}
	if c.MaxRapidTrigger == 0 {
		c.MaxRapidTrigger = 5
	}
	if c.MaxHaptics == 0 {
		c.MaxHaptics = 5
	}
	return c, nil
}

// AnalogConfig reads one button's current actuation / rapid-trigger / haptics.
func (d *Device) AnalogConfig(button int) (AnalogConfig, error) {
	idx, err := d.analogIndex()
	if err != nil {
		return AnalogConfig{}, err
	}
	r, err := d.Call(idx, 0x02, byte(button))
	if err != nil {
		return AnalogConfig{}, err
	}
	if len(r) < 4 {
		return AnalogConfig{}, ErrShortRead
	}
	return AnalogConfig{
		Actuation:    int(r[1] >> 2),
		RapidTrigger: int(r[2] >> 2),
		Haptics:      int(r[3] >> 2),
		rtFlag:       r[2] & 0x01,
	}, nil
}

// writeAnalog sends a full config for a button, wire-encoding each field and
// preserving the firmware sensitivity flag on the rapid-trigger byte.
func (d *Device) writeAnalog(button int, c AnalogConfig) error {
	idx, err := d.analogIndex()
	if err != nil {
		return err
	}
	wireAct := byte((c.Actuation & 0x3F) << 2)
	wireRT := byte((c.RapidTrigger&0x3F)<<2) | (c.rtFlag & 0x01)
	wireHap := byte((c.Haptics & 0x3F) << 2)
	_, err = d.Call(idx, 0x01, byte(button), wireAct, wireRT, wireHap)
	return err
}

// SetHaptics sets the click-haptic intensity (0..MaxHaptics) for a button,
// reading first so the other fields (and the sensitivity flag) are preserved.
func (d *Device) SetHaptics(button, value int) error {
	cur, err := d.AnalogConfig(button)
	if err != nil {
		return err
	}
	cur.Haptics = value
	return d.writeAnalog(button, cur)
}

// SetActuation sets the actuation-point depth (1..MaxActuation) for a button.
func (d *Device) SetActuation(button, value int) error {
	cur, err := d.AnalogConfig(button)
	if err != nil {
		return err
	}
	cur.Actuation = value
	return d.writeAnalog(button, cur)
}

// SetRapidTrigger sets the rapid-trigger sensitivity (1..MaxRapidTrigger).
func (d *Device) SetRapidTrigger(button, value int) error {
	cur, err := d.AnalogConfig(button)
	if err != nil {
		return err
	}
	cur.RapidTrigger = value
	return d.writeAnalog(button, cur)
}
