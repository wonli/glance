package wordlayout

import "strings"

func (c *layoutCursor) beginSection(sourceID, startType string) {
	startType = strings.ToLower(startType)
	switch startType {
	case "continuous":
		return
	case "nextpage", "evenpage", "oddpage":
		if c.pendingBreak {
			if c.used {
				c.nextPage()
			}
			c.pendingBreak = false
		} else if c.used && !c.reuseEmptySectionPage() {
			c.nextPage()
		}
		if startType == "evenpage" || startType == "oddpage" {
			wantEven := startType == "evenpage"
			for c.pageIndex >= 0 && ((c.pageIndex+1)%2 == 0) != wantEven {
				c.nextPage()
			}
		}
	default:
		c.diagnostic(DiagnosticWarning, sourceID, "section-start-type", "The section uses an unknown start type; it was laid out as a next-page section.")
		if c.pendingBreak {
			if c.used {
				c.nextPage()
			}
			c.pendingBreak = false
		} else if c.used && !c.reuseEmptySectionPage() {
			c.nextPage()
		}
	}
}

// Empty paragraphs may overflow immediately before a next-page section.
// That automatic advance already supplies the section's new page. Explicit
// page breaks and odd/even section padding must still retain authored blanks.
func (c *layoutCursor) reuseEmptySectionPage() bool {
	if !c.automaticPageStart {
		return false
	}
	page := c.result.Pages[c.pageIndex]
	if len(page.Images) != 0 || len(page.TableCells) != 0 || len(page.ParagraphBorders) != 0 {
		return false
	}
	for _, fragment := range page.Text {
		if strings.TrimSpace(fragment.Text) != "" {
			return false
		}
	}
	c.currentCol = 0
	c.y = c.content.YPT
	c.used = false
	c.automaticPageStart = false
	c.hasPreviousParagraph = false
	c.previousParagraphAfterPT = 0
	c.previousParagraphLineGapPT = 0
	return true
}
