package desktopui

// WM_POINTER coordinates are packed signed 16-bit screen pixels. Preserve
// negative monitor positions instead of clamping them at the window origin.
func nativeMousePointerPosition(x, y int32) uintptr {
	return uintptr(uint16(int16(x))) | uintptr(uint16(int16(y)))<<16
}
