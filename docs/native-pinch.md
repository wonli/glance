# macOS native trackpad pinch

Glance receives macOS magnification through `NSEvent`'s local event monitor. The monitor is scoped to the NSView/window provided by Gio's public `app.AppKitViewEvent`. It registers and unregisters on the AppKit main queue and is removed when the view changes or its window closes. Its Go registry is removed before asynchronous native teardown, so late events cannot mutate a closed window.

No Gio source changes, local Gio replacement, private symbols, method swizzling, synthetic wheel events or global input monitoring are used. The `gioui.org` module remains managed normally by `go.mod`; upgrading it does not require migrating a fork. The application bridge still needs compilation and interaction regression checks when Gio changes its public AppKit view or semantic APIs.

The native callback only queues relative magnification and window-local coordinates and sends a nonblocking wake notification. A per-view managed background task calls `Window.Invalidate`, and exits on view removal or window shutdown. Calling `Invalidate` directly from the AppKit callback is forbidden: Gio can synchronously reenter event delivery on its main thread while holding its invalidation mutex; the window goroutine can then request another invalidation during zoom and deadlock with it. Document state stays on the Gio window goroutine. Adjacent magnification updates combine multiplicatively to preserve fine deltas. Each gesture binds to the document and body viewport where it began; switching documents, showing a blocking modal or receiving stale window geometry prevents applying the gesture to a different target.

When queued events arrive, the window reads the preceding completed frame's operations through a separate `input.Router` and its public semantic tree. A clipped semantic viewport marker identifies the real body bounds, including tabs, Ribbon, navigation panes, ruler and device scale. This extra operation parsing happens only on gesture frames. It does not run document parsing or file IO. Pinch positions are then normalized to body-local physical pixels before reusing grid/page zoom anchors. Word Web/initial flow layout keeps its existing flow zoom behavior; paginated Word anchors the canonical page point, subject to viewport scroll limits and centered-page constraints.

Spread increases scale; pinch together decreases it. No modifier is required. Existing view zoom limits and status percentages apply. Document text, font sizes, row heights and column widths are not edited by this bridge. Wheel zoom and normal two-finger scrolling keep their existing event paths. Other platforms are unchanged.

## Validation

Run from the repository root:

```sh
go test ./internal/desktopui -run 'Test(NativePinch|SpreadsheetNativePinch|WordNativePinch|Zoom|GridWheelZoom|WordWheelZoom|SpreadsheetBody|WordBody)' -count=1
```

The synthetic regression tests cover relative scale, fine-delta accumulation, coalescing, limits, window offsets, 1x/2x device metrics, grid/page anchors, the Word ruler, modal/menu blocking and stale session frames. `TestNativePinchCallbackNeverWaitsForWindowInvalidation` reproduced the synchronous callback blockage before the handoff fix; `TestNativePinchWorkerHandoffAndShutdown` checks nonblocking callbacks during a busy redraw and view teardown. The native pinch tests pass under `-race`. They do not prove physical trackpad delivery or native monitor teardown under repeated live window creation.

For device validation, start `go run ./cmd/glance` from the source directory. Verify spread/pinch and ordinary two-finger scrolling in both Excel and Word, body boundaries, frozen panes, multi-page Word, tab switches during gestures, modal blocking, and closing/reopening multiple windows. Physical-device validation is pending until exercised with a real trackpad; a successful build is not equivalent to that check.
