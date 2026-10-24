// ocrprobe is an experimental cloud OCR probe; output goes to stdout only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"glance/internal/ocr"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	file := flag.String("file", "", "local document to upload")
	remote := flag.String("url", "", "document URL (alternative to -file)")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall timeout")
	flag.Parse()
	if (*file == "") == (*remote == "") {
		return fmt.Errorf("specify exactly one of -file or -url")
	}
	token := os.Getenv("PADDLEOCR_TOKEN")
	if token == "" {
		return fmt.Errorf("set PADDLEOCR_TOKEN")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	source := ocr.Source{URL: *remote}
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		source.Name = *file
		source.Reader = f
	}
	client := ocr.Client{Token: token}
	job, err := client.Submit(ctx, source, ocr.Options{})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "submitted job:", job.ID)
	job, err = client.Wait(ctx, job.ID, func(j ocr.Job) {
		fmt.Fprintf(os.Stderr, "%s: %d/%d pages\n", j.State, j.Progress.ExtractedPages, j.Progress.TotalPages)
	})
	if err != nil {
		return err
	}
	pages, err := client.Results(ctx, job)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(pages)
}
