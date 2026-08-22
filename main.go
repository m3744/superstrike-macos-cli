// Command superstrike is a macOS CLI for the Logitech PRO X 2 Superstrike.
// It speaks HID++ 2.0 directly over IOKit — no G HUB, no daemon, one binary.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"superstrike/internal/hidpp"
)

func main() {
	probe    := flag.Bool("probe",    false, "print device info + full HID++ feature table")
	profile  := flag.Bool("profile",  false, "dump the active onboard profile (read-only)")
	profiles := flag.Bool("profiles", false, "list all profiles + control sectors (read-only)")
	scan     := flag.Bool("scan",     false, "list all Logitech HID interfaces + HID++ ping results")
	setDPI  := flag.Int("set-dpi",  0, "write DPI to the active onboard profile (100..44000)")
	setRate := flag.Int("set-rate", 0, "write polling rate Hz to the active onboard profile (125/250/500/1000/2000/4000/8000)")
	hits        := flag.Bool("hits", false, "read HITS analog button config (actuation, rapid-trigger, haptics)")
	setActL     := flag.Int("set-actuation-l", -1, "left button actuation point (1..10)")
	setActR     := flag.Int("set-actuation-r", -1, "right button actuation point (1..10)")
	setRTL      := flag.Int("set-rt-l",        -1, "left button rapid trigger (0..5)")
	setRTR      := flag.Int("set-rt-r",        -1, "right button rapid trigger (0..5)")
	setHapticsL := flag.Int("set-haptics-l",   -1, "left button haptics intensity (0..5)")
	setHapticsR := flag.Int("set-haptics-r",   -1, "right button haptics intensity (0..5)")
	switchProf  := flag.Int("switch-profile",   0, "switch active profile by slot number (1..5)")
	enableProf  := flag.Int("enable-profile",   0, "enable profile slot (1..5)")
	disableProf := flag.Int("disable-profile",  0, "disable profile slot (1..5)")
	setName     := flag.String("set-name", "", "set name of the active profile")
	remapB1     := flag.String("remap-b1", "", "remap button 1 (Left)   — see action syntax below")
	remapB2     := flag.String("remap-b2", "", "remap button 2 (Right)  — see action syntax below")
	remapB3     := flag.String("remap-b3", "", "remap button 3 (Middle) — see action syntax below")
	remapB4     := flag.String("remap-b4", "", "remap button 4 (Back)   — see action syntax below")
	remapB5     := flag.String("remap-b5", "", "remap button 5 (Forward)— see action syntax below")
	preset      := flag.String("preset", "", "apply a named HITS preset (superlight2)")
	flag.Parse()

	hitsWrite  := *setActL >= 0 || *setActR >= 0 || *setRTL >= 0 || *setRTR >= 0 || *setHapticsL >= 0 || *setHapticsR >= 0
	remapAny   := *remapB1 != "" || *remapB2 != "" || *remapB3 != "" || *remapB4 != "" || *remapB5 != ""
	presetName := strings.ToLower(strings.TrimSpace(*preset))

	switch {
	case *probe:
		runProbe()
	case *profile:
		runProfile()
	case *profiles:
		runProfiles()
	case *scan:
		runScan()
	case *setDPI != 0:
		runSetDPI(*setDPI)
	case *setRate != 0:
		runSetRate(*setRate)
	case *hits:
		runHits()
	case hitsWrite:
		runSetHits(*setActL, *setActR, *setRTL, *setRTR, *setHapticsL, *setHapticsR)
	case *switchProf != 0:
		runSwitchProfile(*switchProf)
	case *enableProf != 0:
		runSetProfileEnabled(*enableProf, true)
	case *disableProf != 0:
		runSetProfileEnabled(*disableProf, false)
	case *setName != "":
		runSetName(*setName)
	case remapAny:
		runRemap([5]string{*remapB1, *remapB2, *remapB3, *remapB4, *remapB5})
	case presetName != "":
		runPreset(presetName)
	default:
		flag.Usage()
	}
}

func openMouse() *hidpp.Device {
	devs, _, err := hidpp.Discover()
	if err != nil || len(devs) == 0 {
		fmt.Fprintln(os.Stderr, "no device found:", err)
		fmt.Fprintln(os.Stderr, "tip: grant Input Monitoring permission in System Settings → Privacy & Security")
		os.Exit(1)
	}
	return pickDevice(devs)
}

// pickDevice selects the best candidate among discovered devices,
// preferring the Superstrike by name and preferring devices that have an
// onboard profile (which the bare receiver does not).
func pickDevice(devs []*hidpp.Device) *hidpp.Device {
	pick := devs[0]
	best := -1 << 30
	for _, d := range devs {
		up := strings.ToUpper(d.Name)
		score := 0
		if strings.Contains(up, "SUPERSTRIKE") {
			score += 6
		}
		if strings.Contains(up, "RECEIVER") {
			score -= 4
		}
		if d.Has(hidpp.FeatOnboardProfile) {
			score += 4
		}
		if score > best {
			best, pick = score, d
		}
	}
	return pick
}

// runProbe prints device info and the full HID++ feature table.
func runProbe() {
	devs, perm, err := hidpp.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, "scan error:", err)
		os.Exit(1)
	}
	if len(devs) == 0 {
		if perm {
			fmt.Fprintln(os.Stderr, "permission denied — grant Input Monitoring in System Settings → Privacy & Security")
		} else {
			fmt.Fprintln(os.Stderr, "no HID++ Logitech device responded — is the mouse powered on and the receiver plugged in?")
		}
		os.Exit(1)
	}
	for _, d := range devs {
		ver, _ := d.Ping()
		name, _ := d.DeviceName()
		fmt.Printf("\n=== %s (idx 0x%02X)  product: %q  HID++ %s ===\n", d.Path, d.Index, d.Name, ver)
		if name != "" {
			fmt.Printf("  marketing name : %s\n", name)
		}
		if b, err := d.Battery(); err == nil && b.Available {
			fmt.Printf("  battery        : %d%% (charging=%v) via %s\n", b.Percent, b.Charging, b.Source)
		}
		if dpi, err := d.DPI(); err == nil {
			// getSensorDpi is documented as returning stale values on this device;
			// read the authoritative current DPI from the active profile sector instead.
			cur := dpi.Current
			if p, perr := d.ActiveProfile(); perr == nil && p.DPIX > 0 {
				cur = p.DPIX
			}
			fmt.Printf("  dpi            : current=%d range=%d-%d step=%d\n", cur, dpi.Min, dpi.Max, dpi.Step)
		}
		if cur, sup, err := d.ReportRate(); err == nil {
			fmt.Printf("  report rate    : current=%dHz supported=%v\n", cur, sup)
		}
		if feats, err := d.EnumerateFeatures(); err == nil {
			fmt.Printf("  features (%d):\n", len(feats))
			for _, f := range feats {
				fmt.Printf("    idx 0x%02X  id 0x%04X  %s\n", f.Index, f.ID, f.Name())
			}
		}
		d.Close()
	}
}

// runProfile dumps the active onboard profile (read-only).
func runProfile() {
	d := openMouse()
	defer d.Close()
	info, err := d.ProfileInfo()
	if err != nil {
		fmt.Fprintln(os.Stderr, "profile info:", err)
		os.Exit(1)
	}
	fmt.Printf("memory: sectorSize=%d count=%d buttons=%d\n", info.SectorSize, info.Count, info.Buttons)
	p, err := d.ActiveProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read profile:", err)
		os.Exit(1)
	}
	fmt.Printf("active profile sector 0x%04X  name=%q\n", p.Sector, p.Name)
	fmt.Printf("  polling rate : %d Hz\n", p.ReportRateHz)
	fmt.Printf("  DPI (X,Y)    : %d, %d\n", p.DPIX, p.DPIY)
	fmt.Printf("  RGB          : %d,%d,%d\n", p.Red, p.Green, p.Blue)
	for i := 0; i < info.Buttons && i < 16; i++ {
		fmt.Printf("  button %d     : %s\n", i+1, p.Buttons[i].Describe())
	}
}

// runProfiles lists all profile slots.
func runProfiles() {
	d := openMouse()
	defer d.Close()
	cur, _ := d.CurrentProfileSector()
	fmt.Printf("current profile sector: 0x%04X\n", cur)
	profs, err := d.Profiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "profiles:", err)
	}
	for _, p := range profs {
		active := ""
		if p.Sector == cur {
			active = " (active)"
		}
		fmt.Printf("  slot %d  sector 0x%04X  enabled=%v%s  name=%q  DPI=(%d,%d)  %dHz\n",
			p.Index, p.Sector, p.Enabled, active, p.Name, p.DPIX, p.DPIY, p.ReportRateHz)
	}
}

// runSetDPI writes DPI to the active onboard profile and verifies the write
// by reading the sector back. DPI applies live per REVERSE_ENGINEERING.md.
func runSetDPI(dpi int) {
	d := openMouse()
	defer d.Close()
	p, err := d.ActiveProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read profile:", err)
		os.Exit(1)
	}
	if err := d.WriteProfileResolution(p.Sector, dpi, dpi); err != nil {
		fmt.Fprintln(os.Stderr, "set DPI:", err)
		os.Exit(1)
	}
	// Verify the write landed by reading back from the profile sector.
	// getSensorDpi (the HID++ getter) is documented as stale on this device.
	after, err := d.ActiveProfile()
	if err == nil && after.DPIX != dpi {
		fmt.Fprintf(os.Stderr, "DPI write may not have applied: profile reads back %d (expected %d)\n", after.DPIX, dpi)
		os.Exit(1)
	}
	fmt.Printf("DPI set to %d on active profile (sector 0x%04X)\n", dpi, p.Sector)
}

// runSetRate writes polling rate to the active onboard profile. The firmware
// only loads a profile's rate on a profile switch, so SetProfileReportRateHz
// bounces through another profile automatically to make it take effect.
func runSetRate(hz int) {
	d := openMouse()
	defer d.Close()
	p, err := d.ActiveProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read profile:", err)
		os.Exit(1)
	}
	if err := d.SetProfileReportRateHz(p.Sector, hz); err != nil {
		fmt.Fprintln(os.Stderr, "set rate:", err)
		os.Exit(1)
	}
	fmt.Printf("Polling rate set to %d Hz on active profile (sector 0x%04X)\n", hz, p.Sector)
}

// runHits reads the HITS analog config for both L and R buttons.
func runHits() {
	d := openMouse()
	defer d.Close()
	caps, err := d.AnalogCaps()
	if err != nil {
		fmt.Fprintln(os.Stderr, "HITS caps:", err)
		os.Exit(1)
	}
	fmt.Printf("HITS capabilities: maxActuation=%d  maxRapidTrigger=%d  maxHaptics=%d\n",
		caps.MaxActuation, caps.MaxRapidTrigger, caps.MaxHaptics)
	for i, name := range hidpp.AnalogButtonNames {
		cfg, err := d.AnalogConfig(i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
			continue
		}
		fmt.Printf("  %-14s actuation=%d  rapid-trigger=%d  haptics=%d\n",
			name+":", cfg.Actuation, cfg.RapidTrigger, cfg.Haptics)
	}
	fmt.Println("  note: rapid-trigger getter returns last active sensitivity and may be stale when RT is off (set to 0).")
}

// runSetHits applies any HITS write flags that are >= 0. Each button gets one
// read-modify-write cycle so unspecified fields are preserved and HID++ traffic
// is kept to a minimum (2 calls per touched button instead of 2 per field).
// Changes apply live — no profile reload needed.
func runSetHits(actL, actR, rtL, rtR, hapL, hapR int) {
	d := openMouse()
	defer d.Close()

	type buttonOp struct {
		button     int
		side       string
		act, rt, hap int
	}
	for _, b := range []buttonOp{
		{0, "Left",  actL, rtL, hapL},
		{1, "Right", actR, rtR, hapR},
	} {
		if b.act < 0 && b.rt < 0 && b.hap < 0 {
			continue
		}
		if err := d.ApplyAnalogConfig(b.button, b.act, b.rt, b.hap); err != nil {
			fmt.Fprintf(os.Stderr, "set %s HITS: %v\n", b.side, err)
			os.Exit(1)
		}
		if b.act >= 0 {
			fmt.Printf("%s button actuation set to %d\n", b.side, b.act)
		}
		if b.rt >= 0 {
			fmt.Printf("%s button rapid-trigger set to %d\n", b.side, b.rt)
		}
		if b.hap >= 0 {
			fmt.Printf("%s button haptics set to %d\n", b.side, b.hap)
		}
	}
}

// runPreset applies a named HITS preset. Currently supported:
//
//	superlight2 — actuation 5, rapid-trigger off, haptics 3
//	             (closest match to the PRO X Superlight 2 LIGHTFORCE click feel)
func runPreset(name string) {
	type preset struct {
		act, rt, hap int
		desc         string
	}
	presets := map[string]preset{
		"superlight2": {5, 0, 3, "Actuation=5  Rapid-trigger=off  Haptics=3  (Superlight 2 feel)"},
	}
	p, ok := presets[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown preset %q — available: superlight2\n", name)
		os.Exit(1)
	}
	fmt.Println("Applying preset:", p.desc)
	runSetHits(p.act, p.act, p.rt, p.rt, p.hap, p.hap)
}

// runSwitchProfile switches the active onboard profile by 1-based slot number.
func runSwitchProfile(slot int) {
	d := openMouse()
	defer d.Close()
	profs, err := d.Profiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, "list profiles:", err)
		os.Exit(1)
	}
	for _, p := range profs {
		if p.Index == slot {
			if err := d.SetCurrentProfileSector(p.Sector); err != nil {
				fmt.Fprintln(os.Stderr, "switch profile:", err)
				os.Exit(1)
			}
			fmt.Printf("Switched to profile %d (sector 0x%04X)  name=%q\n", slot, p.Sector, p.Name)
			return
		}
	}
	fmt.Fprintf(os.Stderr, "profile slot %d not found\n", slot)
	os.Exit(1)
}

// runSetProfileEnabled enables or disables a profile slot by 1-based index.
func runSetProfileEnabled(slot int, enabled bool) {
	d := openMouse()
	defer d.Close()
	if err := d.SetProfileEnabled(slot, enabled); err != nil {
		fmt.Fprintln(os.Stderr, "set profile enabled:", err)
		os.Exit(1)
	}
	state := "enabled"
	if !enabled {
		state = "disabled"
	}
	fmt.Printf("Profile slot %d %s\n", slot, state)
}

// runSetName sets the active profile's name (up to 24 chars, stored UTF-16LE).
func runSetName(name string) {
	d := openMouse()
	defer d.Close()
	p, err := d.ActiveProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read profile:", err)
		os.Exit(1)
	}
	if err := d.SetProfileName(p.Sector, name); err != nil {
		fmt.Fprintln(os.Stderr, "set name:", err)
		os.Exit(1)
	}
	fmt.Printf("Profile %d name set to %q\n", p.Index, name)
}

// runRemap applies button remaps to the active profile.
// Empty strings are skipped. Action syntax:
//
//	Mouse:    left right middle back forward
//	Function: next-dpi prev-dpi cycle-dpi default-dpi dpi-shift
//	          next-profile prev-profile cycle-profile battery-status
//	Media:    play-pause next-track prev-track stop mute vol-up vol-down
//	Key:      a-z 0-9 f1-f12 space enter tab escape backspace delete
//	          (with optional modifiers: ctrl+c  shift+f5  ctrl+shift+z)
//	Disable:  disabled
func runRemap(remaps [5]string) {
	d := openMouse()
	defer d.Close()
	p, err := d.ActiveProfile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "read profile:", err)
		os.Exit(1)
	}
	for i, s := range remaps {
		if s == "" {
			continue
		}
		action, err := parseButtonAction(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "button %d: %v\n", i+1, err)
			os.Exit(1)
		}
		if err := d.SetProfileButton(p.Sector, i, action); err != nil {
			fmt.Fprintf(os.Stderr, "button %d remap: %v\n", i+1, err)
			os.Exit(1)
		}
		fmt.Printf("Button %d → %s\n", i+1, action.Describe())
	}
}

// parseButtonAction converts a user-facing action string to a ButtonAction.
func parseButtonAction(s string) (hidpp.ButtonAction, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if s == "disabled" || s == "none" || s == "disable" {
		return hidpp.ButtonAction{Kind: hidpp.ButtonDisabled}, nil
	}

	// Mouse buttons
	mouseMap := map[string]uint16{
		"left": 0x0001, "right": 0x0002, "middle": 0x0004,
		"back": 0x0008, "forward": 0x0010, "dpi-button": 0x2000,
	}
	if code, ok := mouseMap[s]; ok {
		return hidpp.ButtonAction{Kind: hidpp.ButtonMouse, Code: code}, nil
	}

	// Mouse functions — normalise name to kebab-case for matching
	normalize := func(n string) string {
		n = strings.ToLower(n)
		n = strings.ReplaceAll(n, " ", "-")
		n = strings.ReplaceAll(n, "(", "")
		n = strings.ReplaceAll(n, ")", "")
		return n
	}
	for _, c := range hidpp.FunctionChoices {
		if normalize(c.Name) == s {
			return hidpp.ButtonAction{Kind: hidpp.ButtonFunction, Code: c.Code}, nil
		}
	}

	// Media keys
	mediaMap := map[string]uint16{
		"play-pause": 0x00CD, "next-track": 0x00B5, "prev-track": 0x00B6,
		"stop": 0x00B7, "mute": 0x00E2, "vol-up": 0x00E9, "vol-down": 0x00EA,
	}
	if code, ok := mediaMap[s]; ok {
		return hidpp.ButtonAction{Kind: hidpp.ButtonConsumer, Code: code}, nil
	}

	// Keyboard key, optionally prefixed with modifiers: "ctrl+c", "shift+f5"
	parts := strings.Split(s, "+")
	var mods byte
	modMap := map[string]byte{"ctrl": 0x01, "shift": 0x02, "alt": 0x04, "super": 0x08, "cmd": 0x08}
	for _, part := range parts[:len(parts)-1] {
		m, ok := modMap[part]
		if !ok {
			return hidpp.ButtonAction{}, fmt.Errorf("unknown modifier %q", part)
		}
		mods |= m
	}
	keyStr := parts[len(parts)-1]
	for _, c := range hidpp.KeyChoices {
		if strings.ToLower(c.Name) == keyStr {
			return hidpp.ButtonAction{Kind: hidpp.ButtonKey, Mods: mods, Code: c.Code}, nil
		}
	}

	return hidpp.ButtonAction{}, fmt.Errorf("unknown action %q — run with -help to see valid actions", s)
}

// runScan lists every Logitech HID interface on the system and whether it
// answers an HID++ ping — useful for diagnosing connection and permission issues.
func runScan() {
	fmt.Println("Logitech (046d) HID interfaces:")
	results := hidpp.Scan()
	if len(results) == 0 {
		fmt.Println("  none found — is the receiver plugged in?")
		return
	}
	for _, r := range results {
		fmt.Printf("\n  %s\n  product=%q  usagePage=0x%04X  usage=0x%04X\n",
			r.Path, r.ProductStr, r.UsagePage, r.Usage)
		if r.HIDPPVer != "" {
			fmt.Printf("  HID++ %s  name=%q  <-- responds\n", r.HIDPPVer, r.MarketName)
		} else {
			fmt.Printf("  no HID++ response on idx 0x01\n")
		}
	}
}
