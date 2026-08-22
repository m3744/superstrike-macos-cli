# superstrike-macos-cli

**Fork of [mclol0/linux-superstrike](https://github.com/mclol0/linux-superstrike).** Ported from Linux/Fyne GUI to a pure macOS CLI.

macOS CLI for the **Logitech PRO X 2 SUPERSTRIKE**. Reads and writes device
settings directly over HID++ 2.0 via IOKit — no G HUB, no daemon, no account.
Uses [go-hid](https://github.com/sstallion/go-hid) (hidapi/IOKit bindings).

## Requirements

- macOS (Apple Silicon or Intel)
- Go 1.21+
- LIGHTSPEED USB receiver plugged in, mouse powered on
- **Input Monitoring** permission for your terminal app
  → System Settings → Privacy & Security → Input Monitoring

## Build

```sh
git clone https://github.com/m3744/superstrike-macos-cli
cd superstrike-macos-cli
go build -o superstrike-cli .
```

## Usage

### Diagnostics

```sh
# List all Logitech HID interfaces and whether they respond to HID++
./superstrike-cli --scan

# Print device info, battery, DPI, polling rate, and full HID++ feature table
./superstrike-cli --probe
```

### Read current settings

```sh
# Show active onboard profile (DPI, polling rate, button assignments)
./superstrike-cli --profile

# List all 5 profile slots
./superstrike-cli --profiles

# Show HITS (haptic trigger) config for both buttons
./superstrike-cli --hits
```

### DPI

```sh
./superstrike-cli --set-dpi 1600
./superstrike-cli --set-dpi 800
```

Range: 100–44000. Writes to the active onboard profile and applies immediately.

**Firmware quirks to be aware of:**

- The live DPI setter (HID++ `0x2202 fn6`) is a no-op on this device's firmware.
  The profile sector write is the only path that takes effect — which is also
  what G HUB does under the hood.
- The HID++ DPI getter (`fn5`) always returns a stale cached value regardless of
  what was written. `--probe` reads the current DPI from the profile sector
  directly (authoritative); `--profile` does the same.

**macOS tracking speed stacks on top of hardware DPI.** If you change DPI and
the cursor feels the same, check System Settings → Mouse → Tracking Speed — a
high OS setting can mask the hardware change. To feel a clean difference, set
the OS tracking speed to the midpoint and compare values far apart (e.g. 400 vs
3200).

### Polling rate

```sh
./superstrike-cli --set-rate 4000
./superstrike-cli --set-rate 1000
```

Supported rates: `125` / `250` / `500` / `1000` / `2000` / `4000` / `8000`.
Writes to the active profile and does a profile-bounce to apply the new rate.

### HITS — haptic inductive trigger system

```sh
# Actuation point depth: 1 (shallow) to 10 (deep)
./superstrike-cli --set-actuation-l 3 --set-actuation-r 3

# Rapid trigger sensitivity: 0 (off) to 5
./superstrike-cli --set-rt-l 2 --set-rt-r 2

# Haptic click intensity: 0 (off) to 5 (strongest)
./superstrike-cli --set-haptics-l 4 --set-haptics-r 4

# Multiple settings in one call
./superstrike-cli --set-actuation-l 4 --set-haptics-l 3 --set-rt-l 1
```

Changes apply live — no profile reload needed.

**Rapid-trigger getter quirk:** `--hits` shows the last *active* RT sensitivity
value. When RT is set to off (0), the getter returns the previous non-zero value
rather than 0 — this is a firmware limitation, not a read error. The write is
applied live regardless.

### HITS presets

Apply a named configuration in one command:

```sh
./superstrike-cli --preset superlight2
```

| Preset | Actuation | Rapid Trigger | Haptics | Description |
|--------|-----------|---------------|---------|-------------|
| `superlight2` | 5 | off | 3 | Closest match to PRO X Superlight 2 LIGHTFORCE click feel |

The `superlight2` preset matches the settings recommended by Logitech to
simulate a standard Superlight 2: mid-depth actuation, no rapid trigger
(the Superlight 2 lacks this feature), and medium haptic intensity.

### Profile management

```sh
# Switch active profile (1–5)
./superstrike-cli --switch-profile 2

# Enable / disable a profile slot
./superstrike-cli --enable-profile 2
./superstrike-cli --disable-profile 2

# Rename the active profile (max 24 chars)
./superstrike-cli --set-name "Gaming"
```

### Button remapping

Remaps apply to the active onboard profile.

```sh
./superstrike-cli --remap-b4 "next-profile" --remap-b5 "vol-up"
./superstrike-cli --remap-b3 "ctrl+c"
./superstrike-cli --remap-b3 "ctrl+shift+z"
./superstrike-cli --remap-b4 "back"       # restore default
```

**Button numbers:**

| # | Default |
|---|---------|
| 1 | Left click |
| 2 | Right click |
| 3 | Middle click |
| 4 | Back |
| 5 | Forward |

**Action syntax:**

| Category | Values |
|----------|--------|
| Mouse | `left` `right` `middle` `back` `forward` |
| Function | `next-dpi` `prev-dpi` `cycle-dpi` `default-dpi` `dpi-shift` `next-profile` `prev-profile` `cycle-profile` `battery-status` |
| Media | `play-pause` `next-track` `prev-track` `stop` `mute` `vol-up` `vol-down` |
| Key | `a`–`z`  `0`–`9`  `f1`–`f12`  `space`  `enter`  `tab`  `escape`  `backspace`  `delete`  `up arrow`  `down arrow`  `left arrow`  `right arrow`  `home`  `end`  `page up`  `page down` |
| Key + modifier | `ctrl+c`  `shift+f5`  `ctrl+shift+z`  (modifiers: `ctrl` `shift` `alt` `super`) |
| Disable | `disabled` |

## Safety

Settings write to the mouse's onboard profile memory, not to firmware flash.

- **DPI and HITS** writes are always reversible — run the command again with the
  old value or open G HUB to restore defaults.
- **Profile writes** (buttons, rate, name) read the current sector first, patch
  only the bytes that change, recompute the CRC, write back, then verify.
  A rejected write leaves the slot unchanged.
- **Never touched:** `0x1802 DeviceReset`, `0x9403 FlashUpdate`, or any
  manufacturing / firmware features. The physical HITS mechanism on buttons 1
  and 2 is unaffected by remapping.
- G HUB (Windows/macOS) can restore factory defaults if needed.

## How it works

The mouse ignores the standard live DPI/rate setter calls in firmware — the only
path that takes effect is editing the **onboard profile** stored on the mouse,
which is what G HUB does under the hood. This tool:

- speaks HID++ 2.0 directly over IOKit via go-hid (no external daemon);
- reads/writes profile sectors (DPI, report rate, buttons, name), recomputes the
  CRC-16/CCITT-FALSE checksum, and verifies the write;
- discovers the device via `hid.Enumerate` filtering on the Logitech vendor page
  (`usagePage=0xFF00, usage=0x0001`), the private HID++ channel on the
  LIGHTSPEED receiver.

For the full protocol details — feature table, DPI/rate encodings, onboard
profile sector layout, haptics, button format — see
[**`REVERSE_ENGINEERING.md`**](REVERSE_ENGINEERING.md).

## Credits

Protocol details are from [mclol0/linux-superstrike](https://github.com/mclol0/linux-superstrike)
and cross-referenced with [Solaar](https://github.com/pwr-Solaar/Solaar) and
[libratbag](https://github.com/libratbag/libratbag).

Unofficial community tool, not affiliated with or endorsed by Logitech.

## License

[MIT](LICENSE)
