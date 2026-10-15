package wordpdf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"glance/internal/fontcatalog"
	"glance/internal/format/docx"
	"glance/internal/wordfont"
	"glance/internal/wordlayout"
	"golang.org/x/image/font/gofont/goregular"
)

func TestPrivateFontSceneExportKeepsLoadedResourceAfterRefresh(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private.ttf")
	if err := os.WriteFile(path, goregular.TTF, 0600); err != nil {
		t.Fatal(err)
	}
	faces, err := fontcatalog.Inspect(ctx, path, fontcatalog.Private)
	if err != nil {
		t.Fatal(err)
	}
	shaper, err := wordfont.NewWithSnapshot(fontcatalog.NewSnapshot(1, faces))
	if err != nil {
		t.Fatal(err)
	}
	word, err := docx.NewBlank()
	if err != nil {
		t.Fatal(err)
	}
	defer word.Close()
	_, runID, err := word.AppendBodyTextParagraph("Private font PDF")
	if err != nil {
		t.Fatal(err)
	}
	if err = word.SetTextRunFontFamily(runID, "Go"); err != nil {
		t.Fatal(err)
	}
	scene, err := wordlayout.NewEngine(shaper).Layout(ctx, wordlayout.Input{DocumentID: "font-snapshot", Blocks: word.WordBlocks(), Layout: word.WordLayoutData(), DefaultPageSetup: wordlayout.DefaultPageSetup()})
	if err != nil {
		t.Fatal(err)
	}
	if len(scene.Pages) != 1 {
		t.Fatalf("pages=%d", len(scene.Pages))
	}
	// The task already loaded its font. Neither source removal nor a new catalog
	// may change the bytes used when exporting this prepared scene.
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	previous := fontcatalog.Default()
	defer fontcatalog.SetDefault(previous)
	fontcatalog.SetDefault(fontcatalog.NewSnapshot(2, nil))
	result, err := ExportScene(ctx, filepath.Join(t.TempDir(), "private.pdf"), scene, shaper, imageSource{document: word}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount != 1 || result.BytesWritten == 0 {
		t.Fatalf("invalid PDF: %+v", result)
	}
}
