# Chrome theme, demo scene and public README Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship an opt-in chrome 3D theme whose scene reacts to agents, a `demo` command that plays a scripted scene on the real screen saver, and a public README.

**Architecture:** Backgrounds gain an optional `update(state)`, so a WebGL background can draw agents. The theme is four built-in extensions (scene, clock, rings, layout) plus a theme manifest, using only the public API. The demo is a pure Go script (elapsed time to cards and limits) that the daemon swaps into the built state file while a demo plays.

**Tech Stack:** Go (stdlib), TypeScript web runtime built with esbuild, raw WebGL2 (no new dependency), `node --test`.

**Spec:** `docs/superpowers/specs/2026-10-26-chrome-theme-demo-readme-design.md`

## Global Constraints

- Everything visible is an extension using the public API in `web/src/api.ts`. `API_VERSION` stays `1`.
- No network at runtime. No new npm `dependencies`. CSP stays as is (`font-src file:`, `connect-src 'none'`, no eval).
- No company logos, no band artwork, album titles or unlicensed fonts. Fonts: SIL OFL only, licence file shipped beside them.
- English, plain words, no em-dashes in docs, UI copy, comments and commit messages. Document only what exists.
- Conventional commits, explicit paths in `git add`, no AI attribution lines.
- Design skills (emil-design-eng, impeccable, design-taste-frontend) load before any UI task (Tasks 3, 4).
- Commands: `go vet ./...`, `go test ./...`, `cd web && npm test`, `cd web && npm run typecheck`, `cd web && npm run build`.

## Review Focus

1. A background with no `update` (Matrix, aerials, image, video, user extensions) must behave exactly as before. Pinned in Task 1.
2. A real approval arriving during a demo must end the demo and show the real card at once. Pinned in Task 6.
3. Privacy mode on: demo text still shows in full; real cards stay masked after the demo ends. Pinned in Task 6.
4. WebGL2 unavailable or context lost: the chrome scene must fall back to a black background with grain, never a blank error or a thrown exception that stops the components. Pinned in Task 3.
5. Many agents (8 cards) or zero cards: the scene stays readable and does not grow its cost per card without bound (cap the signs drawn at 6). Pinned in Task 3.

## Plan rulings (from the code facts)

- No 3D library. The scene is one WebGL2 fragment shader (signed distance fields, ray marching) drawn at reduced size. This keeps the "no new dependency" line and the bundle small. Cost if wrong: a later switch to a library, contained in one extension.
- Signs show the agent's letter in the chrome font, drawn into a small canvas texture atlas. The SVG marks are not drawn in WebGL (they are `<img>` files; reading their pixels is not worth the risk). Cards elsewhere keep the real marks.
- The chrome theme needs its own layout, `builtin.chrome-layout`, a copy of `builtin.spotlight` with the chrome clock and rings ids. The approval card and task row stay the built-in ones.
- Demo settings live in `config.json` (the daemon already polls its mtime every 500 ms). The daemon swaps the demo cards and limits in after `statefile.Build`, so privacy masking never touches them.
- Demo attention drives the sound: the daemon feeds the attention of the file it actually writes into `ShouldPlaySound`.

---

### Task 1: Backgrounds get the view state

**Files:**
- Modify: `web/src/api.ts` (`BackgroundExt`), `web/src/runtime.ts` (background branch of `launch()`, `reconcile()`, `tickClock()`)
- Create: `web/test/background-update.test.ts`
- Modify: `docs/extensions.md` (background section, API types), `docs/guides/add-a-background.md` (new "React to your agents" step)
- Create: `docs/guides/examples/<id>/` (a tiny background that tints by `state.mode`), `internal/settings/guides_examples_test.go` gains a check that the example entry file defines `update`

**Interfaces:**
- Produces:
  ```ts
  export interface BackgroundExt {
    start(root: HTMLElement, ctx: Ctx): void;
    /** Optional. Called after start and whenever the view state changes, like a component's update. */
    update?(state: ViewState): void;
    stop(): void;
  }
  ```

- [ ] **Step 1: Write the failing tests** in `web/test/background-update.test.ts`, using the existing `fakeItem` / `keeper()` style from `web/test/instance.test.ts`. If `runtime.ts` cannot be driven from a test as it is, first extract the fan-out into a small exported function (for example `fanOut(items, state)` in `web/src/instance.ts`) and test that. Cases:
  - `background with update gets the state after start`
  - `background with update gets every later state`
  - `background without update is started and stopped as before and never called with state`
  - `background update that throws does not stop components from updating` (same isolation the components already have)
- [ ] **Step 2:** Run `cd web && npm test`. Expected: the new tests fail.
- [ ] **Step 3:** Implement: set `item.update` in the background branch of `launch()` when the extension has `update`; include `background.item` in both `updateAll` calls (`reconcile()` and `tickClock()`).
- [ ] **Step 4:** Run `npm test` and `npm run typecheck`. Expected: all pass.
- [ ] **Step 5:** Docs and the guide example. Run `go test ./internal/settings/`. Expected: pass.
- [ ] **Step 6: Commit** `feat(web): pass the view state to backgrounds that ask for it`

### Task 2: Chrome fonts and the shared font loader

**Files:**
- Create: `extensions/builtin.chrome-clock/fonts/` and `extensions/builtin.chrome-rings/fonts/` (the font files each component uses, plus `OFL.txt` in each), `web/src/builtins/shared/fonts.ts`, `web/test/fonts.test.ts`

**Interfaces:**
- Produces: `export function loadFont(family: string, url: string, descriptors?: FontFaceDescriptors): Promise<boolean>` in `web/src/builtins/shared/fonts.ts`. It resolves `false` (never throws) when the font cannot load, so a component falls back to the system font. It loads each family once per page.

- [ ] **Step 1:** Pick fonts: a condensed display face and a monospace face, both SIL OFL (for example Anton and JetBrains Mono, or close alternatives). Download from the official source repository (`github.com/google/fonts` `ofl/` folder or the font's own repo) as `.woff2` when available, else `.ttf`. Record the source URL and licence in each `fonts/OFL.txt` header. If the download fails on this machine, stop and report: the owner fetches the files.
- [ ] **Step 2: Write the failing tests** (`web/test/fonts.test.ts`, with a fake `FontFace` and `document.fonts`): `loads a family once even when asked twice`; `resolves false when the font fails to load`; `resolves false when FontFace is missing`.
- [ ] **Step 3:** Run `npm test`. Expected: fail. Implement `loadFont`. Run again. Expected: pass.
- [ ] **Step 4:** Check the build ships the fonts: `npm run build`, then confirm `build/extensions/builtin.chrome-clock/fonts/` exists.
- [ ] **Step 5: Commit** `feat(web): add open-licence chrome fonts and a shared font loader`

### Task 3: The chrome scene background

Load emil-design-eng, impeccable and design-taste-frontend before writing visual code.

**Files:**
- Create: `extensions/builtin.chrome-scene/extension.json`, `web/src/builtins/chrome-scene/index.ts` (WebGL setup, loop, `update`), `web/src/builtins/chrome-scene/logic.ts` (pure), `web/src/builtins/chrome-scene/shader.ts` (GLSL strings), `web/test/chrome-scene.test.ts`

**Interfaces:**
- Consumes: `BackgroundExt.update` (Task 1), `ViewState`, `Ctx.requestFrame(cb, fps)`.
- Produces (in `logic.ts`):
  ```ts
  export type SignState = "running" | "finished" | "approval" | "idle";
  export interface Sign { source: string; letter: string; state: SignState; flashStartMs?: number; }
  export const MAX_SIGNS = 6;
  /** One sign per card, newest first, capped at MAX_SIGNS; none on a secondary display. A card that just turned finished gets flashStartMs. */
  export function signsFrom(state: ViewState, previous: readonly Sign[], nowMs: number): Sign[];
  /** 0 = calm, 1 = full pink; eases toward 1 while any approval is open. */
  export function pinkTarget(state: ViewState): number;
  /** Render size for a screen size: at most half in each direction, at least 320 wide. */
  export function renderSize(width: number, height: number, scale: number): { width: number; height: number };
  ```
- Manifest options: `fps` (number, default 30, min 10, max 60), `scale` (number, default 0.5, min 0.25, max 1), `grain` (number, default 0.6, min 0, max 1).

- [ ] **Step 1: Write the failing tests** for `logic.ts`:
  - `one sign per card with the agent's first letter`
  - `approval card gives an approval sign`
  - `card that turns finished gets a flash start, an old finished card does not`
  - `eight cards give six signs`
  - `secondary display gives no signs`
  - `pink target is 1 with an open approval and 0 without`
  - `render size is half the screen and never below 320 wide`
- [ ] **Step 2:** Run `npm test`. Expected: fail. Implement `logic.ts`. Run again. Expected: pass.
- [ ] **Step 3:** Implement `index.ts` and `shader.ts`. One full-screen triangle; one fragment shader with: a slow warping chrome shape lit by a procedural environment, up to `MAX_SIGNS` chrome slabs carrying letters from a canvas atlas texture (letters drawn with the chrome display font via `loadFont`), lime flash and pink glitch from uniforms, then grain, scan lines and light chromatic offset. Draw at `renderSize`, let CSS scale the canvas up. Loop through `ctx.requestFrame(draw, fps)`; `stop()` cancels it and releases the GL context. `prefers-reduced-motion`: freeze the warp, replace the glitch with a color change.
- [ ] **Step 4: Failure paths** (tests with a fake canvas whose `getContext("webgl2")` returns `null`, and one that fires `webglcontextlost`): `no WebGL2 falls back to black with CSS grain and does not throw`; `context lost stops the loop and falls back`; `update before start does not throw`.
- [ ] **Step 5:** Run `npm test`, `npm run typecheck`, `npm run build`. Expected: pass. Preview: open `web/build` runtime in the built-in browser with a fixture state (the existing dev preview, if any; else a scratch HTML page in the scratchpad that loads the built runtime and calls `window.agentScreensaver.setState`) and take screenshots of idle, running, approval. Show them to the owner.
- [ ] **Step 6: Commit** `feat(web): add the chrome scene background`

### Task 4: Chrome clock, chrome rings, layout and theme

Load the design skills first.

**Files:**
- Create: `extensions/builtin.chrome-clock/extension.json`, `web/src/builtins/chrome-clock/index.ts`; `extensions/builtin.chrome-rings/extension.json`, `web/src/builtins/chrome-rings/index.ts`; `extensions/builtin.chrome-layout/extension.json`; `extensions/builtin.chrome/extension.json`; tests `web/test/chrome-clock.test.ts`, `web/test/chrome-rings.test.ts`
- Modify: `docs/extensions.md` (the built-in themes line), `internal/extensions` tests if they list built-in ids

**Interfaces:**
- Consumes: `loadFont` (Task 2); the formatting helpers the built-in clock and limit rings already use (`web/src/format.ts`, `web/src/builtins/shared/`). Reuse them; do not copy logic.
- Produces: `builtin.chrome-clock` (options `size`: `large` | `small`, like `builtin.clock`), `builtin.chrome-rings` (no options), `builtin.chrome-layout` (the spotlight modes with the two chrome ids), `builtin.chrome` theme: background `builtin.chrome-scene`, layout `builtin.chrome-layout`, style `--accent: #c8ff2e`, `--attention: #ff3d9a`, `--done: #c8ff2e`, `--card-opacity: 0.94` (starting values; tune with the design skills).

- [ ] **Step 1: Write the failing tests:** clock shows the same time text as `builtin.clock` for the same `nowMs`; clock `small` and `large` sizes; rings show one ring per limit in `agentOrder`, the reset countdown text and the "est." mark exactly as `builtin.limit-rings` does; both still render with the system font when `loadFont` resolves `false`.
- [ ] **Step 2:** Run `npm test`. Expected: fail. Implement. Run again. Expected: pass.
- [ ] **Step 3:** Manifests for layout and theme. Run `go test ./...` (manifest validation and built-in id lists). Expected: pass.
- [ ] **Step 4:** Preview screenshots as in Task 3 Step 5 (idle, approval, secondary display). The approval card must stay fully readable. Show them to the owner.
- [ ] **Step 5: Commit** `feat(web): add the chrome theme`

### Task 5: Demo script

**Files:**
- Create: `internal/demo/demo.go`, `internal/demo/demo_test.go`

**Interfaces:**
- Consumes: `state.Card`, `state.Limit` (`internal/state/state.go`).
- Produces:
  ```go
  // Loop is the length of one pass of the scripted scene.
  const Loop = 30 * time.Second
  // Scene returns the cards and limits to show elapsedMs into a demo of totalMs, at wall time nowMs.
  func Scene(elapsedMs, totalMs, nowMs int64) ([]state.Card, []state.Limit)
  ```
  Values from the spec, Part 3 (sources `claude`, `codex`, `cursor`; titles "refactor auth middleware", "add rate limit tests", a made-up Cursor title; request "Bash: npm run migrate"; Claude context 34%, branch `feat/auth`; limits Claude 5h 42% to 61% over `totalMs`, week 28%, Codex 5h 17%, week 9%, resets set from `nowMs`).

- [ ] **Step 1: Write the failing table test** `TestScene`: at 0 s three running cards; at 7 s Codex finished; at 11 s Claude approval with the request text; at 19 s Claude running again; at 25 s all finished; at 31 s the loop restarted (same as 1 s); Claude 5h limit is 42 at 0, 61 at `totalMs`, and in between at the halfway point; no card text contains a real path or user name (all strings come from the script).
- [ ] **Step 2:** Run `go test ./internal/demo/`. Expected: fail. Implement. Run again. Expected: pass.
- [ ] **Step 3: Commit** `feat(demo): add the scripted demo scene`

### Task 6: Daemon and CLI wiring for the demo

**Files:**
- Modify: `internal/config/config.go` (demo fields), `internal/daemon/daemon.go` (`Step`, `writeState`), `internal/cli/settings.go` (`demo` in `settingsCommands`), `cmd/agent-screensaver/main.go` (`subcommands`, usage text), their tests
- Modify: `docs/` CLI reference wherever subcommands are listed (find with `rg -n "privacy" docs README.md`)

**Interfaces:**
- Consumes: `demo.Scene` (Task 5), `ShouldPlaySound`, `watcher.Active()` via `Step(nowMs, saverActive)`.
- Produces:
  - `config.Config` fields: `DemoSeconds int \`json:"demo_seconds,omitempty"\``, `DemoArmedMs int64 \`json:"demo_armed_ms,omitempty"\``.
  - CLI: `agent-screensaver demo [--seconds N]` (default 60, 10 to 600) sets both fields and prints "Demo ready. Start the screen saver now; it plays for N seconds."; `agent-screensaver demo stop` clears them.
  - Daemon behavior: armed and the saver becomes active: the demo starts (start time kept in memory). While it plays, after `statefile.Build`, the file's cards and limits are replaced with `demo.Scene(...)`. It ends (and clears the config fields) when: N seconds pass; the armed demo is older than 5 minutes without the saver showing; a real card in `d.state` is an approval; `demo stop` clears the config. The attention used for `ShouldPlaySound` is the attention of the written file.

- [ ] **Step 1: Write the failing tests** (patterns: `newTestDaemon`, `fakeWatcher`, time as a parameter, `paths.ForHome(t.TempDir())`):
  - `TestDemoStartsWhenSaverShows` (armed, saver inactive: real state written; saver active: demo cards written)
  - `TestDemoEndsAfterItsSeconds` (and the config fields are cleared)
  - `TestArmedDemoExpiresWithoutSaver` (5 minutes)
  - `TestRealApprovalEndsDemo` (the next written file holds the real approval card)
  - `TestDemoTextIsNotMaskedInPrivacyMode` and `TestRealCardsMaskedAfterDemo`
  - `TestDemoApprovalPlaysSound` (once, at the scripted approval)
  - `TestDemoCommand` (sets fields, rejects `--seconds 5` and `601`, `demo stop` clears)
- [ ] **Step 2:** Run `go test ./internal/... ./cmd/...`. Expected: fail. Implement. Run again. Expected: pass.
- [ ] **Step 3:** Run `go vet ./...` and `go test ./...`. Expected: pass.
- [ ] **Step 4: Commit** `feat(daemon): play a scripted demo scene on the screen saver`

### Task 7: Owner checkpoint (manual)

- [ ] **Step 1:** Install the build on the owner's Mac (the owner runs the install command; the agent never starts ScreenSaverEngine).
- [ ] **Step 2:** The owner picks the Chrome theme, runs `agent-screensaver demo`, starts the saver, and watches one full demo.
- [ ] **Step 3:** CPU: the owner measures with the method in `docs/decisions/0002-*.md` (`top -l 3 -s 10`, first sample dropped) for Matrix and for Chrome during the demo. Record both numbers in the ledger. Target: Chrome at most about 3 times Matrix. If over: lower the `scale` default first, then `fps`, and measure again.
- [ ] **Step 4:** The owner approves the look or lists changes. Changes loop back to Task 3 or 4 before Task 8.

### Task 8: README, media and docs

**Files:**
- Modify: `README.md` (full rewrite), `docs/known-limits.md` if a new limit appears (for example WebGL2 needed by the Chrome theme)
- Create: `docs/media/README.md` (what each file is and how to record it), `docs/media/how-it-works.svg` (or a Mermaid block in the README; builder's choice)

**Interfaces:**
- Consumes: everything above; `scripts/build-release.sh` output name `agent-screensaver-<version>-macos.zip`; repo `github.com/stavgayer/agent-screensaver`.

- [ ] **Step 1:** Write the README in the spec's order (Part 4). Hero: a committed GIF path `docs/media/demo.gif` and an HTML comment marking where the owner pastes the GitHub-uploaded video link. Screenshot paths under `docs/media/` with the same marking until the owner records them. All links relative.
- [ ] **Step 2:** Check every claim against the code: each command exists (`agent-screensaver` usage output), each linked file exists (a script that extracts relative links from `README.md` and checks each path exists), each agent capability in the table matches `docs/agents.md` and `docs/known-limits.md`.
- [ ] **Step 3:** `docs/media/README.md`: the exact recording steps (pick theme, `agent-screensaver demo`, start the saver, Cmd+Shift+5, 20 seconds, convert to GIF with a stated free tool, upload the .mp4 in GitHub's web editor).
- [ ] **Step 4: Commit** `docs: write the public README`

### Final gate

- [ ] `go vet ./...`, `go test ./...`, `cd web && npm test`, `npm run typecheck`, `npm run build`, `scripts/build-release.sh`. All pass.
- [ ] Whole-branch review on the most capable model against the spec.
