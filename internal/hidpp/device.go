package hidpp

import (
	"fmt"
	"strings"
	"time"

	hid "github.com/sstallion/go-hid"
)

const logitechVendorID = uint16(0x046D)

// Discover enumerates Logitech vendor-defined HID interfaces (usage page
// 0xFF**) and returns an open Device for each that answers a HID++ ping.
// We take only usage=0x0001 per service to avoid duplicating the same mouse
// across the two vendor interfaces a LIGHTSPEED receiver exposes.
// The permDenied return is always false on macOS — hidapi surfaces access
// errors via the returned error from Open instead.
func Discover() (devices []*Device, permDenied bool, err error) {
	var candidates []*hid.DeviceInfo
	if enumErr := hid.Enumerate(logitechVendorID, 0x0000, func(info *hid.DeviceInfo) error {
		if info.UsagePage>>8 == 0xFF && info.Usage == 0x0001 {
			candidates = append(candidates, info)
		}
		return nil
	}); enumErr != nil {
		return nil, false, enumErr
	}

	for _, info := range candidates {
		// Try receiver slot 0x01 first (LIGHTSPEED paired mouse), then 0xFF
		// (direct USB/BT), then additional slots.
		for _, idx := range []byte{0x01, 0xFF, 0x02, 0x03, 0x04, 0x05, 0x06} {
			d, oerr := Open(info.Path, idx)
			if oerr != nil {
				break
			}
			d.Timeout = 350 * time.Millisecond
			if _, perr := d.Ping(); perr != nil {
				d.Close()
				continue
			}
			d.Timeout = 4 * time.Second
			d.Name = info.ProductStr
			devices = append(devices, d)
			break
		}
	}
	return devices, false, nil
}

// ScanResult is one entry from a broad device scan across all Logitech HID
// interfaces, regardless of usage page.
type ScanResult struct {
	Path       string
	ProductStr string
	UsagePage  uint16
	Usage      uint16
	HIDPPVer   string // empty if device did not respond to HID++ ping
	MarketName string // DeviceNameType (0x0005) if available
}

// Scan enumerates every Logitech HID interface and pings each one. Useful for
// diagnosing which interface is the HID++ channel and whether the mouse is
// reachable.
func Scan() []ScanResult {
	var results []ScanResult
	_ = hid.Enumerate(logitechVendorID, 0x0000, func(info *hid.DeviceInfo) error {
		res := ScanResult{
			Path:       info.Path,
			ProductStr: info.ProductStr,
			UsagePage:  info.UsagePage,
			Usage:      info.Usage,
		}
		d, err := Open(info.Path, 0x01)
		if err != nil {
			results = append(results, res)
			return nil
		}
		d.Timeout = 400 * time.Millisecond
		if ver, perr := d.Ping(); perr == nil {
			res.HIDPPVer = ver
			res.MarketName, _ = d.DeviceName()
		}
		d.Close()
		results = append(results, res)
		return nil
	})
	return results
}

// DeviceName reads the marketing name via feature 0x0005 (DeviceNameType),
// assembling it across however many 16-byte chunks the device reports.
func (d *Device) DeviceName() (string, error) {
	f, err := d.FeatureIndex(FeatDeviceName)
	if err != nil || f.Index == 0 {
		return "", fmt.Errorf("DeviceNameType unavailable")
	}
	lenResp, err := d.Call(f.Index, 0x00) // getDeviceNameCount
	if err != nil {
		return "", err
	}
	if len(lenResp) < 1 {
		return "", ErrShortRead
	}
	total := int(lenResp[0])
	var b strings.Builder
	for off := 0; off < total; off += 15 {
		chunk, err := d.Call(f.Index, 0x01, byte(off)) // getDeviceName(offset)
		if err != nil {
			return "", err
		}
		for _, c := range chunk {
			if c == 0 {
				break
			}
			b.WriteByte(c)
		}
		if b.Len() >= total {
			break
		}
	}
	return strings.TrimRight(b.String(), "\x00 "), nil
}

// Battery holds a normalised battery reading from whichever battery feature the
// device implements.
type Battery struct {
	Percent   int
	Charging  bool
	Available bool
	Source    string // which feature reported it
}

// Battery reads charge state, preferring UnifiedBattery (0x1004) and falling
// back to BatteryUnifiedLevelStatus (0x1000).
func (d *Device) Battery() (Battery, error) {
	if f, err := d.FeatureIndex(FeatUnifiedBattery); err == nil && f.Index != 0 {
		// getStatus = func 1: [stateOfCharge, ?, status, ...]
		resp, err := d.Call(f.Index, 0x01)
		if err == nil && len(resp) >= 3 {
			return Battery{
				Percent:   int(resp[0]),
				Charging:  resp[2] == 1 || resp[2] == 3,
				Available: true,
				Source:    "UnifiedBattery(0x1004)",
			}, nil
		}
	}
	if f, err := d.FeatureIndex(FeatBatteryStatus); err == nil && f.Index != 0 {
		// getBatteryLevelStatus = func 0: [level%, nextLevel, status]
		resp, err := d.Call(f.Index, 0x00)
		if err == nil && len(resp) >= 3 {
			return Battery{
				Percent:   int(resp[0]),
				Charging:  resp[2] >= 3,
				Available: true,
				Source:    "BatteryUnifiedLevelStatus(0x1000)",
			}, nil
		}
	}
	return Battery{Available: false}, nil
}

// DPIInfo describes the adjustable-DPI capability of sensor 0.
type DPIInfo struct {
	Current int
	Default int
	Min     int
	Max     int
	Step    int
}

// dpiFeature resolves the device's DPI feature, preferring the extended
// variant (0x2202) used by current mice like the Superstrike, then falling
// back to the classic 0x2201. ext reports which one was found.
func (d *Device) dpiFeature() (idx byte, ext bool) {
	if f, err := d.FeatureIndex(FeatExtAdjDPI); err == nil && f.Index != 0 {
		return f.Index, true
	}
	if f, err := d.FeatureIndex(FeatAdjustableDPI); err == nil && f.Index != 0 {
		return f.Index, false
	}
	return 0, false
}

func be16(b []byte) int {
	if len(b) < 2 {
		return 0
	}
	return int(b[0])<<8 | int(b[1])
}

// decodeDpiList decodes a DPI capability blob into min/max/step. Values are
// 16-bit big-endian, zero-terminated. A value whose top 3 bits are set
// (val>>13 == 0b111) is a range marker: its low 13 bits are the step and the
// following word is the range's upper bound.
func decodeDpiList(b []byte) (min, max, step int) {
	for i := 0; i+1 < len(b); {
		val := int(b[i])<<8 | int(b[i+1])
		if val == 0 {
			break
		}
		if val>>13 == 0b111 { // range step marker
			step = val & 0x1FFF
			i += 2
			if i+1 < len(b) {
				if last := int(b[i])<<8 | int(b[i+1]); last > max {
					max = last
				}
				i += 2
			}
			continue
		}
		if min == 0 || val < min {
			min = val
		}
		if val > max {
			max = val
		}
		i += 2
	}
	return min, max, step
}

// readDpiList fetches and decodes the sensor-0 DPI list. The extended feature
// (0x2202) pages the list via request (sensor, direction, page) with data
// starting at byte 3; the classic feature (0x2201) returns it in one reply
// with data at byte 1.
func (d *Device) readDpiList(idx byte, ext bool) (min, max, step int) {
	var data []byte
	if ext {
		for page := 0; page < 16; page++ {
			r, err := d.Call(idx, 0x02, 0x00, 0x00, byte(page))
			if err != nil || len(r) < 4 {
				break
			}
			data = append(data, r[3:]...)
			if n := len(data); n >= 2 && data[n-2] == 0 && data[n-1] == 0 {
				break
			}
		}
	} else if r, err := d.Call(idx, 0x01, 0x00); err == nil && len(r) > 1 {
		data = r[1:]
	}
	return decodeDpiList(data)
}

// DPI reads sensor 0's current DPI and range. Function numbers differ between
// the extended (0x2202: list=fn2, get=fn5) and classic (0x2201: list=fn1,
// get=fn2) features; in both, the current DPI is a big-endian word at offset 1.
func (d *Device) DPI() (DPIInfo, error) {
	idx, ext := d.dpiFeature()
	if idx == 0 {
		return DPIInfo{}, fmt.Errorf("no adjustable-DPI feature")
	}
	var info DPIInfo
	getFn := byte(0x02)
	if ext {
		getFn = 0x05
	}
	info.Min, info.Max, info.Step = d.readDpiList(idx, ext)
	cur, err := d.Call(idx, getFn, 0x00)
	if err != nil {
		return info, err
	}
	if len(cur) >= 5 {
		info.Current = be16(cur[1:3])
		info.Default = be16(cur[3:5])
		if info.Current == 0 { // some firmware reports current in the second word
			info.Current = info.Default
		}
	}
	if info.Min == 0 {
		info.Min = 100
	}
	if info.Max == 0 {
		info.Max = 32000
	}
	if info.Step == 0 {
		info.Step = 50
	}
	return info, nil
}

// CurrentDPI reads only sensor 0's current DPI (a single call, no list paging)
// — cheap enough for the periodic dashboard refresh.
func (d *Device) CurrentDPI() (int, error) {
	idx, ext := d.dpiFeature()
	if idx == 0 {
		return 0, fmt.Errorf("no adjustable-DPI feature")
	}
	getFn := byte(0x02)
	if ext {
		getFn = 0x05
	}
	cur, err := d.Call(idx, getFn, 0x00)
	if err != nil {
		return 0, err
	}
	if len(cur) < 5 {
		return 0, ErrShortRead
	}
	v := be16(cur[1:3])
	if v == 0 {
		v = be16(cur[3:5])
	}
	return v, nil
}

// SetDPI sets sensor 0's DPI. Extended (0x2202) setSensorDpi is fn6 and takes
// X, Y and LOD words; classic (0x2201) setSensorDpi is fn3 with just X.
func (d *Device) SetDPI(dpi int) error {
	idx, ext := d.dpiFeature()
	if idx == 0 {
		return fmt.Errorf("no adjustable-DPI feature")
	}
	var err error
	if ext {
		// [sensor, dpiX_hi, dpiX_lo, dpiY_hi, dpiY_lo, lod] — Y must mirror X or
		// the device zeroes vertical movement / rejects the call.
		_, err = d.Call(idx, 0x06, 0x00, byte(dpi>>8), byte(dpi&0xFF), byte(dpi>>8), byte(dpi&0xFF), 0x00)
	} else {
		_, err = d.Call(idx, 0x03, 0x00, byte(dpi>>8), byte(dpi&0xFF))
	}
	// Some setters on this device apply without sending a HID++ reply (and the
	// getters report stale values), so a timeout is not a failure — treat it as
	// best-effort applied.
	if err == ErrTimeout {
		return nil
	}
	return err
}

// extRateHz maps the 0x8061 ExtendedAdjustableReportRate index (0..6) to Hz.
// The device's getReportRateList returns a 16-bit bitmask where bit i means
// index i is supported.
var extRateHz = []int{125, 250, 500, 1000, 2000, 4000, 8000}

// legacyRateValues maps the classic 0x8060 byte values to Hz.
var legacyRateValues = []struct {
	Value byte
	Hz    int
}{
	{8, 125}, {4, 250}, {2, 500}, {1, 1000},
}

// ReportRate reads the current polling rate in Hz, preferring the extended
// feature 0x8061 (list=fn1 16-bit mask, get=fn2 index) and falling back to the
// classic 0x8060 (list=fn0 mask, get=fn1 value).
func (d *Device) ReportRate() (current int, supported []int, err error) {
	if f, e := d.FeatureIndex(FeatExtReportRate); e == nil && f.Index != 0 {
		if list, e := d.Call(f.Index, 0x01); e == nil && len(list) >= 2 {
			mask := be16(list[0:2])
			for i, hz := range extRateHz {
				if mask&(1<<uint(i)) != 0 {
					supported = append(supported, hz)
				}
			}
		}
		cur, e := d.Call(f.Index, 0x02)
		if e != nil {
			return 0, supported, e
		}
		if len(cur) >= 1 && int(cur[0]) < len(extRateHz) {
			current = extRateHz[cur[0]]
		}
		return current, supported, nil
	}

	f, ferr := d.FeatureIndex(FeatReportRate)
	if ferr != nil || f.Index == 0 {
		return 0, nil, fmt.Errorf("no report-rate feature")
	}
	if list, e := d.Call(f.Index, 0x00); e == nil && len(list) >= 1 {
		mask := list[0]
		for _, rv := range legacyRateValues {
			if mask&rv.Value != 0 {
				supported = append(supported, rv.Hz)
			}
		}
	}
	cur, e := d.Call(f.Index, 0x01)
	if e != nil {
		return 0, supported, e
	}
	if len(cur) >= 1 {
		for _, rv := range legacyRateValues {
			if rv.Value == cur[0] {
				current = rv.Hz
			}
		}
	}
	return current, supported, nil
}

// SetReportRate sets the polling rate (Hz), preferring extended 0x8061
// (setReportRate = fn3, takes the index) then classic 0x8060 (fn2, value).
func (d *Device) SetReportRate(hz int) error {
	if f, e := d.FeatureIndex(FeatExtReportRate); e == nil && f.Index != 0 {
		for i, h := range extRateHz {
			if h == hz {
				_, err := d.Call(f.Index, 0x03, byte(i))
				if err != nil {
					if cur, _, rerr := d.ReportRate(); rerr == nil && cur == hz {
						return nil // applied despite a missing ack
					}
				}
				return err
			}
		}
		return fmt.Errorf("unsupported report rate %dHz", hz)
	}
	f, err := d.FeatureIndex(FeatReportRate)
	if err != nil || f.Index == 0 {
		return fmt.Errorf("no report-rate feature")
	}
	for _, rv := range legacyRateValues {
		if rv.Hz == hz {
			_, err = d.Call(f.Index, 0x02, rv.Value)
			return err
		}
	}
	return fmt.Errorf("unsupported report rate %dHz", hz)
}
