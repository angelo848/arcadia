package winapi

// Rect is a screen rectangle in pixels (same layout as the Win32 RECT).
type Rect struct {
	Left, Top, Right, Bottom int32
}

// Width returns the rectangle width.
func (r Rect) Width() int32 { return r.Right - r.Left }

const (
	overlayBaseDPI    = 96
	overlayBaseWidth  = 240
	overlayBaseHeight = 56
	overlayTopMargin  = 48
)

// overlaySize returns the overlay size in pixels for the given dpi.
func overlaySize(dpi int) (w, h int) {
	return overlayBaseWidth * dpi / overlayBaseDPI, overlayBaseHeight * dpi / overlayBaseDPI
}

// overlayRect places a w x h overlay centered horizontally at the top of the
// work area, below a margin scaled by dpi.
func overlayRect(work Rect, dpi, w, h int) Rect {
	x := int(work.Left) + (int(work.Width())-w)/2
	y := int(work.Top) + overlayTopMargin*dpi/overlayBaseDPI
	return Rect{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}
}
