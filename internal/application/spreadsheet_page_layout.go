package application

import (
	"context"
	"fmt"

	"glance/internal/document"
)

// ApplySpreadsheetPageLayoutPatch applies exactly one explicit page setup
// operation and reports the resulting neutral layout.
func ApplySpreadsheetPageLayoutPatch(editor document.SpreadsheetPageLayoutEditor, sheet string, patch SpreadsheetPageLayoutPatch) (document.SpreadsheetPageLayout, bool, error) {
	if editor == nil || sheet == "" {
		return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
	}
	current, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		return document.SpreadsheetPageLayout{}, false, err
	}
	changed := false
	switch patch.Operation {
	case "orientation":
		if patch.Orientation == nil || patch.PaperSize != nil || hasMarginPatch(patch) || patch.PrintArea != nil || patch.PrintTitles != nil {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		if *patch.Orientation != "portrait" && *patch.Orientation != "landscape" {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		changed = current.Orientation != *patch.Orientation
		if changed {
			err = editor.SetSheetPageOrientation(sheet, *patch.Orientation)
		}
	case "margins":
		if patch.MarginTop == nil || patch.MarginRight == nil || patch.MarginBottom == nil || patch.MarginLeft == nil || patch.Orientation != nil || patch.PaperSize != nil || patch.PrintArea != nil || patch.PrintTitles != nil {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		margins := [4]float64{*patch.MarginTop, *patch.MarginRight, *patch.MarginBottom, *patch.MarginLeft}
		for _, margin := range margins {
			if margin < 0 || margin > 20 {
				return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
			}
		}
		changed = current.MarginTop != margins[0] || current.MarginRight != margins[1] || current.MarginBottom != margins[2] || current.MarginLeft != margins[3]
		if changed {
			err = editor.SetSheetPageMargins(sheet, margins[0], margins[1], margins[2], margins[3])
		}
	case "paper_size":
		if patch.PaperSize == nil || patch.Orientation != nil || hasMarginPatch(patch) || patch.PrintArea != nil || patch.PrintTitles != nil {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		if *patch.PaperSize != 1 && *patch.PaperSize != 5 && *patch.PaperSize != 8 && *patch.PaperSize != 9 {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		changed = current.PaperSize != *patch.PaperSize
		if changed {
			err = editor.SetSheetPageSize(sheet, *patch.PaperSize)
		}
	case "print_area":
		if patch.PrintArea == nil || patch.Orientation != nil || patch.PaperSize != nil || hasMarginPatch(patch) || patch.PrintTitles != nil {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		changed = current.PrintArea != *patch.PrintArea
		if changed {
			err = editor.SetSheetPrintArea(sheet, *patch.PrintArea)
		}
	case "print_titles":
		if patch.PrintTitles == nil || patch.Orientation != nil || patch.PaperSize != nil || hasMarginPatch(patch) || patch.PrintArea != nil {
			return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
		}
		changed = current.PrintTitles != *patch.PrintTitles
		if changed {
			err = editor.SetSheetPrintTitles(sheet, *patch.PrintTitles)
		}
	default:
		return document.SpreadsheetPageLayout{}, false, ErrInvalidCommandArguments
	}
	if err != nil {
		return document.SpreadsheetPageLayout{}, false, err
	}
	if !changed {
		return current, false, nil
	}
	updated, err := editor.GetSheetPageLayout(sheet)
	if err != nil {
		return document.SpreadsheetPageLayout{}, false, fmt.Errorf("read updated spreadsheet page layout: %w", err)
	}
	return updated, true, nil
}

func hasMarginPatch(patch SpreadsheetPageLayoutPatch) bool {
	return patch.MarginTop != nil || patch.MarginRight != nil || patch.MarginBottom != nil || patch.MarginLeft != nil
}

func ApplySpreadsheetGridlines(ctx context.Context, editor document.SpreadsheetGridlinesEditor, sheet string, show bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if editor == nil || sheet == "" {
		return false, ErrInvalidCommandArguments
	}
	current, err := editor.GetSheetGridlines(sheet)
	if err != nil {
		return false, err
	}
	if current == show {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := editor.SetSheetGridlines(sheet, show); err != nil {
		return false, err
	}
	return true, nil
}
