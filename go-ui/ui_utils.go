package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// hoverButton extends the standard Fyne Button to provide a pointer cursor on hover.
type hoverButton struct {
	widget.Button
}

// NewHoverButton creates a new button that changes the OS cursor to a pointer on hover.
func NewHoverButton(label string, tapped func()) *hoverButton {
	h := &hoverButton{}
	h.Text = label
	h.OnTapped = tapped
	h.ExtendBaseWidget(h)
	return h
}

// Cursor declares that this widget has a custom cursor
func (h *hoverButton) Cursor() desktop.Cursor {
	return desktop.PointerCursor
}

// NewHoverButtonWithIcon is like NewHoverButton but includes an icon
func NewHoverButtonWithIcon(label string, icon fyne.Resource, tapped func()) *hoverButton {
	h := &hoverButton{}
	h.Text = label
	h.Icon = icon
	h.OnTapped = tapped
	h.ExtendBaseWidget(h)
	return h
}
