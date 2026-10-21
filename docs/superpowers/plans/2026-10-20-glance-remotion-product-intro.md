# Glance Remotion Product Intro Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Execution mode will be selected after the user reviews this plan. Track completed work with checkbox (`- [ ]`) syntax.

## 2026-10-21 approved revision

The user approved execution in the current workspace and explicitly asked for no commits. This revision supersedes all prior fixed 30-second/900-frame constraints and the earlier platform-size voiceover; Tasks 1–8 document earlier stages. The current cut follows the approved final script and the latest design spec:

- Describe Glance as an evolving, lightweight, native office product for ordinary daily needs, without framing it as a replacement for mature office suites.
- Use the approved developer narration, including the whole-month October line, the Vibe Coding/2-million-row CSV line, the double-click/USB line, and the comment CTA.
- Keep all seven Pen `T5ajtB` file icons and the 3D open-box flow; preserve the complete 28.7-second CSV source recording and its full frame.
- Show AI from the user-supplied 13–27s interaction recording at full 2546×1606 frame, without claiming a completed answer.
- Use the confirmed MiniMax Southern Young Man narration and instrumental bed. Do not add or use sound effects.
- Derive composition duration and scene timing from generated narration; no maximum length is set.
- Deliver `my-video/out/glance-intro-vertical.mp4`. Render-based media inspection is the verification; do not add or run automated tests unless the user asks.

**Goal:** Produce a polished, narration-length, 9:16 Mandarin product introduction for Glance as a reproducible Remotion project and H.264 MP4.

**Architecture:** Keep the Remotion toolchain and media in `my-video/`, separate from the Go application build. One composition derives its frame count and scene ranges from the measured narration; shared typography, app-window, captions, background, and transitions stay frame-based.

**Tech Stack:** TypeScript, React, Remotion, FFmpeg for media inspection/preparation, H.264 MP4 output.

**Spec:** [2026-10-20-glance-remotion-product-intro-design.md](../specs/2026-10-20-glance-remotion-product-intro-design.md)

## Global Constraints

- Do not change Glance product source, Go dependencies, or product build configuration.
- Keep Node/Remotion dependencies isolated under `my-video/`; render output goes to `my-video/out/glance-intro-vertical.mp4` and `out/` is ignored.
- Composition is 1080×1920 at 30 fps; duration follows the narration and preserves caption/CTA safe areas.
- Use the Glance brand mark and the complete CSV recording. Preserve the complete CSV source and application canvas with no crop or masks; accelerate it only to fit its assigned scene.
- Keep claims tied to visible proof. Do not imply complete format support or a completed AI response from a partial interaction clip.
- Use the confirmed synthetic Mandarin narration and instrumental music; no sound effects. Do not clone a person's voice or publish the video externally.
- Preserve the final approved developer-led script: Glance is not trying to replace mature suites; it aims to serve ordinary daily office needs with a lightweight, open-and-use product, file viewing and editing, AI built into the base, native portability, ongoing refinement, and a comment invitation.
- Do not commit files as part of implementation unless the user later asks.

## Review Focus

- At 9:16, the complete source video frame remains visible in establishing shots and captions stay legible without covering the relevant UI.
- The complete CSV source recording is shown at full frame, per the user's direction; do not crop, mask, or cover product UI.
- The AI interaction shows the current spreadsheet and invocation context without presenting its intermediate reasoning as a final answer.
- All seven Pen file icons animate through the opening box and flowing workspace sequence.
- Narration determines the edit length; captions match the final spoken words, and music does not mask speech or clip.
- Motion has clear emphasis and varied direction while keeping the product interface readable.

---

### Task 1: Scaffold the isolated Remotion project

**Files:**
- Create: `my-video/package.json`
- Create: `my-video/pnpm-lock.yaml`
- Create: `my-video/tsconfig.json`
- Create: `my-video/remotion.config.ts`
- Create: `my-video/.gitignore`
- Create: `my-video/src/index.ts`
- Create: `my-video/src/Root.tsx`
- Create: `my-video/src/GlanceIntro.tsx`
- Create: `my-video/src/data/shot-timing.ts`

**Interfaces:**
- Register one composition named `GlanceIntro`, with width 1080, height 1920, fps 30, and duration 900 frames.
- Keep composition registration in `src/index.ts`; keep dimensions, frame rate, and output settings in one place rather than duplicating constants across scenes.
- Add `studio` and `render` npm scripts. The render script targets `GlanceIntro` and writes to `out/glance-intro-vertical.mp4`.

- [x] Create a minimal Remotion TypeScript project and lock compatible package versions with pnpm.
- [x] Configure `out/` as generated output and ignore it; keep source assets and source code versionable.
- [x] Add the five approved time blocks: 0–4s, 4–9s, 9–15s, 15–23s, and 23–30s.
- [x] Make a low-resolution full-composition render and confirm the project opens and the composition dimensions/duration are correct. Remotion Studio was not opened separately.

### Task 2: Audit, prepare, and stage safe media assets

**Files:**
- Create: `my-video/public/assets/`
- Create: `my-video/src/data/assets.ts`
- Source media to inspect: `/Users/mima1234/Desktop/glance/录屏2026-10-20 20.17.56.mov`
- Source stills to inspect: `/Users/mima1234/Desktop/glance/1.png`, `/Users/mima1234/Desktop/glance/2.png`
- Brand source: `internal/desktopui/assets/brand/glance-dock-icon.svg`

**Interfaces:**
- `assets.ts` maps stable asset names to files in `public/assets/`; scenes refer to these names instead of embedding source-machine paths.
- The full supplied recording is remuxed as one continuous video asset. Keep the source original untouched.

- [x] Inspect the full recording and both stills at useful resolution. The user confirmed the displayed filenames/data are demo-only and directed that the recording remain complete.
- [x] Preserve all 64.9167 seconds of source picture and the full application canvas. Remux the H.264 stream to MP4 without re-encoding or crop; accelerate it to fit the approved composition.
- [x] Copy the full recording and the Glance brand mark into `public/assets/`; keep the original source untouched.
- [x] Record source duration, remux method, playback rate, audio absence, and package-size source details in `assets.ts` or `my-video/README.md`.

### Task 3: Build the visual system and opening/closing scenes

**Files:**
- Create: `my-video/src/styles/tokens.ts`
- Create: `my-video/src/components/Background.tsx`
- Create: `my-video/src/components/DemoWindow.tsx`
- Create: `my-video/src/components/Caption.tsx`
- Create: `my-video/src/components/Transition.tsx`
- Create: `my-video/src/scenes/IntroScene.tsx`
- Create: `my-video/src/scenes/OutroScene.tsx`
- Modify: `my-video/src/GlanceIntro.tsx`

**Interfaces:**
- `DemoWindow` plays the complete source video continuously, fits the full 2554×1640 canvas without crop, and uses frame-based playback speed to include the entire source in the 30-second composition.
- `Caption` accepts text and optional emphasis segments, with placement confined to the approved safe zones.
- Scene timing and transition behavior use composition frame numbers; avoid duration values derived from wall-clock timers.

- [x] Define the dark navy/cold blue palette, type scale, spacing, safe areas, and motion easing in shared tokens.
- [x] Build reusable full-window framing, caption, and transition components. Confirm CJK glyphs render in the local Remotion environment.
- [x] Implement the 0–4s developer-led opener with Glance identity, complete app shell, and the short line “我是老王，正在打磨 Glance。”
- [x] Implement the 23–30s close with the brand mark and “欢迎持续关注老王” CTA; keep enough hold time for the CTA to be read.
- [x] Preview these sections in the vertical composition and adjust scale/caption placement before building the denser middle shots.

### Task 4: Implement the package, launch, and document-format scenes

**Files:**
- Create: `my-video/src/scenes/PackageLaunchScene.tsx`
- Create: `my-video/src/scenes/DocumentScene.tsx`
- Create: `my-video/src/components/PackageCard.tsx`
- Create: `my-video/src/data/claims.ts`
- Modify: `my-video/src/GlanceIntro.tsx`

**Interfaces:**
- `claims.ts` stores platform, package type, display size, and source artifact name together so a size cannot be shown without its platform/type label.
- `PackageCard` receives one complete platform claim and never infers package type from the size alone.

- [x] Re-read the current `dist/` artifact sizes and encode rounded display values with precise labels: macOS ARM64 portable ZIP, macOS Intel portable ZIP, and Windows single-file EXE.
- [x] Animate the 4–9s platform cards, then transition into reviewed real startup footage that lands on the workspace. Keep startup wording qualitative.
- [x] Build the 9–15s montage from reviewed Glance screens for Word, Excel, PowerPoint, PDF, Markdown, and JSON where the source material supports them.
- [x] Show “Go + Gio 原生桌面” as a concise technology marker and keep the main app shell intact in the hero shot.
- [x] Time the representative editing push-in so its key content remains readable and the movement does not hide the surrounding product context.

### Task 5: Implement the Agent-first collaboration scene

**Files:**
- Create: `my-video/src/scenes/AgentScene.tsx`
- Create: `my-video/src/components/SharedDocumentDiagram.tsx`
- Modify: `my-video/src/GlanceIntro.tsx`

**Interfaces:**
- `SharedDocumentDiagram` presents the desktop interface and Agent connecting to the same document capability/runtime layer; labels remain brief and technically bounded.
- Agent visuals use either reviewed real UI or a clearly illustrative diagram, and do not depict unsupported edits as product evidence.

- [x] Compose the 15–23s sequence with the current document UI as the anchor and the Agent entering as a first-class collaborator.
- [x] Animate a clear shared-capability connection between desktop and Agent, then return attention to the document.
- [x] Use the approved claim “和桌面共用同一套文档能力” and supporting label “围绕当前文档协作”.
- [x] Check that diagram labels, arrows, and app UI stay legible together at phone preview size.

### Task 6: Finalize narration, captions, music, and mix

**Files:**
- Create: `my-video/public/audio/`
- Create: `my-video/src/data/narration.ts`
- Create: `my-video/src/components/TimedCaptions.tsx`
- Modify: `my-video/src/GlanceIntro.tsx`

**Interfaces:**
- `narration.ts` maps each spoken line to an audio asset and frame range; caption text is derived from or checked against the final recorded line.
- Music and effects use explicit volume levels and frame-based start/end times.

- [x] Generate synthetic Mandarin narration with a conversational script and time each line to the 30-second arc.
- [x] Generate an instrumental tech bed and restrained transition effects locally; no external stock assets were used.
- [x] Add captions synchronized to the narration, preserving the approved developer voice and avoiding dense simultaneous text.
- [x] Mix voice above music and trim all assets to the composition length. Audio peak is -5.9 dBFS; a human full listen-through remains for final subjective review.

### Task 7: Render, inspect, and deliver the MP4

**Files:**
- Modify as needed: `my-video/src/**`
- Output: `my-video/out/glance-intro-vertical.mp4`
- Optional notes: `my-video/README.md`

- [x] Render the full 1080×1920, 30 fps composition as H.264 MP4 using the project render script.
- [x] Inspect a final still sheet sampled every three seconds, including the opener, scene changes, Agent diagram, and CTA. A continuous human watch-through with sound remains for review.
- [x] Check phone-scale readability, safe zones, app shell visibility, source footage framing, platform-size labels, narration/caption sync, and audio levels; fix the opening title line break and rerender.
- [x] Confirm the final artifact path, dimensions, and duration; keep product claims tied to visible product footage.
- [x] Do not run automated tests unless the user asks; no tests were run. Do not commit implementation files.

### Task 8: Apply the 2026-10-21 story and camera revision

**Files:**
- Modify: `my-video/src/scenes/CsvPreviewScene.tsx`
- Modify: `my-video/src/scenes/AIFoundationScene.tsx`
- Modify: `my-video/src/data/assets.ts`
- Add: `my-video/public/assets/ai-current-doc-demo.mp4`
- Modify: project README and this spec/plan
- Output: `my-video/out/glance-intro-vertical.mp4`

- [x] Use the user's complete 2-million-row CSV recording and animate the full-frame window with zoom and position changes.
- [x] Use AI interaction recording source seconds 13–27, preserving its full canvas; animate the camera and the selected T5ajtB CSV/PDF icons toward the AI base.
- [x] Retain the confirmed narration and instrumental music while excluding all sound effects.
- [x] Render the 1080×1920, 30 fps, 900-frame MP4. Inspect key frames and verify that video and audio tracks are present and span the composition.
- [x] Keep all files uncommitted.

**Review note:** The final render was inspected visually at three-second intervals, and its audio stream was checked for format, duration, and peak level. A human continuous listen-through is still recommended for subjective voice/music balance and transitions.

## Self-review

- The revised cut covers the approved local-preview, limited-basics, AI-base, dynamic-camera, and developer-led story from the design spec.
- The complete CSV frame remains visible; the AI excerpt shows active document context and invocation without claiming a complete result.
- Verification is render-based and manual because the requested deliverable is a video; no automated tests were added or run.
- The user approved current-workspace execution and no commits. Implementation files and the generated MP4 remain uncommitted.
- Independent final review found no critical or important issues. Two nonmaterial documentation mismatches were deferred: the README's old CSV timing and the spec's closing-box description.

### Task 9: Apply the approved dialogue and produce the narration-length cut

**Files:**
- Modify: `my-video/src/data/narration-script.json`
- Modify: `my-video/src/data/narration-timing.json`
- Modify: `my-video/src/scenes/IntroScene.tsx`
- Modify: `my-video/src/scenes/CsvPreviewScene.tsx`
- Modify: `my-video/src/scenes/PackageLaunchScene.tsx`
- Modify: `my-video/src/scenes/OutroScene.tsx`
- Modify: project README, design spec, and this plan
- Output: `my-video/out/glance-intro-vertical.mp4`

- [x] Apply the final user-approved dialogue without dropping the earlier approved core messages or historical copy.
- [x] Remove the fixed 30-second duration and derive the timeline, music length, captions, and scene lengths from actual generated voice audio.
- [x] Remove platform-specific package sizes and all transition sound effects from the active cut; show the approved under-100-MB, double-click, and USB ideas.
- [x] Use the approved cover as frame 0, then show selected full-frame shots from the original product recording in the opening. Retain the file-box animation, complete CSV and AI frames, and comment invitation in the close.
- [x] Regenerate narration and the instrumental bed, render the MP4, and inspect key frames, source-frame integrity, captions, duration, and audio tracks.
- [x] Leave all files uncommitted.

### Task 10: Put the cover at frame 0 and add real footage to the opening

**Files:**
- Modify: `my-video/src/GlanceIntro.tsx`
- Modify: `my-video/src/scenes/IntroScene.tsx`
- Modify: `my-video/src/data/assets.ts`
- Output: `my-video/out/glance-intro-vertical.mp4`

- [x] Show the final Glance cover on frame 0 and transition into product footage.
- [x] Replace the 17.7-second static preview with full-frame source shots of home, Word, Excel, JSON, and document editing; preserve the source recording and vary camera scale, tilt, and position.
- [x] Render the full video, inspect frame 0 and the first 12 seconds, and confirm the audio/video encoding.
- [x] Leave all files uncommitted.
