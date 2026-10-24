// alignprobe locates previously recognized OCR pages in Markdown, without network
// calls or modifications to input files. Pass ocrprobe JSON files in PDF order.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"

	"glance/internal/ocr"
	"glance/internal/textalign"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	inputs := flag.String("ocr", "", "comma-separated ocrprobe JSON paths, in PDF order")
	mdPath := flag.String("md", "", "exact Markdown source path")
	cells := flag.Int("max-cells", 0, "dynamic-programming cells per page (default 4 million, maximum 16 million)")
	candidates := flag.Int("candidates", 0, "page candidate limit (default 12, maximum 64)")
	jointCells := flag.Int("joint-cells", 0, "forward+reverse cells for continuous-page refinement (default 2 million, max 8 million)")
	first := flag.Int("first-page", 1, "original PDF number of the first OCR page")
	flag.Parse()
	if *inputs == "" || *mdPath == "" || *first < 1 {
		return fmt.Errorf("use -ocr page1.json,page2.json -md /path/book.md [-first-page 1]")
	}
	md, err := readBounded(*mdPath, 8<<20)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var pages []textalign.Page
	for _, name := range strings.Split(*inputs, ",") {
		raw, err := readBounded(strings.TrimSpace(name), 64<<20)
		if err != nil {
			return err
		}
		var recognized []ocr.Page
		if err = json.Unmarshal(raw, &recognized); err != nil {
			return fmt.Errorf("invalid ocrprobe JSON: %w", err)
		}
		if len(recognized) == 0 {
			return fmt.Errorf("OCR input contains no pages")
		}
		for _, p := range recognized {
			l, err := p.Layout()
			if err != nil {
				return err
			}
			sort.SliceStable(l.Blocks, func(i, j int) bool {
				if l.Blocks[i].Order == nil {
					return false
				}
				if l.Blocks[j].Order == nil {
					return true
				}
				return *l.Blocks[i].Order < *l.Blocks[j].Order
			})
			page := textalign.Page{Number: *first + len(pages)}
			for _, b := range l.Blocks {
				page.Blocks = append(page.Blocks, textalign.Block{ID: b.ID, Text: b.Text, Label: b.Label, Bounds: b.Bounds})
			}
			pages = append(pages, page)
		}
	}
	results, err := textalign.Align(ctx, pages, string(md), textalign.Options{MaxCells: *cells, MaxCandidates: *candidates, MaxJointCells: *jointCells})
	if err != nil {
		return err
	}
	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(results)
}
func readBounded(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input exceeds size limit: %s", path)
	}
	return data, nil
}
