//go:build darwin

package desktopui

import (
	"context"
	"crypto/sha256"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"glance/internal/format/docx"
	"glance/internal/format/wordpdf"
	"glance/internal/wordlayout"
)

// Opt-in compatibility regression for the independently authored tax agreement.
// The user's file is never copied into the repository or logged by this test.
func TestLegacyDOCIndependentSamplePagination(t *testing.T) {
	path := os.Getenv("GLANCE_DOC_PAGINATION_SAMPLE")
	if path == "" {
		t.Skip("set GLANCE_DOC_PAGINATION_SAMPLE to the independent tax-agreement DOC fixture")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(source)
	loaded, err := loadDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	shaper, err := newWordTextShaper()
	if err != nil {
		closeLoadedDocument(loaded)
		t.Fatal(err)
	}
	desktop := newDesktopRuntime()
	app := newAppWithRuntime(nil, desktop)
	app.wordLayoutEngine = wordlayout.NewEngine(shaper)
	t.Cleanup(func() { app.stopAndWaitManagedBackground(); close(app.shutdown); desktop.shutdown() })
	if err := app.installDocument(loaded); err != nil {
		t.Fatal(err)
	}
	select {
	case pending := <-app.wordLayoutResults:
		if pending.err != nil {
			t.Fatal(pending.err)
		}
		app.applyWordLayoutResult(pending)
	case <-time.After(10 * time.Second):
		t.Fatal("sample desktop pagination did not finish")
	}
	state := app.documentSessionState
	if state.wordLayoutResult == nil {
		t.Fatal("desktop rejected sample page scenes")
	}
	assertTaxAgreementPages(t, state.word, state.wordLayoutResult)
	output := filepath.Join(t.TempDir(), "reopened.docx")
	if err := state.word.SaveAs(output); err != nil {
		t.Fatal(err)
	}
	reopened, err := docx.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	result, err := wordlayout.NewEngine(shaper).Layout(context.Background(), wordlayout.Input{DocumentID: "reopened", Blocks: reopened.WordBlocks(), Layout: reopened.WordLayoutData(), DefaultPageSetup: defaultWordPageSetup})
	if err != nil {
		t.Fatal(err)
	}
	assertTaxAgreementPages(t, reopened, &result)
	if pdfPath := os.Getenv("GLANCE_DOC_PAGINATION_PDF"); pdfPath != "" {
		if _, err := wordpdf.ExportScene(context.Background(), pdfPath, *state.wordLayoutResult, shaper, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(after) != before {
		t.Fatal("sample DOC source was changed")
	}
	t.Log("accepted desktop scenes and saved/reopened DOCX: 2 A4 pages; clauses III–V and signature on page 2; source unchanged")
}

func assertTaxAgreementPages(t *testing.T, d *docx.Document, result *wordlayout.Result) {
	t.Helper()
	if len(result.Pages) != 2 {
		t.Fatalf("sample pages=%d; want 2", len(result.Pages))
	}
	layout := d.WordLayoutData()
	if len(layout.Sections) != 1 || layout.Sections[0].Grid.LinePitchPT != 15.6 {
		t.Fatal("source section line grid was lost")
	}
	page := result.Pages[0].PageSetup
	if page.WidthPT != 595.3 || page.HeightPT != 841.9 {
		t.Fatalf("page setup=%+v; want source A4 geometry", page)
	}
	blocks := d.WordBlocks()
	var labelX []float64
	for _, marker := range []string{"纳税人识别号", "身份证号", "联系电话"} {
		found := false
		for _, b := range blocks {
			if b.Paragraph == nil {
				continue
			}
			for _, in := range b.Paragraph.Inlines {
				if in.Text == nil {
					continue
				}
				at := strings.Index(in.Text.Text, marker)
				if at < 0 {
					continue
				}
				if in.Text.Style.FontFamilyASCII != "黑体" || in.Text.Style.FontFamilyEastAsia != "黑体" {
					t.Fatalf("form font lost: %+v", in.Text.Style)
				}
				offset := len([]rune(in.Text.Text[:at]))
				for _, f := range result.Pages[0].Text {
					if f.RunID != in.Text.ID {
						continue
					}
					for _, g := range f.Glyphs {
						if g.RuneStart == offset {
							labelX = append(labelX, g.XPT)
							found = true
						}
					}
				}
			}
		}
		if !found {
			t.Fatalf("missing first-page label %s", marker)
		}
	}
	if len(labelX) != 3 || math.Abs(labelX[1]-labelX[2]) > .1 || math.Abs(labelX[0]-labelX[1]) > 6.1 {
		t.Fatalf("right labels misaligned: x=%v", labelX)
	}

	if len(blocks) != 30 {
		t.Fatalf("body paragraphs=%d; want the 30 source main-story paragraphs", len(blocks))
	}
	for _, marker := range []string{"三、", "四、", "五、", "签章"} {
		found := false
		for _, b := range blocks {
			if b.Paragraph == nil {
				continue
			}
			var text strings.Builder
			ids := map[string]bool{}
			for _, inline := range b.Paragraph.Inlines {
				if inline.Text != nil {
					text.WriteString(inline.Text.Text)
					ids[inline.Text.ID] = true
				}
			}
			if strings.Contains(text.String(), "PAGE") {
				t.Fatal("header/footer field instructions were appended to the body")
			}
			if !strings.Contains(text.String(), marker) {
				continue
			}
			found = true
			for _, p := range result.Pages {
				for _, f := range p.Text {
					if ids[f.RunID] && p.Index != 1 {
						t.Fatalf("sample marker %s is on page %d; want second page", marker, p.Index+1)
					}
				}
			}
		}
		if !found {
			t.Fatalf("source marker %s was lost", marker)
		}
	}
}
