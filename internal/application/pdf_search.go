package application

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
)

const (
	maxPDFSearchPages   = 100
	maxPDFSearchResults = 100
	maxPDFSearchQuery   = 256
)

func registerPDFSearchCapability(catalog *Catalog) error {
	return catalog.Register(CapabilityRegistration{
		Descriptor: CapabilityDescriptor{
			ID: "pdf.document.search", Name: "搜索 PDF 文本",
			Description: "在明确 PDF 文档的指定页范围内执行有界、不区分大小写的文本搜索。每次最多扫描 100 页、读取 1000 个文本/批注项目并返回 100 个匹配；未截断时可用 next_start_page 继续扫描，截断时应缩小页范围。搜索不改变文档或窗口状态。",
			Category:    CapabilityQuery, Formats: []Format{FormatPDF}, Risk: RiskLow,
			RequiresDocument: true, RequiresRevision: true,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":256},"start_page":{"type":"integer","minimum":1,"maximum":50000},"page_limit":{"type":"integer","minimum":1,"maximum":100},"max_results":{"type":"integer","minimum":1,"maximum":100}},"required":["query","start_page","page_limit","max_results"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"total_pages":{"type":"integer","minimum":1},"start_page":{"type":"integer","minimum":1},"pages_scanned":{"type":"integer","minimum":0,"maximum":100},"next_start_page":{"type":"integer","minimum":0},"matches":{"type":"array","items":{"type":"object","properties":{"page_number":{"type":"integer","minimum":1},"run":{"type":"integer","minimum":0},"text":{"type":"string"},"editable":{"type":"boolean"},"bounds":{"type":"object","properties":{"x":{"type":"number"},"y":{"type":"number"},"w":{"type":"number"},"h":{"type":"number"}},"required":["x","y","w","h"],"additionalProperties":false}},"required":["page_number","run","text","editable","bounds"],"additionalProperties":false}},"results_truncated":{"type":"boolean"},"page_content_truncated":{"type":"boolean"}},"required":["query","total_pages","start_page","pages_scanned","next_start_page","matches","results_truncated","page_content_truncated"],"additionalProperties":false}`),
		},
		Handler: func(ctx context.Context, capability CapabilityContext, raw json.RawMessage) (CapabilityResult, error) {
			var input struct {
				Query      string `json:"query"`
				StartPage  int    `json:"start_page"`
				PageLimit  int    `json:"page_limit"`
				MaxResults int    `json:"max_results"`
			}
			if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Query) == "" || len([]rune(input.Query)) > maxPDFSearchQuery || input.StartPage < 1 || input.StartPage > 50000 || input.PageLimit < 1 || input.PageLimit > maxPDFSearchPages || input.MaxResults < 1 || input.MaxResults > maxPDFSearchResults {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			input.Query = strings.TrimSpace(input.Query)
			snapshot, err := capability.Runtime.Queries().InspectDocument(ctx, InspectRequest{
				DocumentID: capability.Document.DocumentID, Start: input.StartPage - 1,
				Limit: input.PageLimit, MaxBytes: MaxQueryResultBytes,
			})
			if err != nil {
				return CapabilityResult{}, err
			}
			if snapshot.Info.Revision != capability.Document.Revision {
				return CapabilityResult{}, ErrRevisionConflict
			}
			var pages struct {
				TotalPages int `json:"total_pages"`
				Start      int `json:"start"`
				Pages      []struct {
					Index    int    `json:"index"`
					Revision uint64 `json:"revision"`
					Text     []struct {
						Run      int    `json:"run"`
						Text     string `json:"text"`
						Editable bool   `json:"editable"`
						Bounds   struct {
							X float64 `json:"x"`
							Y float64 `json:"y"`
							W float64 `json:"width"`
							H float64 `json:"height"`
						} `json:"bounds"`
					} `json:"text"`
				} `json:"pages"`
			}
			if err := json.Unmarshal(snapshot.Content, &pages); err != nil || pages.TotalPages < 1 {
				return CapabilityResult{}, ErrInvalidCommandResult
			}
			if input.StartPage > pages.TotalPages {
				return CapabilityResult{}, ErrInvalidCommandArguments
			}
			matches := make([]pdfSearchMatch, 0, min(input.MaxResults, 16))
			resultsTruncated := false
			for _, page := range pages.Pages {
				if page.Revision != capability.Document.Revision {
					return CapabilityResult{}, ErrRevisionConflict
				}
				for _, text := range page.Text {
					if !containsFolded(text.Text, input.Query) {
						continue
					}
					if len(matches) == input.MaxResults {
						resultsTruncated = true
						continue
					}
					matches = append(matches, pdfSearchMatch{
						PageNumber: page.Index + 1, Run: text.Run,
						Text: pdfSearchExcerpt(text.Text, input.Query), Editable: text.Editable,
						Bounds: pdfSearchBounds{X: text.Bounds.X, Y: text.Bounds.Y, W: text.Bounds.W, H: text.Bounds.H},
					})
				}
			}
			nextStartPage := 0
			pagesScanned := len(pages.Pages)
			if pagesScanned > 0 && input.StartPage+pagesScanned <= pages.TotalPages {
				nextStartPage = input.StartPage + pagesScanned
			}
			pageContentTruncated := false
			for _, diagnostic := range snapshot.Diagnostics {
				if diagnostic.Code == "page_content_truncated" {
					pageContentTruncated = true
					break
				}
			}
			if resultsTruncated || pageContentTruncated {
				nextStartPage = 0
			}
			return capabilityJSONResult(pdfSearchResult{
				Query: input.Query, TotalPages: pages.TotalPages, StartPage: input.StartPage,
				PagesScanned: pagesScanned, NextStartPage: nextStartPage, Matches: matches,
				ResultsTruncated: resultsTruncated, PageContentTruncated: pageContentTruncated,
			}, false)
		},
	})
}

type pdfSearchResult struct {
	Query                string           `json:"query"`
	TotalPages           int              `json:"total_pages"`
	StartPage            int              `json:"start_page"`
	PagesScanned         int              `json:"pages_scanned"`
	NextStartPage        int              `json:"next_start_page"`
	Matches              []pdfSearchMatch `json:"matches"`
	ResultsTruncated     bool             `json:"results_truncated"`
	PageContentTruncated bool             `json:"page_content_truncated"`
}

type pdfSearchMatch struct {
	PageNumber int             `json:"page_number"`
	Run        int             `json:"run"`
	Text       string          `json:"text"`
	Editable   bool            `json:"editable"`
	Bounds     pdfSearchBounds `json:"bounds"`
}

type pdfSearchBounds struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

func containsFolded(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

func pdfSearchExcerpt(text, query string) string {
	runes := []rune(text)
	folded := make([]rune, len(runes))
	for i, r := range runes {
		folded[i] = unicode.ToLower(r)
	}
	queryRunes := []rune(strings.ToLower(query))
	start := 0
	if len(queryRunes) <= len(folded) {
		for i := 0; i+len(queryRunes) <= len(folded); i++ {
			matched := true
			for j := range queryRunes {
				if folded[i+j] != queryRunes[j] {
					matched = false
					break
				}
			}
			if matched {
				start = max(0, i-80)
				break
			}
		}
	}
	end := min(len(runes), start+240)
	if len(runes) > end {
		return string(runes[start:end]) + "…"
	}
	return string(runes[start:end])
}
