package desktopui

import (
	"reflect"

	"gioui.org/widget"
)

// editorHasActiveComposition reads Gio's private IME range because widget.Editor
// does not expose composition state. Keep this compatibility probe at the UI
// input boundary and fail closed if Gio changes its internal layout.
func editorHasActiveComposition(editor *widget.Editor) bool {
	if editor == nil {
		return false
	}
	value := reflect.ValueOf(editor).Elem().FieldByName("ime")
	if !value.IsValid() {
		return true
	}
	state := value.FieldByName("imeState")
	if !state.IsValid() {
		return true
	}
	rangeValue := state.FieldByName("composition")
	if !rangeValue.IsValid() {
		return true
	}
	start, end := rangeValue.FieldByName("Start"), rangeValue.FieldByName("End")
	if !start.IsValid() || !end.IsValid() || start.Kind() < reflect.Int || start.Kind() > reflect.Int64 || end.Kind() < reflect.Int || end.Kind() > reflect.Int64 {
		return true
	}
	return end.Int() > start.Int()
}
