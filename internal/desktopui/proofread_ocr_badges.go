package desktopui

import (
	"image"
	"math"

	"gioui.org/layout"
)

// A badge belongs to a complete source difference run, independently of the
// virtual X editor. Its initial height comes from the A/X paper geometry; X
// text scrolling must affect only live connector endpoints, never this snapshot.
func proofreadingOCRStepBadgeConnections(gtx layout.Context, view *pdfViewState, state *proofreadingWorkspace, step *proofreadingOCRStepState, comparison proofreadingOCRStepComparison) []proofreadingOCRConnection {
	aOrigin, bOrigin, ok := proofreadingOCRStepConnectorPageOrigins(gtx, view, state, comparison.APage, comparison.BPage)
	if !ok {
		return nil
	}
	aSize := proofreadingOCRStepSourcePagePixelSize(view, comparison.APage)
	bSize := proofreadingXPagePixelSize(gtx, view, state, comparison.BPage)
	if aSize.Y <= 0 || bSize.Y <= 0 {
		return nil
	}
	type rowKey struct {
		block, run int
		kind       proofreadingOCRDiffOpKind
	}
	type row struct {
		index, top, bottom int
	}
	rows := make(map[rowKey]row)
	var connections []proofreadingOCRConnection
	runStart := 0
	for index, frame := range comparison.Frames {
		if index == 0 || frame.Kind != comparison.Frames[index-1].Kind || frame.ABlockID != comparison.Frames[index-1].ABlockID || frame.BPosition > comparison.Frames[index-1].BPosition+1 {
			runStart = index
		}
		if frame.Kind == proofreadingOCRDiffEqual || frame.Kind == proofreadingOCRDiffBOnly ||
			frame.ABlockID < 0 || frame.AOffset < 0 || frame.BPosition < comparison.BStart || frame.BPosition > comparison.BEnd {
			continue
		}
		bounds, located := proofreadingOCRStepSourceCharacterBounds(step, comparison, frame, aSize)
		if !located {
			continue
		}
		y := (bounds.Min.Y + bounds.Max.Y) / 2
		key := rowKey{block: frame.ABlockID, run: runStart, kind: frame.Kind}
		// OCR glyph boxes on one line can have different heights. Use their
		// vertical overlap rather than their exact centers as the row identity.
		existing, found := rows[key]
		overlap := min(existing.bottom, bounds.Max.Y) - max(existing.top, bounds.Min.Y)
		if found && overlap > 0 && 2*overlap >= min(existing.bottom-existing.top, bounds.Dy()) {
			connections[existing.index].allFrames = append(connections[existing.index].allFrames, frame)
			continue
		}
		rows[key] = row{index: len(connections), top: bounds.Min.Y, bottom: bounds.Max.Y}
		bY := int(math.Round(float64(y) * float64(bSize.Y) / float64(aSize.Y)))
		connections = append(connections, proofreadingOCRConnection{
			from: aOrigin.Add(image.Pt(bounds.Max.X, y)), to: bOrigin.Add(image.Pt(0, bY)),
			kind: frame.Kind, frame: frame, allFrames: []proofreadingOCRDiffFrame{frame},
		})
	}
	return connections
}

func proofreadingOCRConnectionContainsFrame(connection proofreadingOCRConnection, frame proofreadingOCRDiffFrame) bool {
	if connection.frame == frame {
		return true
	}
	for _, grouped := range connection.allFrames {
		if grouped == frame {
			return true
		}
	}
	return false
}
