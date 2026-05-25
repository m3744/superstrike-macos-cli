package hidpp

import "fmt"

// OnboardProfiles (0x8100) selects whether the mouse runs from its onboard
// memory (hardware profile owns DPI, report rate and button actions) or hands
// control to the host (software). DPI/report-rate writes via 0x2202 / 0x8061
// only take effect in Host mode — in Onboard mode the profile overrides them,
// and the device rejects live writes. Decoded from Solaar hidpp20.py /
// settings_templates.py (OnboardProfiles).
//
//	fn2 getMode (0x20) -> [mode]
//	fn1 setMode (0x10)  : [mode]
const (
	OnboardModeOnboard = 0x01 // hardware profile in control
	OnboardModeHost    = 0x02 // software (host) in control
)

// OnboardModeName renders a mode byte for display.
func OnboardModeName(mode byte) string {
	switch mode {
	case OnboardModeOnboard:
		return "Onboard (hardware profile)"
	case OnboardModeHost:
		return "Host (software control)"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", mode)
	}
}

func (d *Device) onboardIndex() (byte, error) {
	f, err := d.FeatureIndex(FeatOnboardProfile)
	if err != nil || f.Index == 0 {
		return 0, fmt.Errorf("OnboardProfiles (0x8100) unavailable")
	}
	return f.Index, nil
}

// OnboardMode reads whether the device is in Onboard or Host mode.
func (d *Device) OnboardMode() (byte, error) {
	idx, err := d.onboardIndex()
	if err != nil {
		return 0, err
	}
	r, err := d.Call(idx, 0x02)
	if err != nil {
		return 0, err
	}
	if len(r) < 1 {
		return 0, ErrShortRead
	}
	return r[0], nil
}

// SetOnboardMode switches between Onboard and Host control.
func (d *Device) SetOnboardMode(mode byte) error {
	idx, err := d.onboardIndex()
	if err != nil {
		return err
	}
	_, err = d.Call(idx, 0x01, mode)
	return err
}
