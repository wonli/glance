package systray

import (
	"github.com/gogpu/systray/internal"
)

// TrayID uniquely identifies a system tray icon. Zero is invalid.
type TrayID uint32

// SystemTray represents a system tray icon with context menu.
// Create with New(). All setter methods return *SystemTray for chaining.
type SystemTray struct {
	impl *internal.Tray
}

// New creates a new system tray icon.
func New() *SystemTray {
	tray, _ := NewChecked()
	return tray
}

// NewChecked creates a system tray and reports native registration errors.
// New is retained for compatibility with callers that ignore initialization errors.
func NewChecked() (*SystemTray, error) {
	t := &SystemTray{}
	t.impl = &internal.Tray{
		ID: internal.NewTrayID(),
	}
	// Pass pointer to impl.Callbacks so platform sees updates from OnClick/OnDoubleClick/OnRightClick.
	platform := internal.NewPlatformTray(&t.impl.Callbacks)
	t.impl.Platform = platform
	if err := platform.Create(); err != nil {
		return t, err
	}
	return t, nil
}

// ID returns the unique identifier for this tray icon.
func (t *SystemTray) ID() TrayID {
	return TrayID(t.impl.ID)
}

// SetIcon sets the tray icon from PNG bytes.
func (t *SystemTray) SetIcon(png []byte) *SystemTray {
	_ = t.impl.SetIcon(png)
	return t
}

// SetIconChecked sets the tray icon and reports native conversion errors.
func (t *SystemTray) SetIconChecked(png []byte) error {
	return t.impl.SetIcon(png)
}

// SetDarkModeIcon sets an alternative icon for dark mode (Windows).
// When set, the tray automatically switches between the light and dark icons
// based on the system theme. On Windows, theme changes are detected via
// WM_SETTINGCHANGE with "ImmersiveColorSet".
func (t *SystemTray) SetDarkModeIcon(png []byte) *SystemTray {
	_ = t.impl.SetDarkModeIcon(png)
	return t
}

// SetTemplateIcon sets a macOS template image (monochrome, adapts to theme).
func (t *SystemTray) SetTemplateIcon(png []byte) *SystemTray {
	t.impl.SetTemplateIcon(png)
	return t
}

// SetAppName sets the application name used in notifications on Linux.
// On Windows and macOS, the OS determines the application identity automatically.
func (t *SystemTray) SetAppName(name string) *SystemTray {
	t.impl.SetAppName(name)
	return t
}

// SetTooltip sets the hover tooltip text.
func (t *SystemTray) SetTooltip(text string) *SystemTray {
	_ = t.impl.SetTooltip(text)
	return t
}

// SetTooltipChecked sets the tray tooltip and reports native errors.
func (t *SystemTray) SetTooltipChecked(text string) error {
	return t.impl.SetTooltip(text)
}

// SetMenu attaches a context menu to the tray icon. Pass nil to remove the
// current menu.
func (t *SystemTray) SetMenu(menu *Menu) *SystemTray {
	var internalMenu *internal.Menu
	if menu != nil {
		internalMenu = menu.impl
	}
	_ = t.impl.SetMenu(internalMenu)
	return t
}

// SetMenuChecked attaches a context menu and reports native setup errors.
func (t *SystemTray) SetMenuChecked(menu *Menu) error {
	var internalMenu *internal.Menu
	if menu != nil {
		internalMenu = menu.impl
	}
	return t.impl.SetMenu(internalMenu)
}

// OnClick registers a left-click handler.
func (t *SystemTray) OnClick(fn func()) *SystemTray {
	t.impl.Callbacks.OnClick = fn
	return t
}

// OnDoubleClick registers a double-click handler.
func (t *SystemTray) OnDoubleClick(fn func()) *SystemTray {
	t.impl.Callbacks.OnDoubleClick = fn
	return t
}

// OnRightClick registers a right-click handler.
func (t *SystemTray) OnRightClick(fn func()) *SystemTray {
	t.impl.Callbacks.OnRightClick = fn
	return t
}

// ShowNotification displays an OS-level notification.
func (t *SystemTray) ShowNotification(title, message string) *SystemTray {
	_ = t.impl.ShowNotification(title, message)
	return t
}

// Show makes the tray icon visible.
func (t *SystemTray) Show() *SystemTray {
	_ = t.impl.Show()
	return t
}

// ShowChecked makes the tray icon visible and reports native registration errors.
func (t *SystemTray) ShowChecked() error {
	return t.impl.Show()
}

// Hide makes the tray icon invisible without removing it.
func (t *SystemTray) Hide() *SystemTray {
	_ = t.impl.Hide()
	return t
}

// Run blocks the calling goroutine, pumping the platform message loop.
// Call from main() after Show(). Returns when Quit() is called.
func (t *SystemTray) Run() error {
	return t.impl.Run()
}

// Remove destroys the tray icon and releases all resources.
func (t *SystemTray) Remove() {
	t.impl.Remove()
}

// Bounds returns the tray icon's screen position (x, y, width, height).
func (t *SystemTray) Bounds() (x, y, w, h int) {
	return t.impl.Bounds()
}
