# Editable X Proofreading Projection

**Goal:** Let a reviewer compare A with editable B text placed on X's A-derived page skeleton, align B flow independently, and add manual page breaks at exact text positions.

**Architecture:** A contributes page count and geometry only; the proofreading path does not read A text or run OCR. B's loaded adapter remains immutable while X holds an independent editable normalized copy derived from B. X maps that copy's flow pages onto A's fixed page frames, with A/X scroll and zoom synchronized and B-page alignment controlled independently by an overlay ruler.

**Tech Stack:** Go, Gio `widget.Editor`, existing PDF page view and format adapters.

## Constraints

- Do not extract or OCR A text.
- Do not overwrite the selected B source file during in-place editing.
- Keep persistent Gio editors and pointer state in `proofreadingWorkspace`.
- A/X page geometry, page scroll, and pinch zoom remain synchronized.
- The B alignment ruler overlays X and does not reduce its page width.
- Manual breaks are rune anchors in B text and retain the insertion point for their visible marker.
- Do not add or run tests unless explicitly requested; use formatting, build, and diff checks.

## Work completed

- [x] Removed the center ratchet and its one-character pagination adjustment controls.
- [x] Added a persistent multiline editor to each visible X page; edits update the workspace's B text draft, and empty X pages accept added text.
- [x] Made X text zoom global across pages and extended the upper range to 5× for sparse pages.
- [x] Paginated B independently from A's page count and added an overlay ruler that maps the current A page to a selected B flow page using drag or wheel input.
- [x] Kept A and X on the shared PDF page position and zoom state.
- [x] Stored manual page breaks at rune offsets, flowed following B text to the next B page, and retained undo with the marker at the click location.
- [x] Kept all A text extraction and OCR out of the proofreading path.
- [x] Auto-saved X's independent proofreading copy as a human-readable Markdown file in the per-user `Glance Office/proofreading` directory; an HTML metadata header stores source hashes, manual breaks, B-page alignment, and X text size while the body remains plain text for readable diffs. Older workspace-database drafts migrate into the file.
- [x] Formatted changed Go files, validated locale JSON, ran `go build ./...`, and ran `git diff --check`.

## Current limits

- The latest X copy is saved as a separate file while the selected B source stays unchanged. Git history, a revision timeline, and rollback UI are deferred until the proofreading flow is stable.
- X edits normalized plain text. Source formatting and structural editing for Word/Markdown adapters are not preserved in the editable projection.
- Automated difference detection and review-state history are still future work; comparison remains a manual A-to-X review.
