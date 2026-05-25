package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// batteryGauge is a custom battery indicator. Charging shows the fill rising
// from the current level to full in a loop; discharging shows the static fill
// with a soft glow breathing over it — so the two states read differently and
// neither looks like the other. The percentage text is overlaid separately by
// the dashboard (centered), so this widget draws no text.
type batteryGauge struct {
	widget.BaseWidget
	level     float32 // 0..1
	charging  bool
	available bool
	pulse     float32 // animated 0..1 (auto-reversing breathe)
	anim      *fyne.Animation
}

func newBatteryGauge() *batteryGauge {
	b := &batteryGauge{}
	b.ExtendBaseWidget(b)
	return b
}

func (b *batteryGauge) set(percent int, charging, available bool) {
	b.level = float32(percent) / 100
	if b.level < 0 {
		b.level = 0
	}
	if b.level > 1 {
		b.level = 1
	}
	b.charging = charging
	b.available = available
	if b.anim == nil && available {
		b.startAnim()
	}
	b.Refresh()
}

func (b *batteryGauge) startAnim() {
	b.anim = fyne.NewAnimation(1400*time.Millisecond, func(f float32) {
		b.pulse = f
		b.Refresh()
	})
	b.anim.RepeatCount = fyne.AnimationRepeatForever
	b.anim.AutoReverse = true
	b.anim.Curve = fyne.AnimationEaseInOut
	b.anim.Start()
}

func batteryColor(level float32, charging bool) color.NRGBA {
	if charging {
		return rgb(accent)
	}
	switch {
	case level > 0.5:
		return color.NRGBA{0x3D, 0xD6, 0x8C, 0xFF}
	case level > 0.2:
		return color.NRGBA{0xE0, 0xB0, 0x3D, 0xFF}
	default:
		return color.NRGBA{0xE0, 0x5A, 0x4A, 0xFF}
	}
}

func (b *batteryGauge) CreateRenderer() fyne.WidgetRenderer {
	body := canvas.NewRectangle(color.Transparent)
	body.StrokeColor = rgb(0x3A3F4D)
	body.StrokeWidth = 2
	body.CornerRadius = 6
	track := canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x12})
	track.CornerRadius = 4
	fill := canvas.NewRectangle(batteryColor(0, false))
	fill.CornerRadius = 4
	glow := canvas.NewRectangle(color.NRGBA{0xFF, 0xFF, 0xFF, 0x00})
	glow.CornerRadius = 4
	cap := canvas.NewRectangle(rgb(0x3A3F4D))
	cap.CornerRadius = 2
	return &batteryRenderer{b: b, body: body, track: track, fill: fill, glow: glow, cap: cap}
}

type batteryRenderer struct {
	b                             *batteryGauge
	body, track, fill, glow, cap *canvas.Rectangle
}

func (r *batteryRenderer) MinSize() fyne.Size { return fyne.NewSize(220, 50) }

func (r *batteryRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.body, r.fill, r.glow, r.cap}
}

func (r *batteryRenderer) Destroy() {}

func (r *batteryRenderer) Layout(size fyne.Size) {
	capW := float32(7)
	bodyW := size.Width - capW - 4
	h := size.Height
	r.body.Resize(fyne.NewSize(bodyW, h))
	r.body.Move(fyne.NewPos(0, 0))
	r.cap.Resize(fyne.NewSize(capW, h*0.46))
	r.cap.Move(fyne.NewPos(bodyW+2, h*0.27))

	pad := float32(5)
	innerW := bodyW - pad*2
	innerH := h - pad*2
	r.track.Resize(fyne.NewSize(innerW, innerH))
	r.track.Move(fyne.NewPos(pad, pad))

	fillW := innerW * r.b.level
	if r.b.charging {
		// rising charge animation: grow from current level toward full
		fillW = innerW * (r.b.level + (1-r.b.level)*r.b.pulse)
	}
	r.fill.Resize(fyne.NewSize(fillW, innerH))
	r.fill.Move(fyne.NewPos(pad, pad))
	r.glow.Resize(fyne.NewSize(fillW, innerH))
	r.glow.Move(fyne.NewPos(pad, pad))
}

func (r *batteryRenderer) Refresh() {
	r.fill.FillColor = batteryColor(r.b.level, r.b.charging)
	// Discharging: a soft white glow breathes over the static fill.
	if r.b.available && !r.b.charging {
		r.glow.FillColor = color.NRGBA{0xFF, 0xFF, 0xFF, byte(0x05 + int(0x1E*r.b.pulse))}
	} else {
		r.glow.FillColor = color.NRGBA{0xFF, 0xFF, 0xFF, 0x00}
	}
	r.Layout(r.b.Size())
	canvas.Refresh(r.b)
}
