package ui

import (
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"golang.org/x/sys/unix"

	"superstrike/internal/hidpp"
)

// startRateMonitor measures the true polling rate by counting the mouse's HID
// input reports on a second, read-only handle to the same hidraw node (so it
// never interferes with HID++ command traffic). The rate is computed over a
// sliding ~1s window of report timestamps and shown live as the mouse moves;
// when idle (no reports) the last measured value stays on screen.
//
// This sidesteps the device's stale getReportRate register entirely — it's the
// actual rate the kernel is receiving.
func (a *App) startRateMonitor() {
	go func() {
		var (
			f      *os.File
			path   string
			stamps []time.Time
		)
		buf := make([]byte, 64)
		defer func() {
			if f != nil {
				f.Close()
			}
		}()
		for {
			// (Re)open when the connected device path changes.
			cur, _ := a.curPath.Load().(string)
			if cur != path {
				if f != nil {
					f.Close()
					f = nil
				}
				path = cur
				stamps = stamps[:0]
				if path != "" {
					f, _ = os.OpenFile(path, os.O_RDONLY, 0)
				}
			}
			if f == nil {
				time.Sleep(time.Second)
				continue
			}

			fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
			n, perr := unix.Poll(fds, 400)
			if perr != nil && perr != unix.EINTR {
				f.Close()
				f = nil
				continue
			}
			now := time.Now()
			if n > 0 {
				nr, rerr := f.Read(buf)
				if rerr != nil {
					f.Close()
					f = nil
					continue
				}
				// One hidraw read == one report. Count input reports (skip HID++
				// short/long/very-long reports, which are command replies/events).
				if nr > 0 && buf[0] != hidpp.ReportShort && buf[0] != hidpp.ReportLong && buf[0] != hidpp.ReportVeryLong {
					stamps = append(stamps, now)
				}
			}

			// Drop timestamps older than 1s.
			cutoff := now.Add(-time.Second)
			i := 0
			for i < len(stamps) && stamps[i].Before(cutoff) {
				i++
			}
			stamps = stamps[i:]

			// Estimate the rate from the MEDIAN interval between consecutive
			// reports — robust to gaps when the mouse is moved in bursts (gaps
			// are a few large outliers; the median stays at the true period).
			if len(stamps) >= 16 {
				diffs := make([]float64, 0, len(stamps)-1)
				for j := 1; j < len(stamps); j++ {
					diffs = append(diffs, stamps[j].Sub(stamps[j-1]).Seconds())
				}
				sort.Float64s(diffs)
				med := diffs[len(diffs)/2]
				if med > 0 {
					measured := nearestRate(1.0 / med)
					if a.measuredRate.Swap(int64(measured)) != int64(measured) {
						hz := measured
						fyne.Do(func() { setText(a.tRate, fmt.Sprintf("%d Hz", hz)) })
					}
				}
			}
		}
	}()
}

// nearestRate snaps a measured frequency to the closest supported polling rate.
func nearestRate(hz float64) int {
	best := hidpp.ReportRates[0]
	bestD := math.Abs(hz - float64(best))
	for _, r := range hidpp.ReportRates[1:] {
		if d := math.Abs(hz - float64(r)); d < bestD {
			bestD, best = d, r
		}
	}
	return best
}
