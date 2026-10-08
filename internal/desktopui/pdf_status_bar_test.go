package desktopui

import (
	"image"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/document"
	"glance/internal/format/pdfdoc"
)

func TestPDFStatusBarTextVerticallyCentered(t *testing.T) {
	tests := []struct {
		name    string
		message string
		dirty   bool
	}{
		{name: "clean without message", message: "", dirty: false},
		{name: "dirty with message", message: "正在准备导出", dirty: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := pdfTestBytes(t)
			d, err := pdfdoc.OpenSnapshot(data)
			if err != nil {
				t.Fatal(err)
			}
			app := New(nil, "")
			defer close(app.shutdown)
			if err := app.installDocument(loadedDocument{path: "report.pdf", pdfDoc: d}); err != nil {
				t.Fatal(err)
			}
			v := app.ensurePDFView()
			v.info = document.PDFInfo{
				Pages: 5,
				Bytes: 2 * 1024 * 1024,
				Dirty: tc.dirty,
			}
			v.page = 0
			v.message = tc.message

			size := image.Pt(800, 34)
			var ops op.Ops
			gtx := layout.Context{
				Ops:         &ops,
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Exact(size),
			}

			dims := app.layoutPDFStatus(gtx)
			if dims.Size.Y != 34 {
				t.Fatalf("status bar height = %d; want 34", dims.Size.Y)
			}

			win, err := headless.NewWindow(size.X, size.Y)
			if err != nil {
				t.Skip("headless window unavailable:", err)
			}
			defer win.Release()

			if err := win.Frame(&ops); err != nil {
				t.Fatal(err)
			}
			shot := image.NewRGBA(image.Rectangle{Max: size})
			if err := win.Screenshot(shot); err != nil {
				t.Fatal(err)
			}

			// Find the bounding box of text ink (pixels darker than background).
			// Status bar has a 1px top border at y=0, and left inset is 20.
			bg := pageBackground
			minY, maxY := -1, -1
			for y := 1; y < size.Y; y++ {
				rowCount := 0
				for x := 20; x < size.X; x++ {
					c := shot.RGBAAt(x, y)
					if int(c.R)+int(c.G)+int(c.B) < int(bg.R)+int(bg.G)+int(bg.B)-60 {
						rowCount++
					}
				}
				if rowCount > 0 {
					if minY == -1 {
						minY = y
					}
					maxY = y
				}
			}

			if minY == -1 {
				t.Fatal("no text ink found in status bar screenshot")
			}

			topPadding := minY
			bottomPadding := size.Y - 1 - maxY
			t.Logf("text bounding Y: [%d, %d], topPadding: %d, bottomPadding: %d", minY, maxY, topPadding, bottomPadding)

			diff := topPadding - bottomPadding
			if diff < 0 {
				diff = -diff
			}
			if diff > 3 {
				t.Fatalf("status bar text is not vertically centered: topPadding=%d, bottomPadding=%d (diff=%d > 3)", topPadding, bottomPadding, diff)
			}
		})
	}
}
