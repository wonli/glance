package desktopui

import (
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"glance/internal/workspace"
	"image"
	"image/png"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestProofreadingTabsKeepCommonStripVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	fixture := newWindowViewFixture(t)
	for _, scale := range []float32{1, 2} {
		host := newAppWithRuntime(nil, newDesktopRuntime())
		host.proofreadingModule = true
		host.managedBackgroundClosed = true
		w := host.ensureProofreadingWorkbench()
		w.loaded = true
		w.projects = []workspace.ProofreadingProject{{ID: "book", Title: "校对测试书稿", PageCount: 500, CurrentPage: 6, ReviewedCount: 3}}
		editor := newAppWithRuntime(nil, host.desktopRuntime)
		editor.managedBackgroundClosed = true
		editor.proofreading = newProofreadingWorkspace(proofreadingWindowRequest{sourceName: "校对测试书稿.pdf"})
		editor.proofreadingTabHost = host
		host.proofreadingTabs = []*proofreadingTab{{id: "proofreading:book", editor: editor}}
		var ops op.Ops
		var router input.Router
		size := image.Pt(int(1440*scale), int(900*scale))
		render := func() *image.RGBA {
			ops.Reset()
			g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Constraints: layout.Exact(size)}
			host.layout(g)
			shot := renderPresentationTestOps(t, &ops, size)
			router.Frame(&ops)
			return shot
		}
		workbench := render()
		// Sharing the source window's populated document collection must not
		// introduce ordinary document tabs into the proofreading strip.
		host.collection = fixture.runtime.collection
		withSourceDocuments := render()
		for y := 0; y < int(42*scale); y++ {
			for x := 0; x < size.X; x++ {
				if workbench.RGBAAt(x, y) != withSourceDocuments.RGBAAt(x, y) {
					t.Fatalf("shared documents changed proofreading tab strip at %gx (%d,%d)", scale, x, y)
				}
			}
		}
		host.activateWorkspaceTab("proofreading:book")
		manuscript := render()
		host.activateRecent()
		returned := render()
		// The strip still contains visible tab labels on both content pages.
		for name, shot := range map[string]*image.RGBA{"workbench": workbench, "manuscript": manuscript, "returned": returned} {
			ink := 0
			for y := int(5 * scale); y < int(42*scale); y++ {
				for x := int(16 * scale); x < int(315*scale); x++ {
					p := shot.RGBAAt(x, y)
					if p.R < 130 && p.G < 130 && p.B < 150 {
						ink++
					}
				}
			}

			if ink < 100 {
				t.Fatalf("%s lost common tab labels at %gx ink=%d", name, scale, ink)
			}
		}

		bodyInk := 0
		for y := int(160 * scale); y < int(400*scale); y++ {
			for x := int(226 * scale); x < int(900*scale); x++ {
				p := workbench.RGBAAt(x, y)
				if p.A > 240 && p.R < 130 && p.G < 130 && p.B < 150 {
					bodyInk++
				}
			}
		}
		if bodyInk < 100 {
			t.Fatalf("workbench body missing: ink=%d sample=%v", bodyInk, workbench.RGBAAt(int(300*scale), int(200*scale)))
		}
		// Switching back has the same strip and content as the initial workbench,
		// without a leftover nested manuscript bar or frame from the old page.
		if string(workbench.Pix) != string(returned.Pix) {
			t.Fatalf("workbench changed after tab round trip at %gx", scale)
		}
		if dir := os.Getenv("GLANCE_PROOF_TABS_CAPTURE"); dir != "" && scale == 1 {
			for name, shot := range map[string]*image.RGBA{"workbench": workbench, "manuscript": manuscript} {
				f, err := os.Create(dir + "/" + name + ".png")
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, shot)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestApplicationHomeIsSeparateFromProofreadingModuleVisual(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := newAppWithRuntime(nil, newDesktopRuntime())
	a.managedBackgroundClosed = true
	var ops op.Ops
	var router input.Router
	size := image.Pt(1280, 820)
	g := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(size)}
	a.layoutDocument(g)
	home := renderPresentationTestOps(t, &ops, size)
	ops.Reset()
	a.layoutRecent(g)
	expected := renderPresentationTestOps(t, &ops, size)
	if string(home.Pix) != string(expected.Pix) {
		t.Fatal("application first tab no longer renders the original homepage")
	}
	if a.workspaceHomeTabWidth() != 112 {
		t.Fatal("application home tab geometry changed")
	}
	a.proofreadingModule = true
	a.ensureProofreadingWorkbench().loaded = true
	ops.Reset()
	a.layoutDocument(g)
	module := renderPresentationTestOps(t, &ops, size)
	if string(home.Pix) == string(module.Pix) {
		t.Fatal("proofreading module did not have its own workbench route")
	}
}
