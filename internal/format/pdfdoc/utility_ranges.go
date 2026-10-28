package pdfdoc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PageRange describes a 1-based inclusive range of PDF pages.
type PageRange struct {
	Start int
	End   int
}

// ParsePageRanges parses comma-separated 1-based page numbers and ascending
// inclusive ranges. The returned ranges preserve the order entered by the user.
func ParsePageRanges(input string, pageCount int) ([]PageRange, error) {
	if pageCount < 1 {
		return nil, fmt.Errorf("PDF page count must be positive")
	}
	parts := strings.Split(input, ",")
	ranges := make([]PageRange, 0, len(parts))
	for index, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, fmt.Errorf("page range item %d is empty", index+1)
		}
		separator := strings.IndexByte(token, '-')
		if separator < 0 {
			page, err := parsePageNumber(token)
			if err != nil {
				return nil, fmt.Errorf("page range item %d: %w", index+1, err)
			}
			ranges = append(ranges, PageRange{Start: page, End: page})
			continue
		}
		if strings.IndexByte(token[separator+1:], '-') >= 0 {
			return nil, fmt.Errorf("page range item %d must contain one hyphen", index+1)
		}
		start, err := parsePageNumber(token[:separator])
		if err != nil {
			return nil, fmt.Errorf("page range item %d start: %w", index+1, err)
		}
		end, err := parsePageNumber(token[separator+1:])
		if err != nil {
			return nil, fmt.Errorf("page range item %d end: %w", index+1, err)
		}
		if end < start {
			return nil, fmt.Errorf("page range item %d is descending", index+1)
		}
		ranges = append(ranges, PageRange{Start: start, End: end})
	}

	for index, pageRange := range ranges {
		if pageRange.End > pageCount {
			return nil, fmt.Errorf("page range item %d ends at page %d, but the PDF has %d pages", index+1, pageRange.End, pageCount)
		}
	}
	ordered := append([]PageRange(nil), ranges...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Start == ordered[j].Start {
			return ordered[i].End < ordered[j].End
		}
		return ordered[i].Start < ordered[j].Start
	})
	for index := 1; index < len(ordered); index++ {
		if ordered[index].Start <= ordered[index-1].End {
			return nil, fmt.Errorf("page ranges overlap at page %d", ordered[index].Start)
		}
	}
	return ranges, nil
}

func parsePageNumber(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("page number is empty")
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("page number %q must contain decimal digits only", value)
		}
	}
	page, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("page number %q is too large", value)
	}
	if page < 1 {
		return 0, fmt.Errorf("page number must be positive")
	}
	return page, nil
}
