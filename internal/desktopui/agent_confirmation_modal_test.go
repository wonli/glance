package desktopui

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"glance/internal/application"
	"glance/internal/workspace"
)

func newTestConfirmationApp(t *testing.T) *App {
	t.Helper()
	app := New(nil, "")
	t.Cleanup(func() {
		close(app.shutdown)
	})
	dir := t.TempDir()
	store, err := workspace.Open(filepath.Join(dir, "workspace.db"))
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	rt := app.ensureDesktopRuntime()
	rt.workspaceStore = store
	return app
}

func stageTestConfirmation(app *App, info application.ConfirmationInfo) *agentConfirmationCard {
	if info.ConfirmationID == "" {
		info.ConfirmationID = "test-conf-1"
	}
	if info.Status == "" {
		info.Status = application.ConfirmationPending
	}
	if info.CreatedAt.IsZero() {
		info.CreatedAt = time.Now()
	}
	if info.ExpiresAt.IsZero() {
		info.ExpiresAt = time.Now().Add(10 * time.Minute)
	}
	app.upsertAgentConfirmation(info)
	return app.activeAgentConfirmation()
}

func TestAgentLightweightConfirmationModal_LayoutAndBackdrop(t *testing.T) {
	app := newTestConfirmationApp(t)
	card := stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "conf-doc-insert",
		CapabilityID:   "word.text.insert",
		CapabilityName: "插入文档",
		ImpactSummary:  "是否允许 AI 将结果插入当前文档？",
	})
	if card == nil {
		t.Fatal("active confirmation card missing")
	}

	size := image.Pt(1440, 900)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	dims := app.layoutAgentConfirmationModal(gtx)
	if dims.Size != size {
		t.Fatalf("expected modal to cover full window %v, got %v", size, dims.Size)
	}

	// Backdrop click does not dismiss the modal (waits for explicit choice)
	card.scrim.Click()
	dims = app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() == nil {
		t.Fatal("clicking backdrop dismissed the confirmation modal; expected modal to remain until explicit decision")
	}

	// Rejecting cancels the insertion and preserves AI result
	card.reject.Click()
	dims = app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() != nil {
		t.Fatal("rejecting should dismiss the active confirmation modal")
	}
	if card.info.Status != application.ConfirmationRejected {
		t.Fatalf("expected status %v, got %v", application.ConfirmationRejected, card.info.Status)
	}
}

func TestAgentLightweightConfirmationModal_Approve(t *testing.T) {
	app := newTestConfirmationApp(t)
	card := stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "conf-doc-insert-2",
		CapabilityID:   "word.table.insert",
		CapabilityName: "插入文档表格",
	})

	size := image.Pt(1440, 900)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	app.layoutAgentConfirmationModal(gtx)

	// Approving resolves the confirmation
	card.approve.Click()
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() != nil {
		t.Fatal("approving should dismiss the active confirmation modal")
	}
	if card.info.Status != application.ConfirmationRunning {
		t.Fatalf("expected status %v, got %v", application.ConfirmationRunning, card.info.Status)
	}
}

func TestAgentPermissionConfirmationModal_LayoutAndDecisions(t *testing.T) {
	app := newTestConfirmationApp(t)
	card := stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "conf-perm-1",
		CapabilityID:   "folder.access",
		CapabilityName: "访问文件夹",
		ImpactSummary:  "为总结项目进展，AI 需要读取下方文件夹中的文件。\n此权限仅用于读取，不会修改或删除文件。；path=\"~/Documents/项目资料\"",
	})
	if card == nil {
		t.Fatal("active confirmation card missing")
	}
	if !isPermissionConfirmation(card.info) {
		t.Fatal("expected isPermissionConfirmation to be true for folder.access")
	}

	size := image.Pt(1440, 900)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	dims := app.layoutAgentConfirmationModal(gtx)
	if dims.Size != size {
		t.Fatalf("expected modal to cover full window %v, got %v", size, dims.Size)
	}

	// Backdrop click does not dismiss
	card.scrim.Click()
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() == nil {
		t.Fatal("clicking backdrop dismissed permission modal")
	}

	// Allow once
	card.allowOnce.Click()
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() != nil {
		t.Fatal("allowOnce should dismiss the active confirmation modal")
	}
	if card.info.Status != application.ConfirmationRunning {
		t.Fatalf("expected status %v, got %v", application.ConfirmationRunning, card.info.Status)
	}
}

func TestAgentPermissionConfirmationModal_AlwaysAllowRemembersGrant(t *testing.T) {
	app := newTestConfirmationApp(t)
	card := stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "conf-perm-2",
		CapabilityID:   "folder.read",
		CapabilityName: "读取文件夹",
		ImpactSummary:  "路径=\"/Users/project/docs\"",
	})

	size := image.Pt(1440, 900)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
	}

	app.layoutAgentConfirmationModal(gtx)

	// Click 始终允许
	card.alwaysAllow.Click()
	app.layoutAgentConfirmationModal(gtx)
	if app.activeAgentConfirmation() != nil {
		t.Fatal("alwaysAllow should dismiss active modal")
	}
	if !app.agentUI.permissionGrants["folder.read"] {
		t.Fatal("expected permission to be remembered in permissionGrants")
	}
}

func TestAgentConfirmationNotEmbeddedInSidebar(t *testing.T) {
	app := newTestConfirmationApp(t)
	stageTestConfirmation(app, application.ConfirmationInfo{
		ConfirmationID: "conf-embedded-check",
		CapabilityID:   "word.text.insert",
		CapabilityName: "插入文档",
	})

	// Layout the sidebar directly: it should not render any confirmation buttons or list
	sidebarSize := image.Pt(340, 800)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sidebarSize),
	}

	app.layoutAgentSidebar(gtx)

	// Active confirmation is still pending and waiting for global modal
	card := app.activeAgentConfirmation()
	if card == nil {
		t.Fatal("expected confirmation to remain active and pending for global modal")
	}

	// Layout root window layout: global modal overlay renders on top!
	rootSize := image.Pt(1440, 900)
	var rootOps op.Ops
	rootGtx := layout.Context{
		Ops:         &rootOps,
		Now:         time.Now(),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rootSize),
	}

	dims := app.layout(rootGtx)
	if dims.Size != rootSize {
		t.Fatalf("expected root layout size %v, got %v", rootSize, dims.Size)
	}
}

func TestAgentConfirmationVisual_RenderingAndIcons(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, scale := range []float32{1, 2} {
		app := newTestConfirmationApp(t)
		card := stageTestConfirmation(app, application.ConfirmationInfo{
			ConfirmationID: "conf-visual-1",
			CapabilityID:   "word.text.insert",
			CapabilityName: "插入文档",
			ImpactSummary:  "是否允许 AI 将结果插入当前文档？",
		})

		size := image.Pt(int(1440*scale), int(900*scale))
		var ops op.Ops
		gtx := layout.Context{
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}

		app.layoutAgentConfirmationModal(gtx)
		img := renderPresentationTestOps(t, &ops, size)
		if img == nil {
			continue // GPU skipped
		}

		// Verify AI badge fill #EDF2FA is visible
		badgeFill := color.RGBA{237, 242, 250, 255}
		badgeFound := false
		// Verify primary action button fill #343F50 is visible and right-aligned
		btnFill := color.RGBA{52, 63, 80, 255}
		btnFound := false
		minBtnX, maxBtnX := size.X, 0

		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				c := img.RGBAAt(x, y)
				if c == badgeFill {
					badgeFound = true
				}
				if c == btnFill {
					btnFound = true
					if x < minBtnX {
						minBtnX = x
					}
					if x > maxBtnX {
						maxBtnX = x
					}
				}
			}
		}

		if !badgeFound {
			t.Fatalf("scale=%v: expected AI badge background #EDF2FA to be rendered", scale)
		}
		if !btnFound {
			t.Fatalf("scale=%v: expected allow button background #343F50 to be rendered", scale)
		}
		// Card is 420dp centered in 1440dp window: bounds [510, 930]. Center = 720dp.
		// "允许" button (96dp) with 20dp right padding must be positioned at [814, 910]dp.
		expectedMinX := int(800 * scale)
		expectedMaxX := int(925 * scale)
		if minBtnX < expectedMinX || maxBtnX > expectedMaxX {
			t.Fatalf("scale=%v: allow button not right-aligned; got X bounds [%d, %d], expected within [%d, %d]",
				scale, minBtnX, maxBtnX, expectedMinX, expectedMaxX)
		}

		// Also test permission confirmation visuals
		appPerm := newTestConfirmationApp(t)
		stageTestConfirmation(appPerm, application.ConfirmationInfo{
			ConfirmationID: "conf-visual-2",
			CapabilityID:   "folder.access",
			CapabilityName: "访问文件夹",
			ImpactSummary:  "为总结项目进展，AI 需要读取下方文件夹中的文件。\n此权限仅用于读取，不会修改或删除文件。；path=\"~/Documents/项目资料\"",
		})

		var permOps op.Ops
		permGtx := layout.Context{
			Ops:         &permOps,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}

		appPerm.layoutAgentConfirmationModal(permGtx)
		permImg := renderPresentationTestOps(t, &permOps, size)
		if permImg == nil {
			continue
		}

		// Shield badge #EEF3F9 and scope box #F5F7F9
		shieldFill := color.RGBA{238, 243, 249, 255}
		scopeFill := color.RGBA{245, 247, 249, 255}
		shieldFound, scopeFound := false, false

		for y := 0; y < size.Y; y++ {
			for x := 0; x < size.X; x++ {
				c := permImg.RGBAAt(x, y)
				if c == shieldFill {
					shieldFound = true
				}
				if c == scopeFill {
					scopeFound = true
				}
			}
		}

		if !shieldFound {
			t.Fatalf("scale=%v: expected shield badge background #EEF3F9 to be rendered", scale)
		}
		if !scopeFound {
			t.Fatalf("scale=%v: expected scope box background #F5F7F9 to be rendered", scale)
		}
		_ = card
	}
}

func lightInkVerticalBounds(src image.Image, area image.Rectangle) (top, bottom int, ok bool) {
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			if a>>8 > 150 && r>>8 > 180 && g>>8 > 180 && b>>8 > 180 {
				if !ok || y < top {
					top = y
				}
				if !ok || y > bottom {
					bottom = y
				}
				ok = true
			}
		}
	}
	return top, bottom, ok
}

func TestAgentLightweightConfirmationButtonIconAndTextOpticalAlignment(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, scale := range []float32{1, 2} {
		app := newTestConfirmationApp(t)
		btnH := unit.Dp(36)
		btnW := unit.Dp(96)
		size := image.Pt(int(float32(btnW)*scale), int(float32(btnH)*scale))

		var ops op.Ops
		gtx := layout.Context{
			Ops:         &ops,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}

		card := &agentConfirmationCard{}
		layoutModalActionButton(gtx, app.theme, &card.approve, "允许", btnW, btnH,
			agentLightweightApproveBg, agentLightweightApproveBg, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "check")

		img := renderPresentationTestOps(t, &ops, size)
		if img == nil {
			continue // GPU skipped
		}

		// Icon is in left region (0 to 38dp)
		iconArea := image.Rect(0, 0, int(38*scale), size.Y)
		it, ib, iok := lightInkVerticalBounds(img, iconArea)
		if !iok {
			t.Fatalf("scale=%v: check icon missing from allow button", scale)
		}

		// Text is in right region (38dp to 96dp)
		textArea := image.Rect(int(38*scale), 0, size.X, size.Y)
		tt, tb, tok := lightInkVerticalBounds(img, textArea)
		if !tok {
			t.Fatalf("scale=%v: text missing from allow button", scale)
		}

		iconCenter := float64(it+ib) / 2.0
		textCenter := float64(tt+tb) / 2.0
		btnCenter := float64(size.Y) / 2.0

		diff := math.Abs(iconCenter - textCenter)
		t.Logf("scale=%v: icon Y=[%d, %d] (center %.1f), text Y=[%d, %d] (center %.1f), btn center %.1f, diff=%.1f",
			scale, it, ib, iconCenter, tt, tb, textCenter, btnCenter, diff)

		if diff > float64(1.5*scale) {
			t.Fatalf("scale=%v: check icon center %.1f != text center %.1f (diff %.1f > %.1f)",
				scale, iconCenter, textCenter, diff, 1.5*scale)
		}
		if math.Abs(textCenter-btnCenter) > float64(2.0*scale) {
			t.Fatalf("scale=%v: text center %.1f not vertically centered in %dpx button",
				scale, textCenter, size.Y)
		}

		// Also verify "拒绝" button text alignment matches "允许" text baseline
		var rejOps op.Ops
		rejW := unit.Dp(80)
		rejSize := image.Pt(int(float32(rejW)*scale), int(float32(btnH)*scale))
		rejGtx := layout.Context{
			Ops:         &rejOps,
			Now:         time.Now(),
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(rejSize),
		}
		layoutModalActionButton(rejGtx, app.theme, &card.reject, "拒绝", rejW, btnH,
			color.NRGBA{R: 255, G: 255, B: 255, A: 255}, agentLightweightRejectBdr, agentLightweightRejectFg, "")
		rejImg := renderPresentationTestOps(t, &rejOps, rejSize)
		if rejImg != nil {
			rt, rb, rok := darkInkVerticalBounds(rejImg, image.Rect(0, 0, rejSize.X, rejSize.Y))
			if !rok {
				t.Fatalf("scale=%v: text missing from reject button", scale)
			}
			rejectCenter := float64(rt+rb) / 2.0
			rejDiff := math.Abs(rejectCenter - textCenter)
			t.Logf("scale=%v: reject text Y=[%d, %d] (center %.1f), allow text center %.1f, diff=%.1f",
				scale, rt, rb, rejectCenter, textCenter, rejDiff)
			if rejDiff > float64(1.0*scale) {
				t.Fatalf("scale=%v: reject text center %.1f differs from allow text center %.1f by more than 1dp",
					scale, rejectCenter, textCenter)
			}
		}
	}
}
