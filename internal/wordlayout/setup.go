package wordlayout

import (
	"glance/internal/document"
	"math"
	"strings"
)

func resolvePageSetup(layout document.WordLayoutData, fallback document.WordPageSetup) document.WordPageSetup {
	if fallback.WidthPT <= 0 || fallback.HeightPT <= 0 {
		fallback = document.WordPageSetup{
			WidthPT: 595.28, HeightPT: 841.89, Orientation: "portrait",
			MarginTopPT: 72, MarginRightPT: 72, MarginBottomPT: 72, MarginLeftPT: 72,
		}
	}
	section := document.WordPageSetup{}
	if len(layout.Sections) > 0 {
		section = layout.Sections[0].PageSetup
	}
	value := func(mask document.WordPageSetupMask, bit document.WordPageSetupMask, actual, defaultValue float64) float64 {
		if mask&bit != 0 || actual != 0 {
			return actual
		}
		return defaultValue
	}
	cols := 1
	if section.Columns >= 1 {
		cols = section.Columns
	} else if fallback.Columns >= 1 {
		cols = fallback.Columns
	}
	colSpace := 36.0
	if section.Set&document.WordPageColumnSpacingSet != 0 || section.ColumnSpacingPT > 0 {
		colSpace = section.ColumnSpacingPT
	} else if fallback.Set&document.WordPageColumnSpacingSet != 0 || fallback.ColumnSpacingPT > 0 {
		colSpace = fallback.ColumnSpacingPT
	}
	setup := document.WordPageSetup{
		WidthPT:         value(section.Set, document.WordPageWidthSet, section.WidthPT, fallback.WidthPT),
		HeightPT:        value(section.Set, document.WordPageHeightSet, section.HeightPT, fallback.HeightPT),
		MarginTopPT:     value(section.Set, document.WordPageMarginTopSet, section.MarginTopPT, fallback.MarginTopPT),
		MarginRightPT:   value(section.Set, document.WordPageMarginRightSet, section.MarginRightPT, fallback.MarginRightPT),
		MarginBottomPT:  value(section.Set, document.WordPageMarginBottomSet, section.MarginBottomPT, fallback.MarginBottomPT),
		MarginLeftPT:    value(section.Set, document.WordPageMarginLeftSet, section.MarginLeftPT, fallback.MarginLeftPT),
		Columns:         cols,
		ColumnSpacingPT: colSpace,
		Orientation:     strings.ToLower(section.Orientation),
	}
	explicitOrientation := section.Set&document.WordPageOrientationSet != 0 || section.Orientation != ""
	if setup.Orientation == "" && section.WidthPT == 0 && section.HeightPT == 0 {
		setup.Orientation = strings.ToLower(fallback.Orientation)
		explicitOrientation = setup.Orientation != ""
	}
	if setup.Orientation == "" {
		if setup.WidthPT > setup.HeightPT {
			setup.Orientation = "landscape"
		} else {
			setup.Orientation = "portrait"
		}
	}
	if explicitOrientation && ((setup.Orientation == "landscape" && setup.WidthPT < setup.HeightPT) || (setup.Orientation == "portrait" && setup.WidthPT > setup.HeightPT)) {
		setup.WidthPT, setup.HeightPT = setup.HeightPT, setup.WidthPT
	}
	return setup
}

func sectionPageSetupsDiffer(sections []document.WordSection, fallback document.WordPageSetup) bool {
	if len(sections) < 2 {
		return false
	}
	first := resolvePageSetup(document.WordLayoutData{Sections: sections[:1]}, fallback)
	for _, section := range sections[1:] {
		current := resolvePageSetup(document.WordLayoutData{Sections: []document.WordSection{section}}, fallback)
		if !equivalentSectionPageSetups(current, first) {
			return true
		}
	}
	return false
}

func equivalentSectionPageSetups(left, right document.WordPageSetup) bool {
	withinTwip := func(a, b float64) bool { return math.Abs(a-b) <= 0.051 }
	return left.Orientation == right.Orientation && left.Columns == right.Columns &&
		withinTwip(left.WidthPT, right.WidthPT) && withinTwip(left.HeightPT, right.HeightPT) &&
		withinTwip(left.MarginTopPT, right.MarginTopPT) && withinTwip(left.MarginRightPT, right.MarginRightPT) &&
		withinTwip(left.MarginBottomPT, right.MarginBottomPT) && withinTwip(left.MarginLeftPT, right.MarginLeftPT) &&
		withinTwip(left.ColumnSpacingPT, right.ColumnSpacingPT)
}

func usesDefaultPageSetup(layout document.WordLayoutData, fallback document.WordPageSetup) bool {
	if fallback.WidthPT <= 0 || fallback.HeightPT <= 0 || len(layout.Sections) == 0 {
		return true
	}
	setup := layout.Sections[0].PageSetup
	hasValue := func(bit document.WordPageSetupMask, actual float64) bool {
		return setup.Set&bit != 0 || actual != 0
	}
	return !hasValue(document.WordPageWidthSet, setup.WidthPT) ||
		!hasValue(document.WordPageHeightSet, setup.HeightPT) ||
		!hasValue(document.WordPageMarginTopSet, setup.MarginTopPT) ||
		!hasValue(document.WordPageMarginRightSet, setup.MarginRightPT) ||
		!hasValue(document.WordPageMarginBottomSet, setup.MarginBottomPT) ||
		!hasValue(document.WordPageMarginLeftSet, setup.MarginLeftPT)
}

func pageContentRect(setup document.WordPageSetup) Rect {
	return Rect{
		XPT:      setup.MarginLeftPT,
		YPT:      setup.MarginTopPT,
		WidthPT:  math.Max(0, setup.WidthPT-setup.MarginLeftPT-setup.MarginRightPT),
		HeightPT: math.Max(0, setup.HeightPT-setup.MarginTopPT-setup.MarginBottomPT),
	}
}
