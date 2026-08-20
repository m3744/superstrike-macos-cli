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
	flag.Parse()

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
			fmt.Printf("  dpi            : current=%d range=%d-%d step=%d\n", dpi.Current, dpi.Min, dpi.Max, dpi.Step)
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

// runSetDPI writes DPI to the active onboard profile. DPI applies live
// (no profile reload needed) per REVERSE_ENGINEERING.md.
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
