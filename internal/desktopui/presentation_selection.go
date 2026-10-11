package desktopui

import "sort"

func presentationSelectedIDs(view *presentationViewState) []int64 {
	if view == nil {
		return nil
	}
	ids := make([]int64, 0, len(view.selectedShapes))
	for id, selected := range view.selectedShapes {
		if id != 0 && selected {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 && view.selectedShape != 0 {
		return []int64{view.selectedShape}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func setPresentationSingleSelection(view *presentationViewState, id int64) {
	if view == nil {
		return
	}
	view.selectedShape = id
	view.selectedShapes = make(map[int64]bool, 1)
	if id != 0 {
		view.selectedShapes[id] = true
	}
}

func togglePresentationSelection(view *presentationViewState, id int64) {
	if view == nil || id == 0 {
		return
	}
	if view.selectedShapes == nil {
		view.selectedShapes = make(map[int64]bool)
		if view.selectedShape != 0 {
			view.selectedShapes[view.selectedShape] = true
		}
	}
	if view.selectedShapes[id] {
		delete(view.selectedShapes, id)
	} else {
		view.selectedShapes[id] = true
	}
	view.selectedShape = 0
	ids := presentationSelectedIDs(view)
	if view.selectedShapes[id] {
		view.selectedShape = id
	} else if len(ids) == 0 {
		view.selectedShape = 0
	} else {
		view.selectedShape = ids[0]
	}
}

func clearPresentationSelection(view *presentationViewState) {
	setPresentationSingleSelection(view, 0)
}
