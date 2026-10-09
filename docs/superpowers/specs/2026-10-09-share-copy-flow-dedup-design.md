# XLSX and DOCX Share-Copy Flow Deduplication

## Goal

Reduce duplicated implementation in the XLSX and DOCX share-copy export flows while preserving all user-visible behavior and document-safety guarantees.

## Current behavior

The spreadsheet and Word flows each validate a destination, capture a self-contained snapshot in a background worker, reopen that snapshot as an independent format adapter, save it with a commit guard, close the temporary adapter, and publish a result to the UI. Their format-specific preflight checks differ: XLSX checks workbook history state, while DOCX also flushes pending text drafts and checks the document revision and draft epoch.

## Design

Add a same-package `internal/desktopui/share_copy.go` for the mechanics shared by both formats:

- Append the requested format extension when the selected path does not already have it, case-insensitively.
- Save a share copy from a supplied snapshot callback, format-specific snapshot revalidation callback, snapshot opener, and commit-guard callback.
- Apply the common completed-result status behavior using the format label and destination filename.

The snapshot writer accepts a small interface implemented by both reopened format adapters: `SaveAsWithCommitGuard` and `Close`. It captures the snapshot, runs the existing freshness check, opens the copied document, saves it with the existing guard, and closes the copied document. Each per-format worker supplies the format-specific callbacks and keeps its existing background goroutine and result channel.

Keep the following behavior in `spreadsheet_share_copy.go` and `word_share_copy.go`:

- Request and picker lifecycle, including Word draft flushing.
- Active-session, closed-session, busy-state, and stale-picker checks.
- Format-specific history/revision/draft-epoch checks and their existing messages.
- Format-specific commit mutexes and final freshness checks.
- Format adapter imports and snapshot-open functions.
- The result types and asynchronous delivery/shutdown handling.

No public API or package boundary changes. The shared helper stays inside `desktopui` and does not expose third-party Office types.

## Error and safety behavior

Preserve the existing ordering and precedence:

1. Return snapshot errors unchanged.
2. Return the existing format-specific stale-snapshot error before opening a copy.
3. Wrap snapshot-open errors with the current localized format-specific validation prefix.
4. Save using the adapter's existing `SaveAsWithCommitGuard` implementation.
5. Always close an opened copy. A close error becomes the result only when saving succeeded; a save error keeps precedence.

The adapter remains responsible for writing to a temporary sibling file and replacing the destination only after the commit guard succeeds. The freshness callbacks and mutexes remain format-specific and execute at the same points as today.

## Compatibility and regression surface

Keep the existing share-copy tests and assertions unchanged. They cover picker cancellation, successful export without changing the source session, same-source overwrite rejection, stale picker choices, write failure, busy-state cleanup, and Word draft flushing. No new user-visible behavior, status text, file naming rule, or save capability is introduced.

## Scope limits

This pass does not unify ordinary Save/Save As operations, move adapter construction out of `desktopui`, alter snapshot formats, change save internals, or refactor other document workflows.
