package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// accentTheme is a dark theme with a teal→blue accent, tuned for this app so it
// looks deliberate rather than the stock Fyne palette. It forces the dark
// variant and overrides a handful of colours; everything else delegates to the
// default theme.
type accentTheme struct{}

var _ fyne.Theme = accentTheme{}

const (
	accent = 0x5BC0FF // light "baby blue" accent, à la Logitech G HUB
)

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: byte(hex >> 16), G: byte(hex >> 8), B: byte(hex), A: 0xFF}
}

func (accentTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return rgb(accent)
	case theme.ColorNameFocus:
		return color.NRGBA{0x5B, 0xC0, 0xFF, 0x88}
	case theme.ColorNameSelection:
		return color.NRGBA{0x5B, 0xC0, 0xFF, 0x40}
	case theme.ColorNameBackground:
		return rgb(0x0D0E13)
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground:
		return rgb(0x14161D)
	case theme.ColorNameButton:
		return rgb(0x1B1E27)
	case theme.ColorNameInputBackground:
		return rgb(0x181A22)
	case theme.ColorNameInputBorder:
		return rgb(0x2A2E3A)
	case theme.ColorNameForeground:
		return rgb(0xE6E9F0)
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return rgb(0x6B7180)
	case theme.ColorNameSeparator, theme.ColorNameShadow:
		return color.NRGBA{0x00, 0x00, 0x00, 0x66}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (accentTheme) Font(s fyne.TextStyle) fyne.Resource { return theme.DefaultTheme().Font(s) }
func (accentTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (accentTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 8
	}
	return theme.DefaultTheme().Size(name)
}
