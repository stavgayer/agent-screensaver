# Agent Screen Saver v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A real macOS screen saver that shows Claude Code and Codex agent status and usage limits over a swappable background, built so that every visible part is an extension.

**Architecture:** Hooks and the Claude status line run `agent-screensaver hook|statusline`, which drop one JSON event file each into a spool folder. A Go daemon drains the spool, keeps the state, polls `codex app-server` for Codex limits, plays the sound, and writes one `state.json` that also carries the resolved view (theme, layout, extensions, options). A thin Swift `.saver` hosts a `WKWebView` that loads a TypeScript runtime; the runtime mounts background and component extensions into layout regions and feeds them the state.

**Tech Stack:** Go >= 1.23, standard library first (`encoding/json`, `archive/zip`, `regexp`, `os/exec`, `flag`, `testing`); cgo + one Objective-C file only for macOS screen saver notifications. No third-party Go modules except `fyne.io/systray`, used only by the menu bar app (Phase 5, spec §7c). The settings window is our own cgo + WKWebView window (owner decision after Task 5.1). Swift 6 + ScreenSaver + WebKit, built with `swiftc` (no Xcode project). TypeScript 5 compiled with esbuild, tested with `node --test` (Node >= 22.18, type stripping). JSON for every data format.

**Spec:** `docs/superpowers/specs/2026-10-20-agent-screensaver-design.md` (read it before any task; sections are cited as "spec §N").

**Go test style (owner's go-testing rules):** table-driven tests with `t.Run(tt.name, ...)`; fixtures in each package's `testdata/`; `t.TempDir()` for every file test; anything that runs real commands or the built binary is an integration test that skips under `go test -short`; system calls that tests must avoid (for example `launchctl`) sit behind a small interface with a fake in tests.

## Global Constraints

- macOS 14 or later, universal build (arm64 + x86_64) for both the Go binary and the `.saver`.
- No network calls anywhere except the local `codex app-server` child process. No analytics. No undocumented endpoints, no browser cookies, no copying of login tokens.
- Never download, store, or ship Apple video files. Aerials play only files macOS already downloaded.
- Frame rate (spec §5): no fixed project-wide cap. Each background sets its own rate as an option (Matrix default 24). The runtime enforces one ceiling for every background: the user's `max_fps` (config; default = display refresh rate; allowed 1..240).
- Card titles: 40 characters max, secrets masked to `•••` before truncation. Privacy mode (default off) removes titles and requests.
- Every built-in visible part (Matrix, clock, rings, task row, approval card, layout, theme) is an extension loaded through the same public API as outside extensions. No private shortcuts.
- Web extensions: no network (CSP `connect-src 'none'`), no file access beyond the data dir, errors isolated per extension.
- Installing never replaces or removes existing user hooks or settings; it only adds, and uninstall removes exactly what install added. Editing a user JSON file keeps its key order and leaves every value we did not touch byte-for-byte the same (use `internal/jsonedit`, never `map[string]any` round trips).
- Hook and status line commands must always exit 0 and finish fast; they must never block or slow Claude Code or Codex.
- `.saver` is signed ad hoc only (`codesign -s - --deep --force`); no Apple Developer account.
- Commits: conventional commits, no AI attribution or co-author lines. Stage explicit paths only, never directories or `-A`.
- TypeScript: no `enum` or `namespace` (Node type stripping cannot run them); use string unions.
- Timing constants (spec §5, §6): finished card TTL 30 min; running with no event for 60 min becomes `no_activity`; any card with no event for 12 h is removed; approval with no event for 10 min becomes `unknown`; Codex limits poll every 5 min while the saver shows plus once after each Codex `finished`; status line passthrough timeout 300 ms.
- Every Go package passes `go vet`; `go test -race ./...` passes.
- Code principles (spec §7b): simple, readable, efficient, extendable, idiomatic, modular, testable, DRY, with the concrete meaning given in the spec table. Every task's reviewer checks its diff against that table, not only its tests. Priority when they conflict: correct, then simple and readable, then efficient.
- No hidden inputs: functions take the clock (`nowMs` or `now func() int64`), paths (`paths.Paths`) and system calls (small interfaces) as parameters, so tests never depend on the real time, `HOME`, or system state.

## Review Focus

1. Claude `Notification` fires for "waiting for your input" (idle) as well as for permission prompts. An idle notification must never create an approval card. Test: Task 1.3 case `claude idle notification is ignored`.
2. A hook gets empty, malformed, or huge stdin, or the spool folder is missing, or the daemon is not running. It must still exit 0 in well under 100 ms and drop the event. Test: Task 3.2 `TestHookGarbageStdinExitsZeroFast`.
3. The user's `~/.claude/settings.json` or `~/.codex/hooks.json` is missing, empty, has other hooks, or was edited by the user after install. Install must keep every other key and entry in order; uninstall must remove only our entries. Tests: Task 1.9 `TestKeyOrderAndUntouchedBytesKept`; Task 3.5 `TestInstallPreservesForeignHooks`, `TestUninstallAfterUserEditKeepsUserEntries`, `TestInstallIntoMissingFiles`.
4. A malicious extension `.zip` with `../` paths, absolute paths, or an id that claims `builtin.*`. It must be rejected with nothing written. Test: Task 1.6 cases `zip slip rejected`, `builtin id rejected`.
5. A limit whose `resets_at` is already in the past (Mac slept, no session since). The ring must say it reset and is waiting for new data, not show a negative countdown. Test: Task 2.1 `reset_in_past_says_waiting`.

---

## Phase 0: Prove the sandbox (gate: owner reports probe results, decision recorded)

### Task 0.1: Repository skeleton

**Files:**
- Create: `go.mod`, `cmd/agent-screensaver/main.go`, `web/package.json`, `web/tsconfig.json`, `web/test/smoke.test.ts`, `.gitignore`, `README.md`, `LICENSE`

**Interfaces:**
- Produces: Go module `agent-screensaver` (rename later with `go mod edit -module github.com/<owner>/agent-screensaver` once the GitHub repo exists) and binary `agent-screensaver`; `web/` npm package with scripts `build`, `test`, `typecheck`.

- [ ] **Step 1:** `git init -b main` in `~/workspace/agent-screensaver` (the folder already holds `docs/`).
- [ ] **Step 2:** `go mod init agent-screensaver` (go directive 1.23). `cmd/agent-screensaver/main.go` dispatches on `os.Args[1]` to subcommands `hook`, `statusline`, `daemon`, `install`, `uninstall`, `theme`, `option`, `privacy`, `sound`, `fps`, `ext`, `tray`, `settings`, each using its own `flag.FlagSet`; for now each prints "not implemented" and exits 1. Unknown or missing subcommand prints usage and exits 2.
- [ ] **Step 3:** Create `web/package.json` (`"type": "module"`, `"engines": {"node": ">=22.18"}`, devDependencies `esbuild` and `typescript`; scripts: `"test": "node --test test/"`, `"typecheck": "tsc --noEmit"`, `"build": "node build.mjs"`). `tsconfig.json`: `strict`, `target ES2022`, `module ESNext`, `moduleResolution bundler`, `allowImportingTsExtensions`, `noEmit`, `lib ["ES2022","DOM"]`.
- [ ] **Step 4:** Write `web/test/smoke.test.ts` with one `assert.equal(1 + 1, 2)` test.
- [ ] **Step 5:** Run `go vet ./... && go test ./...` (expect `no test files`, success) and `cd web && npm install && npm test` (expect 1 pass).
- [ ] **Step 6:** `.gitignore`: `build/`, `dist/`, `web/node_modules/`, `.superpowers/`. `LICENSE`: MIT, copyright "Stav Gayer". `README.md`: one paragraph from spec §1.
- [ ] **Step 7:** Commit: `git add go.mod cmd/agent-screensaver/main.go web/package.json web/package-lock.json web/tsconfig.json web/test/smoke.test.ts .gitignore README.md LICENSE docs/superpowers/specs/2026-10-20-agent-screensaver-design.md docs/superpowers/specs/mockups/layout-b-clock.html docs/superpowers/plans/2026-10-20-agent-screensaver-v1.md` then `git commit -m "chore: bootstrap repository"`.


### Task 0.2: Sandbox probe saver

Spec §5 "First task of the build". Nothing in Phase 1+ that depends on file locations starts before this gate.

**Files:**
- Create: `probe/ProbeView.swift`, `probe/Info.plist`, `probe/build.sh`, `probe/seed.sh`
- Create (after owner run): `docs/decisions/0001-data-dir.md`

**Interfaces:**
- Produces: the constant `DATA_DIR` (absolute path pattern relative to `$HOME`, or absolute) used by Task 1.8 `Paths`, and the answer "can the saver read Apple aerial files".

- [ ] **Step 1:** `probe/seed.sh` creates, in each candidate dir, `probe/state.json` (`{"ok":true}`) and `probe/index.html` (a page that shows "web load OK" and, via a `<script src="data.js">` sibling that sets `window.data`, shows "sibling script OK"). Candidates:
  1. `$HOME/Library/Application Support/agent-screensaver`
  2. `/Users/Shared/agent-screensaver` (mode 700)
  3. `$HOME/Library/Containers/com.apple.ScreenSaver.Engine.legacyScreenSaver/Data/Library/Application Support/agent-screensaver`
- [ ] **Step 2:** `ProbeView.swift`: a `ScreenSaverView` that, for each candidate, tries `Data(contentsOf: state.json)` and reports OK/denied, then shows one `WKWebView` per candidate loaded with `loadFileURL(index.html, allowingReadAccessTo: candidateDir)`. It also tries to read `/Library/Application Support/com.apple.idleassetsd/Customer/entries.json` and to open (not play) the first `.mov` found under `.../Customer/4KSDR240FPS/`, reporting OK/denied/none. It prints every result on screen in large text and with `NSLog("AGS-PROBE ...")`.
- [ ] **Step 3:** `build.sh` compiles with `swiftc -emit-library` for arm64 and x86_64, joins with `lipo`, assembles `build/AgentProbe.saver`, signs ad hoc.
- [ ] **Step 4:** Ask the owner to: run `probe/seed.sh`, download at least one aerial in System Settings → Screen Saver (choose any Cityscape), double-click `build/AgentProbe.saver`, Preview it, and paste the on-screen results (or `log show --last 5m --predicate 'eventMessage CONTAINS "AGS-PROBE"'`).
- [ ] **Step 5:** Write `docs/decisions/0001-data-dir.md`: the results table, the chosen `DATA_DIR` (first candidate where both the file read and the web load passed), and whether aerial files are readable. If no candidate passes, stop and bring it to the owner.
- [ ] **Step 6:** Commit `probe/*`, `docs/decisions/0001-data-dir.md`: `git commit -m "chore: add sandbox probe and record data dir decision"`.

---

---

## Phase 1: Go core with unit tests (gate: `go vet ./... && go test -race ./...` all green)

All packages in this phase are pure or touch only paths passed in. Each package `internal/<name>/<name>.go` has its tests in `internal/<name>/<name>_test.go` and fixtures in `internal/<name>/testdata/`. Steps follow the same cycle in every task: write the failing table-driven test, run `go test ./internal/<name>/` and see it FAIL, implement, run again and see it PASS, commit. The tasks below list only what is specific.

### Task 1.1: Event format

**Files:** Create `internal/event/event.go`, `internal/event/event_test.go`, `docs/event-format.md`

**Interfaces:**
- Produces:
  ```go
  const Version = 1
  type Window string   // FiveHour Window = "5h"; Week Window = "week"
  type LimitReading struct {
      Window      Window  `json:"window"`
      UsedPercent float64 `json:"used_percent"`
      ResetsAt    *int64  `json:"resets_at,omitempty"` // Unix seconds
  }
  type Kind string     // KindStarted "started", KindToolIntent "tool_intent", KindApproval "approval",
                       // KindResumed "resumed", KindFinished "finished", KindEnded "ended", KindLimits "limits"
  type Event struct {
      V       int            `json:"v"`
      Source  string         `json:"source"`
      Session string         `json:"session"`
      Kind    Kind           `json:"kind"`
      AtMs    int64          `json:"at_ms"`
      Project string         `json:"project,omitempty"`
      Title   string         `json:"title,omitempty"`
      Limits  []LimitReading `json:"limits,omitempty"`
  }
  func Parse(data []byte) (Event, error)
  ```
- [ ] **Step 1: Tests** (`TestParse`, table): `minimal started event` (`{"v":1,"source":"claude","session":"s1","kind":"started","at_ms":1}` → `KindStarted`, no limits); `unknown version` (`v:2` → error); `empty source`, `empty session` → error; `source over 128 chars` → error; `unknown kind` → error; `input over 64 KiB` → error. `TestLimitsRoundTrip`: a reading `{5h, 62.0, 1800000000}` survives `json.Marshal` → `Parse`.
- [ ] **Step 2:** Implement: check size first, then `json.Unmarshal`, then version, kind, and field limits.
- [ ] **Step 3:** Write `docs/event-format.md`: the JSON shape, every kind and its meaning (spec §4 table), and that any program can be a source by writing one such file into the spool folder (spec §7a). This is a public contract.
- [ ] **Step 4:** Commit `internal/event/event.go internal/event/event_test.go docs/event-format.md`: `feat: add versioned event format`.

### Task 1.2: Safe short titles

**Files:** Create `internal/text/text.go`, `internal/text/text_test.go`

**Interfaces:**
- Produces: `const TitleMax = 40`; `func MaskSecrets(s string) string`; `func ShortTitle(s string) string` (first non-empty line, whitespace collapsed, `MaskSecrets` first, then cut to `TitleMax` runes at the last word end with `…` appended when cut; counts runes, never splits one).
- [ ] **Step 1: Tests** (`TestMaskSecrets`, table, assert the secret is gone and `•••` is present): `bearer token` (`curl -H 'Authorization: Bearer abc123def456'`); `env assignment` (`API_KEY=sk-live-XYZ npm start`); `flag` (`--password hunter2`); `colon` (`token: ghp_abcdefghijklmnopqrstuvwxyz0123456789`); `long random string` (a 40-char base64-like run); `normal word kept` (`implementation` unchanged). `TestShortTitle`, table: `cuts at word end` (`"Refactor the authentication module so it uses the new session store"` → `"Refactor the authentication module so…"`); `first line only`; `hebrew does not split runes` (result is valid UTF-8); `short input unchanged`.
- [ ] **Step 2:** Implement with `regexp` (compiled once at package level). Patterns: `(?i)(bearer\s+)\S+`, `(?i)\b(token|key|secret|password|passwd|pwd|api[_-]?key)\b(\s*[:=]\s*|\s+)\S+`, `[A-Za-z0-9+/_\-]{32,}`.
- [ ] **Step 3:** Commit both files: `feat: add secret masking and short titles`.

### Task 1.3: Agent adapters: hook input mapping (Claude and Codex)

**Files:** Create `internal/agents/agents.go` (interface, registry, shared helpers), `internal/agents/claude.go`, `internal/agents/codex.go`, `internal/agents/agents_test.go`, `internal/agents/testdata/claude/*.json`, `internal/agents/testdata/codex/*.json`

**Interfaces:**
- Consumes: `event.Event`, `event.Kind*` (Task 1.1), `text.ShortTitle` (Task 1.2).
- Produces:
  - Extension point (spec §7b):
    ```go
    type Adapter interface {
        Name() string                                     // "claude", "codex"; also the event Source
        Map(input []byte, nowMs int64) (event.Event, bool) // false = ignore
    }
    func All() []Adapter                    // the one registry: Claude, Codex
    func ByName(name string) (Adapter, bool)
    ```
    `Claude` and `Codex` are the two implementations. Adding a tool later = one new file + one line in `All()`.
  - Shared helpers in `agents.go`, used by both adapters (DRY): `hookFields(input []byte) (fields, bool)` (event name, session from `session_id` → `thread_id` → `conversation_id`, project = `filepath.Base(cwd)`, prompt, tool name and input), and
  - `func DescribeTool(name string, input map[string]any) string` (returns `text.ShortTitle(name + ": " + detail)`; detail = `command` for shell tools, `file_path` or `path` for edit and write tools, else empty, so the result is just the tool name).

Mapping (spec §4 table). `Project` = `filepath.Base(cwd)`. `Session` from `session_id`, else `thread_id`, else `conversation_id`; none → ignore. Event name from `hook_event_name`.

| `hook_event_name` | Claude | Codex |
|---|---|---|
| `UserPromptSubmit` | `started`, title `ShortTitle(prompt)` | same |
| `PreToolUse` | `tool_intent`, title `DescribeTool` | `tool_intent` |
| `Notification` | `approval` only if `notification_type == "permission_prompt"`, or if `notification_type` is absent and `message` contains "permission" (case-insensitive); otherwise ignore | n/a |
| `PermissionRequest` | `approval`, title `DescribeTool` | `approval`, title `DescribeTool` |
| `PostToolUse` | `resumed` | `resumed` |
| `Stop` | `finished` | `finished` |
| `SessionEnd` | `ended` | `ended` |
| anything else | ignore | ignore |

- [ ] **Step 1:** Write fixtures from the official docs (Claude: https://code.claude.com/docs/en/hooks; Codex: https://learn.chatgpt.com/docs/hooks): one file per cell above, plus `claude/notification_idle.json` (`notification_type: "idle_prompt"`, message "Claude is waiting for your input"). Add a comment at the top of `claude.go` and `codex.go` saying which field names come from docs and are re-checked in Task 6.1.
- [ ] **Step 2: Tests** (`TestMap`, table with fields `name, adapter, fixture, wantOK, wantKind, wantSession, wantProject, wantTitle`): one row per table cell; plus `claude idle notification is ignored` (Review Focus 1); `missing session is ignored`; `unknown event is ignored`; `invalid json is ignored`. `TestRegistry`: `ByName("claude")` and `ByName("codex")` found, `ByName("x")` not. `TestDescribeTool`: `Bash` + `{"command":"npm run migrate"}` → `"Bash: npm run migrate"`; secrets masked; unknown tool → name only.
- [ ] **Step 3:** Implement: unmarshal into `map[string]any`, `switch` on the event name, read fields with checked type assertions (never panic).
- [ ] **Step 4:** Commit the four Go files and every fixture file by name: `feat: add agent adapters for Claude and Codex hooks`.

### Task 1.4: Status line limits

**Files:** Create `internal/statusline/statusline.go`, `internal/statusline/statusline_test.go`, `internal/statusline/testdata/{full,five_hour_only,no_rate_limits}.json`

**Interfaces:**
- Produces: `func LimitsEvent(input []byte, nowMs int64) (event.Event, bool)` (source `"claude"`, kind `limits`, session from `session_id`, readings from `rate_limits.five_hour` and `rate_limits.seven_day`, fields `used_percentage` and `resets_at`; each window optional; no readings → false).
- [ ] **Step 1:** Fixtures shaped per https://code.claude.com/docs/en/statusline.
- [ ] **Step 2: Tests** (`TestLimitsEvent`, table): `both windows` (62.0 and 38.0, resets as given); `one window`; `no rate limits` → false; `percent clamped to 0..100`; `invalid json` → false.
- [ ] **Step 3:** Implement. **Step 4:** Commit the five files: `feat: read Claude rate limits from status line input`.

### Task 1.5: State reducer

**Files:** Create `internal/state/state.go`, `internal/state/state_test.go`

**Interfaces:**
- Consumes: `event.Event`, `event.Kind*`, `event.LimitReading`, `event.Window`.
- Produces:
  ```go
  type CardKind string // KindApproval "approval", KindRunning "running", KindFinished "finished",
                       // KindUnknown "unknown", KindNoActivity "no_activity"
  type Card struct {
      Source          string   `json:"source"`
      Session         string   `json:"session"`
      Kind            CardKind `json:"kind"`
      Project         string   `json:"project,omitempty"`
      Title           string   `json:"title,omitempty"`
      Request         string   `json:"request,omitempty"`
      StartedAtMs     int64    `json:"started_at_ms"`
      UpdatedAtMs     int64    `json:"updated_at_ms"`
      ApprovalSinceMs *int64   `json:"approval_since_ms,omitempty"`
  }
  type Limit struct {
      Source      string       `json:"source"`
      Window      event.Window `json:"window"`
      UsedPercent float64      `json:"used_percent"`
      ResetsAt    *int64       `json:"resets_at,omitempty"`
      UpdatedAtMs int64        `json:"updated_at_ms"`
  }
  type State struct { Cards []Card; Limits []Limit }
  func (s *State) Apply(e event.Event)
  func (s *State) Tick(nowMs int64)
  func (s *State) NeedsAttention() bool
  const ( FinishedTTLMs = 30 * 60_000; NoActivityAfterMs = 60 * 60_000; RemoveAfterMs = 12 * 3_600_000; ApprovalUnknownAfterMs = 10 * 60_000 )
  ```

Rules (card key = `Source` + `Session`):
- `started`: create or reset the card → `running`, `Title` = event title, `StartedAtMs` = `AtMs`, `Request` cleared.
- `tool_intent`: `Request` = event title; kind unchanged (create a `running` card if none).
- `approval`: kind `approval`, `ApprovalSinceMs` = `AtMs`; if the event has a title, `Request` = it.
- `resumed`: kind `running`, `ApprovalSinceMs` = nil.
- `finished`: kind `finished`.
- `ended`: remove the card unless it is `finished`.
- `limits`: upsert each reading by `Source` + `Window`.
- Every event fills `Project` if it has one and sets `UpdatedAtMs = max(UpdatedAtMs, AtMs)`.
- `Tick`: `finished` older than TTL → removed; `approval` with no event for 10 min → `unknown` (both tools; my reading of spec §4); `running` with no event for 60 min → `no_activity`; any card with no event for 12 h → removed.
- [ ] **Step 1: Tests** (`TestApply` table of event sequences → expected cards; `TestTick` table of `(cards, now)` → expected cards): `started creates running card`; `two sessions same project two cards`; `approval then resumed returns to running`; `approval keeps prompt title and sets request`; `tool intent fills request for titleless approval`; `finished kept at 29 min removed at 31 min`; `approval becomes unknown after 10 min`; `running becomes no activity after 60 min`; `any card removed after 12 h`; `ended removes running keeps finished`; `limits upsert by source and window`; `needs attention only with approval`; `late event does not rewind updated at`.
- [ ] **Step 2:** Implement. **Step 3:** Commit both files: `feat: add card and limit state reducer`.

### Task 1.6: Extension manifests, install and remove

**Files:** Create `internal/extensions/extensions.go`, `internal/extensions/zip.go`, `internal/extensions/extensions_test.go`, `internal/extensions/testdata/...` (valid and invalid manifest folders, `slip.zip`, `absolute.zip` generated by a `//go:generate`-free helper inside the test using `archive/zip`), `docs/extensions.md`

**Interfaces:**
- Produces:
  ```go
  const APIVersion = 1
  type Type string        // TypeBackground "background", TypeComponent "component", TypeLayout "layout", TypeTheme "theme"
  type OptionKind string  // "string", "number", "boolean", "color", "enum", "file"
  type OptionSpec struct {
      Kind    OptionKind      `json:"type"`
      Default json.RawMessage `json:"default"`
      Min     *float64        `json:"min,omitempty"`
      Max     *float64        `json:"max,omitempty"`
      Values  []string        `json:"values,omitempty"`
      Label   string          `json:"label,omitempty"`
  }
  type ExtRef struct { ID string `json:"id"`; Options map[string]any `json:"options,omitempty"` }
  type ThemeSpec struct { Background ExtRef `json:"background"`; Layout string `json:"layout"`; Style map[string]any `json:"style"` }
  type Manifest struct {
      ID, Name, Version string
      Type    Type                  `json:"type"`
      API     int                   `json:"api"`
      Entry   string                `json:"entry,omitempty"`
      Options map[string]OptionSpec `json:"options,omitempty"`
      Layout  json.RawMessage       `json:"layout,omitempty"`
      Theme   *ThemeSpec            `json:"theme,omitempty"`
  }
  type Installed struct { Manifest Manifest; Dir string }
  func Validate(m Manifest, dir string) error
  func ValidateOption(spec OptionSpec, v any) error
  func Scan(extDir string) []Installed               // skips invalid ones, never fails
  func InstallFrom(src, extDir string) (Manifest, error) // folder or .zip
  func Remove(id, extDir string) error
  ```

Layout JSON shape (spec §7a, layouts are data only): `{"modes": {"idle": {"<region>": [ExtRef...]}, "attention": {...}, "secondary": {...}}}` where `<region>` ∈ `top-left`, `top-right`, `center`, `bottom`; `secondary` optional.

Validation: id matches `^[a-z0-9-]+(\.[a-z0-9-]+)+$`; `API == APIVersion`; `Entry` required for background and component, relative, inside `dir` after `filepath.Clean`, and the file exists; `Layout` required and well-formed for layouts (known regions only); `Theme` required for themes; option defaults pass `ValidateOption`.

Install: unpack or copy into a temp dir inside `extDir`, validate, then rename over `extDir/<id>`. Zip: before writing anything, reject any entry whose cleaned name is absolute, contains a `..` element, or is a symlink.

- [ ] **Step 1: Tests** (`TestValidate`, `TestValidateOption`, `TestInstallFrom`, `TestRemove`, all table-driven): `valid background`; `bad id`; `unsupported api`; `entry outside dir` (`../x.js`); `unknown region`; `number out of range`; `enum not in values`; `color must be #rrggbb`; `scan skips invalid`; `install from folder`; `install from zip`; `zip slip rejected` (entry `../../evil`, nothing written; Review Focus 4); `absolute zip path rejected`; `symlink in zip rejected`; `builtin id rejected` (`builtin.anything`); `same id replaced atomically`; `remove refuses builtin`.
- [ ] **Step 2:** Implement.
- [ ] **Step 3:** Write `docs/extensions.md`: the five extension types, manifest fields, option kinds, layout regions and modes, the web API (placeholder now, filled in Task 2.1), the safety rules (spec §7a). This is the public authoring guide.
- [ ] **Step 4:** Commit the Go files, every testdata file by name, and `docs/extensions.md`: `feat: add extension manifests and local install`.

### Task 1.7: Config and view resolution

**Files:** Create `internal/config/config.go`, `internal/config/config_test.go`, `internal/view/view.go`, `internal/view/view_test.go`, `internal/view/testdata/extensions/...`

**Interfaces:**
- Consumes: `extensions.Manifest`, `extensions.Installed`, `extensions.ExtRef`, `extensions.ValidateOption`.
- Produces:
  ```go
  // package config
  type Config struct {
      Theme              string                    `json:"theme"`      // default "builtin.default"
      Options            map[string]map[string]any `json:"options"`    // ext id → key → value
      Sound              bool                      `json:"sound"`      // default true
      SoundFile          string                    `json:"sound_file"` // default "/System/Library/Sounds/Glass.aiff"
      Privacy            bool                      `json:"privacy"`    // default false
      MaxFPS             int                       `json:"max_fps"`    // 0 = display refresh rate (default); else 1..240
      PreviousStatusline string                    `json:"previous_statusline,omitempty"`
  }
  func Default() Config
  func Load(path string) Config            // missing or broken file → Default()
  func (c Config) Save(path string) error  // atomic (temp file + rename)

  // package view
  type ResolvedExt struct { ID string `json:"id"`; EntryURL string `json:"entry_url"`; Options map[string]any `json:"options"` }
  type View struct {
      Theme      string          `json:"theme"`
      Background ResolvedExt     `json:"background"`
      Layout     json.RawMessage `json:"layout"`
      Components []ResolvedExt   `json:"components"`
      Style      map[string]any  `json:"style"`
      Privacy    bool            `json:"privacy"`
      MaxFPS     int             `json:"max_fps"` // from Config; 0 = no ceiling beyond the display
  }
  func Resolve(cfg config.Config, installed []extensions.Installed) View
  ```

Option merge order per extension: manifest defaults → theme or layout `ExtRef.Options` → `cfg.Options[id]`. A value that fails `ValidateOption` is dropped (the lower layer wins). Missing theme, layout, or extension → fall back to `builtin.default` and its parts. `Components` lists each component referenced by any layout mode once, with manifest defaults merged with `cfg.Options`; per-placement options stay inside `Layout`. `EntryURL` is a `file://` URL to the entry file.

- [ ] **Step 1: Tests** (table-driven): `default when file missing`; `broken file falls back to default`; `save then load round trip`; `user over theme over default`; `invalid user option dropped`; `missing theme falls back to builtin default`; `components listed once`; `privacy passes through`.
- [ ] **Step 2:** Implement. **Step 3:** Commit the four Go files and each testdata file by name: `feat: resolve active theme, layout and options`.

### Task 1.8: Paths, spool and state file

**Files:** Create `internal/paths/paths.go`, `internal/spool/spool.go`, `internal/spool/spool_test.go`, `internal/statefile/statefile.go`, `internal/statefile/statefile_test.go`

**Interfaces:**
- Consumes: `DATA_DIR` decision (Task 0.2), `event.Event`, `state.State`, `view.View`.
- Produces:
  ```go
  // package paths
  type Paths struct { DataDir, SpoolDir, StateFile, ConfigFile, ExtensionsDir, RuntimeDir, MediaDir, InstallRecord string }
  func ForHome(home string) Paths // spool/, state.json, config.json, extensions/, runtime/, media/, install-record.json under DataDir

  // package spool
  func Write(dir string, e event.Event) error // "<at_ms>-<pid>-<nanos>.json.tmp" then rename to ".json"; creates dir
  func Drain(dir string) []event.Event        // *.json sorted by name; deletes each after reading; deletes unparsable ones; ignores .tmp

  // package statefile
  type StateFile struct {
      Version       int           `json:"version"` // 1
      GeneratedAtMs int64         `json:"generated_at_ms"`
      ServiceOK     bool          `json:"service_ok"`
      Cards         []state.Card  `json:"cards"`
      Limits        []state.Limit `json:"limits"`
      View          view.View     `json:"view"`
  }
  func Build(st state.State, v view.View, nowMs int64) StateFile // when v.Privacy: Title and Request emptied
  func WriteAtomic(path string, f StateFile) error
  ```
- [ ] **Step 1: Tests:** `TestWriteThenDrainOnce`; `TestDrainOrdersByTime`; `TestDrainIgnoresTmpAndDeletesGarbage`; `TestDrainMissingDirIsEmpty`; `TestBuildPrivacyStripsTitles`; `TestWriteAtomicLeavesNoTmp` (file parses back).
- [ ] **Step 2:** Implement. **Step 3:** Commit the five files: `feat: add spool and atomic state file`.

### Task 1.9: Order-preserving JSON editing

Go's `map[string]any` sorts keys when it writes JSON, which would rewrite the user's settings in a new order. This package edits only what we touch.

**Files:** Create `internal/jsonedit/jsonedit.go`, `internal/jsonedit/jsonedit_test.go`, `internal/jsonedit/testdata/settings.json`

**Interfaces:**
- Produces:
  ```go
  type Object struct { /* keys in original order + raw values */ }
  func Parse(data []byte) (*Object, error)       // empty or whitespace input → empty object; non-object → error
  func (o *Object) Get(key string) (json.RawMessage, bool)
  func (o *Object) Set(key string, raw json.RawMessage) // keeps position if present, appends if new
  func (o *Object) Delete(key string)
  func (o *Object) Keys() []string
  func (o *Object) Marshal() ([]byte, error)     // 2-space indent, trailing newline
  ```
  Implement with `json.Decoder.Token()` for the top level and `json.RawMessage` for values, so untouched values keep their bytes (re-indented only).
- [ ] **Step 1: Tests:** `TestKeyOrderAndUntouchedBytesKept` (parse `testdata/settings.json` with keys `z, a, hooks, m`; set `hooks`; marshal; keys still `z, a, hooks, m`; values of `z`, `a`, `m` equal after `json.Compact`; Review Focus 3); `TestEmptyInputIsEmptyObject`; `TestNonObjectIsError` (`[]`, `"x"`); `TestInvalidJSONIsError` (install must abort, never overwrite); `TestSetNewKeyAppends`; `TestDelete`.
- [ ] **Step 2:** Implement. **Step 3:** Commit the three files: `feat: add order-preserving JSON editing`.

**Phase 1 gate:** `go vet ./... && go test -race ./...` all green.


---

---

## Phase 2: Web runtime and built-in extensions (gate: `npm test`, `npm run typecheck`, visual check in browser)

### Task 2.1: Extension API and formatting

**Files:** Create `web/src/api.ts`, `web/src/format.ts`, `web/test/format.test.ts`; Modify `docs/extensions.md`

**Interfaces:**
- Produces (`api.ts`, exported and later emitted as `runtime/api.d.ts`):
  ```ts
  export const API_VERSION = 1;
  export type Region = "top-left" | "top-right" | "center" | "bottom";
  export type Mode = "idle" | "attention" | "secondary";
  export type CardKind = "approval" | "running" | "finished" | "unknown" | "no_activity";
  export interface Card { source: string; session: string; kind: CardKind; project?: string; title?: string; request?: string; started_at_ms: number; updated_at_ms: number; approval_since_ms?: number }
  export interface Limit { source: string; window: "5h" | "week"; used_percent: number; resets_at?: number; updated_at_ms: number }
  export interface ViewState { mode: Mode; cards: Card[]; limits: Limit[]; nowMs: number; serviceOk: boolean; display: "main" | "secondary" }
  export interface Ctx { options: Readonly<Record<string, unknown>>; requestFrame(cb: (nowMs: number) => void): () => void; now(): number; display: "main" | "secondary" }
  export interface BackgroundExt { start(root: HTMLElement, ctx: Ctx): void; stop(): void }
  export interface ComponentExt { mount(root: HTMLElement, ctx: Ctx): void; update(state: ViewState): void; unmount(): void }
  // An extension's entry module: export default function create(): BackgroundExt | ComponentExt
  ```
- Produces (`format.ts`): `formatDuration(ms: number): string` (`"3m"`, `"24m"`, `"1h 12m"`, under 1 min → `"<1m"`); `formatResetIn(resetsAtS: number | undefined, nowMs: number): string` (`"1h40m"`, `"35m"`; past or now → `"reset, waiting for new data"`; undefined → `""`); `ringLevel(pct: number): "ok" | "warn" | "high"` (`< 50` ok, `50..=80` warn, `> 80` high); `formatClock(d: Date): string` (24 h `HH:MM`); `formatDate(d: Date): string` (`en-GB`, weekday day month, e.g. `"Tuesday 20 October"`).

- [ ] **Step 1: Write failing tests** in `format.test.ts`: durations above; `formatResetIn` for +100 min → `"1h40m"`; `reset_in_past_says_waiting` (Review Focus 5); `ringLevel(49.9)` ok, `ringLevel(50)` warn, `ringLevel(80)` warn, `ringLevel(80.1)` high; `formatClock(new Date(2026,9,20,9,5))` → `"09:05"`; `formatDate` → `"Tuesday 20 October"`.
- [ ] **Step 2:** `npm test` → FAIL. **Step 3:** implement. **Step 4:** `npm test` and `npm run typecheck` → PASS.
- [ ] **Step 5:** Paste the `api.ts` types into `docs/extensions.md` (replacing the placeholder from Task 1.6).
- [ ] **Step 6:** Commit `web/src/api.ts web/src/format.ts web/test/format.test.ts docs/extensions.md`: `feat(web): add extension API types and formatting`.

### Task 2.2: Frame cap, mode selection, layout regions

**Files:** Create `web/src/frame.ts`, `web/src/mode.ts`, `web/src/layout.ts` and tests `web/test/{frame,mode,layout}.test.ts`

**Interfaces:**
- Consumes: types from `api.ts`.
- Produces:
  - `frame.ts`: `shouldRender(lastMs: number, nowMs: number, fps: number): boolean` (true when `nowMs - lastMs >= 1000 / fps`); `createFrameScheduler(fps: number, raf = requestAnimationFrame): { request(cb): () => void }` (one shared rAF loop; each request runs at `min(requestedFps, maxFps)`; `maxFps` comes from `view.max_fps`, and `0` or missing means no ceiling beyond the display's own refresh).
  - `mode.ts`: `screenMode(cards: Card[], display: "main" | "secondary"): Mode` (`secondary` display → `"secondary"`; any `approval` card → `"attention"`; else `"idle"`); `approvalQueue(cards: Card[]): { first?: Card; more: number }` (oldest `approval_since_ms` first).
  - `layout.ts`: `type ExtRef = { id: string; options?: Record<string, unknown> }`; `regionsFor(layout: unknown, mode: Mode): Partial<Record<Region, ExtRef[]>>` (unknown regions dropped; missing `secondary` mode → `{ center: [{ id: "builtin.clock", options: { size: "large" } }] }`; broken layout → same fallback for every mode).
- [ ] **Step 1: Write failing tests:** `runs_at_requested_fps_under_ceiling` (asks 120, ceiling 120 → 120); `ceiling_wins_when_lower` (asks 120, ceiling 30 → 30); `no_ceiling_means_requested`; `renders_after_interval`; `secondary_display_mode`; `attention_when_any_approval`; `approval_queue_oldest_first_and_counts_more` (3 approvals → `more === 2`); `regions_for_idle`; `drops_unknown_region`; `broken_layout_falls_back_to_clock`.
- [ ] **Step 2:** `npm test` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit the six files: `feat(web): add frame cap, screen modes and layout regions`.

### Task 2.3: Runtime: loader, isolation, page

**Files:** Create `web/src/loader.ts`, `web/src/runtime.ts`, `web/index.html`, `web/styles.css`, `web/build.mjs`, `web/test/loader.test.ts`

**Interfaces:**
- Consumes: Tasks 2.1, 2.2; the `StateFile` JSON shape from Task 1.8 (`view.background`, `view.layout`, `view.components`, `view.style`).
- Produces:
  - `loader.ts`: `safeCall<T>(label: string, fn: () => T, onFail: (label: string, err: unknown) => void): T | undefined`; `loadExtension(entryUrl: string, timeoutMs = 2000): Promise<(() => BackgroundExt | ComponentExt) | undefined>` (dynamic `import()`; rejects slow or broken modules by returning `undefined`).
  - `runtime.ts`: global `window.agentScreensaver = { setState(json: string, display: "main" | "secondary"): void }`. On each call: parse; on first call or theme change, mount the background and the components per region; afterwards call `update(viewState)` on each mounted component. A component that throws in `mount` or `update` is unmounted and its region shows nothing (spec §7a). The `body` gets class `mode-<mode>`; the change between `idle` and `attention` is a CSS transition (clock moves to the corner, spec §3). `view.style` keys starting with `--` are set as CSS variables on `:root`. If `service_ok` is false or the state is older than 2 minutes, show a small line "service not running".
  - `index.html`: CSP meta `default-src 'none'; script-src file: 'self'; style-src file: 'self' 'unsafe-inline'; img-src file: data:; media-src file:; font-src file:; connect-src 'none'`; four region containers with ids `region-top-left`, `region-top-right`, `region-center`, `region-bottom` plus `#background`.
  - `build.mjs`: esbuild bundles `src/runtime.ts` → `../build/runtime/runtime.js`, copies `index.html`, `styles.css`, emits `api.d.ts` (`tsc --emitDeclarationOnly` for `src/api.ts`), and bundles each built-in `src/builtins/<name>.ts` → `../build/extensions/builtin.<name>/index.js` next to its `extension.json`.
- [ ] **Step 1: Write failing tests** (`loader.test.ts`, no DOM needed): `safe_call_returns_value`; `safe_call_reports_and_swallows_errors`; `load_extension_returns_undefined_for_missing_module`; `load_extension_times_out` (a module URL created with a `data:` URL that never resolves its top-level await).
- [ ] **Step 2:** `npm test` → FAIL. **Step 3:** implement. **Step 4:** → PASS; `npm run typecheck` → PASS.
- [ ] **Step 5:** Commit the six files: `feat(web): add runtime with isolated extension loading`.

### Task 2.4: Built-in extensions and default theme

**Files:** Create `web/src/builtins/{matrix,clock,limit-rings,task-row,approval-card}.ts`; Create `extensions/builtin.matrix/extension.json`, `extensions/builtin.clock/extension.json`, `extensions/builtin.limit-rings/extension.json`, `extensions/builtin.task-row/extension.json`, `extensions/builtin.approval-card/extension.json`, `extensions/builtin.spotlight/extension.json`, `extensions/builtin.default/extension.json`; Create `web/dev/preview.html`, `web/dev/state-idle.json`, `web/dev/state-attention.json`; Create `web/test/builtins.test.ts`

**Interfaces:**
- Consumes: `api.ts`, `format.ts`, `mode.ts` only. Built-ins MUST NOT import `runtime.ts` or anything outside these (Global Constraints, no private shortcuts).
- Produces (manifests; values from spec §3, §7, mockup `docs/superpowers/specs/mockups/layout-b-clock.html`):
  - `builtin.matrix` (background): options `color` (color, `#00ff66`), `speed` (number 0.25..4, 1), `charset` (enum `katakana|latin|digits|hebrew`, `katakana`), `fps` (number 1..240, 24), `density` (number 0.25..2, 1). Uses `ctx.requestFrame`.
  - `builtin.clock` (component): option `size` (enum `large|small`, `large`); shows `formatClock` and, when large, `formatDate`; updates every second via `ctx.requestFrame` at 1 fps.
  - `builtin.limit-rings` (component): one ring per `Limit` in order Claude 5h, Claude week, Codex 5h, Codex week; color class from `ringLevel`; 5h rings show `formatResetIn`; a ring whose `updated_at_ms` is over 15 min old shows `"updated Xm ago"`.
  - `builtin.task-row` (component): one card per `running`, `finished`, `unknown`, `no_activity` card: tool, project, title, and `"running 24m"` / `"finished 8m ago"` / `"check the terminal"` / `"no activity"`.
  - `builtin.approval-card` (component): `approvalQueue(cards).first`: tool, `"waiting 3m"`, `"Needs approval"`, project, `request`; `"+N more"` when `more > 0`; pulsing orange via CSS class.
  - `builtin.spotlight` (layout): `idle`: `center` clock large, `top-right` limit-rings, `bottom` task-row. `attention`: `top-left` clock small, `top-right` limit-rings, `center` approval-card, `bottom` task-row. `secondary`: `center` clock large.
  - `builtin.default` (theme): background `builtin.matrix` with defaults; layout `builtin.spotlight`; style `--accent: #00e07a`, `--attention: #ffb020`, `--done: #78c8ff`, `--card-opacity: 0.82`.
- [ ] **Step 1: Write failing tests** (`builtins.test.ts`, pure parts only): `task_row_label_running` (`"running 24m"`), `task_row_label_finished`, `approval_label_more`, `rings_order_claude_then_codex`. Export the label helpers from each builtin module to test them.
- [ ] **Step 2:** `npm test` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Add a Go test in `internal/extensions/extensions_test.go`: `TestAllBuiltinManifestsAreValid` (runs `Validate` on every folder in `build/extensions/` after `npm run build`; `t.Skip` with a clear message if `build/` is missing). Run `go test ./internal/extensions/` → PASS.
- [ ] **Step 6: Visual check:** `npm run build`, open `web/dev/preview.html` (it loads `build/runtime` and calls `setState` with `state-idle.json`, then with `state-attention.json` after 5 s) in the browser. Compare against the mockup: big clock idle; clock moves top-left and orange card pulses in attention; rings colored; Matrix at its 24 fps default, and at 120 fps after `option set builtin.matrix fps 120` on a 120 Hz display (check with the browser performance panel). Take one screenshot of each mode.
- [ ] **Step 7:** Commit every file listed above explicitly: `feat(web): add built-in Matrix, clock, rings, cards and spotlight theme`.

**Phase 2 gate:** `npm test`, `npm run typecheck`, `go test ./...` green; both screenshots match the mockup.

---

## Phase 3: Integration and wiring (gate: integration tests green, harness run shows real state)

### Task 3.1: The `.saver`

**Files:** Create `saver/AgentSaverView.swift`, `saver/Info.plist`, `saver/build.sh`; Copy `tools/harness/harness.swift` from the spike (scratchpad `spike-saver/harness.swift`) and adapt

**Interfaces:**
- Consumes: `DATA_DIR` (Task 0.2), `runtime_dir/index.html`, `state_file` (Task 1.8), `window.agentScreensaver.setState(json, display)` (Task 2.3).
- Produces: `build/AgentScreensaver.saver` (bundle id `dev.agentscreensaver.saver`, principal class `AgentScreensaver.AgentSaverView`).

Behavior:
- One `WKWebView` filling the view, `loadFileURL(runtime/index.html, allowingReadAccessTo: DATA_DIR)`.
- A 1-second `Timer` reads `state.json` modification date; on change, reads it and calls `setState`. `display` is `"main"` when `window?.screen == NSScreen.screens.first`, else `"secondary"`.
- On `com.apple.screensaver.willstop` (DistributedNotificationCenter): stop the timer, `webView.stopLoading()`, remove the web view, then `exit(0)` (spec §5).
- `isPreview == true`: same page, no change.

- [ ] **Step 1:** Write `saver/build.sh` (like `probe/build.sh`; copies nothing into the bundle except the binary and `Info.plist`; the page lives in `DATA_DIR`).
- [ ] **Step 2:** Implement `AgentSaverView.swift` (keep it under ~120 lines).
- [ ] **Step 3:** Adapt the harness: `harness <saver> <seconds> <screenshot.png>` loads the bundle, runs it in a window, takes the screenshot at the midpoint.
- [ ] **Step 4:** Copy `build/runtime` and `build/extensions` into a temp `DATA_DIR`, write `web/dev/state-attention.json` (with a `view` block resolved for that temp dir) as `state.json`, run the harness for 10 s. Expected: screenshot shows the attention layout over the Matrix. Look at the screenshot.
- [ ] **Step 5:** Commit `saver/AgentSaverView.swift saver/Info.plist saver/build.sh tools/harness/harness.swift`: `feat(saver): add screen saver host`.

### Task 3.2: `hook` and `statusline` commands

**Files:** Create `internal/cli/hook.go`, `internal/cli/statusline.go`, `cmd/agent-screensaver/main_test.go`; Modify `cmd/agent-screensaver/main.go`

**Interfaces:**
- Consumes: `agents.ByName`, `statusline.LimitsEvent`, `spool.Write`, `paths.ForHome`, `config.Load`.
- Produces:
  - `func RunHook(adapter agents.Adapter, stdin io.Reader, p paths.Paths, now func() int64) int` → always 0. Reads at most 1 MiB of stdin (the rest is discarded), maps, writes the event. Prints nothing on stdout (the tools may treat stdout as hook output). Errors go to `DataDir/hook-errors.log` (truncated at 1 MiB). When the env var `AGS_RECORD_PAYLOADS=1` is set, it also saves the raw stdin to `DataDir/payloads/<tool>-<event>-<at_ms>.json` (used only in Task 6.1).
  - `func RunStatusline(stdin io.Reader, stdout io.Writer, p paths.Paths, now func() int64) int` → always 0. Writes the limits event if any, then runs `cfg.PreviousStatusline` (if set and not our own binary) with the same stdin through `sh -c`, with a 300 ms timeout (`exec.CommandContext`), and copies its stdout. On timeout or failure it prints nothing.
  - `func RunPrevious(cmd string, stdin []byte, timeout time.Duration) (string, bool)`
  - CLI: `agent-screensaver hook --tool <name>` (looked up with `agents.ByName`; unknown name → exit 0, error logged), `agent-screensaver statusline`.
- [ ] **Step 1: Tests:** `main_test.go` has a `TestMain` that builds the binary once into a temp dir; tests run it with `HOME` set to `t.TempDir()` and skip under `-short`. `TestHookWritesEventFile`; `TestHookGarbageStdinExitsZeroFast` (table: garbage, empty, 5 MiB: exit 0, under 100 ms; Review Focus 2); `TestHookCreatesDataDir`; `TestStatuslinePassesThroughPrevious` (previous = `echo hi` → stdout `hi`, limits event written); `TestStatuslineTimesOutSlowPrevious` (previous = `sleep 2` → returns in under 500 ms, exit 0); `TestStatuslineDoesNotCallItself`.
- [ ] **Step 2:** `go test ./cmd/...` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit the four files: `feat(cli): add hook and statusline commands`.

### Task 3.3: Limits providers (Codex)

**Files:** Create `internal/limits/limits.go` (interface, registry, `PollDue`), `internal/limits/codex.go`, `internal/limits/limits_test.go`, `internal/limits/testdata/codex_rate_limits_response.json`

**Interfaces:**
- Produces (extension point, spec §7b):
  ```go
  type Provider interface {
      Source() string                                         // "codex"; matches the agents.Adapter name
      Read(ctx context.Context) ([]event.LimitReading, error)
  }
  func All(lookPath func(string) (string, error)) []Provider // registry; Codex only when `codex` is on PATH
  func PollDue(lastPollMs *int64, saverActive, sourceFinishedSincePoll bool, nowMs int64) bool
  // true when the saver is active and: never polled, or 5 min passed, or that source finished a task since the last poll
  ```
  `type Codex struct { Bin string }` implements `Provider`; `func ParseCodexRateLimits(resp []byte) []event.LimitReading` (primary window → `5h`, secondary → `week`).
- [ ] **Step 1:** Run `codex app-server generate-json-schema --out <tmp>`; read the `initialize` and `account/rateLimits/read` shapes; write the fixture from the schema; record the exact handshake (requests and notifications in order) in a comment at the top of `codex.go`.
- [ ] **Step 2: Tests** (table): `primary is 5h and secondary is week`; `missing windows give empty`; `PollDue`: `never polled and active`, `inactive never due`, `due after 5 min`, `not due at 4 min`, `due after source finished`; `All` with a fake `lookPath`: codex present → one provider, absent → none.
- [ ] **Step 3:** Implement. `Codex.Read` starts `codex app-server` with `exec.CommandContext`, writes newline-delimited JSON-RPC, reads lines until the response id matches or the context ends, then kills the child.
- [ ] **Step 4:** `TestCodexReadLive` (skips under `-short` and when `codex` is not on `PATH`): calls `Read` with a 10 s timeout and logs the result. Run once with `go test -run TestCodexReadLive -v ./internal/limits/`; expect two readings.
- [ ] **Step 5:** Commit the four files: `feat: add limits providers with Codex app-server`.

### Task 3.4: Platform pieces and the daemon

**Files:** Create `internal/platform/platform.go` (interfaces only), `internal/platform/saverwatch_darwin.go`, `internal/platform/saverwatch_darwin.m`, `internal/platform/sound_darwin.go`, `internal/daemon/daemon.go`, `internal/daemon/daemon_test.go`; Modify `cmd/agent-screensaver/main.go`

**Interfaces:**
- Consumes: everything in Phase 1, `limits.Provider`, `limits.PollDue`.
- Produces (extension points, spec §7b; macOS implementations use the `_darwin` file suffix so Linux and Windows files can sit next to them later):
  ```go
  // package platform
  type SaverWatcher interface { Active() bool }
  type SoundPlayer  interface { Play(file string) error }   // must not block
  type ServiceManager interface {
      Install(label string, args []string) (path string, err error) // start a process now and at every login (daemon, later the tray)
      Uninstall(label string) error
  }
  func NewSaverWatcher() SaverWatcher   // macOS: distributed notifications
  func NewSoundPlayer() SoundPlayer     // macOS: /usr/bin/afplay, started without waiting
  ```
  macOS `SaverWatcher`: cgo with one Objective-C file subscribing to `com.apple.screensaver.didstart` and `com.apple.screensaver.didstop` on `NSDistributedNotificationCenter`; `CFRunLoopRun()` on a goroutine locked to its OS thread (`runtime.LockOSThread`); an exported Go callback stores the value in an `atomic.Bool`. (`ServiceManager` is implemented in Task 3.5.)
  ```go
  // package daemon
  func ShouldPlaySound(prevAttention, nowAttention, saverActive, soundEnabled bool) bool // only false → true while the saver shows
  type StepResult struct { StateWritten, PlaySound bool; PollSources []string }
  func New(p paths.Paths) *Daemon
  func (d *Daemon) Step(nowMs int64, saverActive bool) StepResult
  func (d *Daemon) SetLimits(source string, readings []event.LimitReading, nowMs int64)
  func Run(ctx context.Context, d *Daemon, w platform.SaverWatcher, s platform.SoundPlayer, providers []limits.Provider) error
  ```
  `Step` has no goroutines and no sleeping, only file IO under `Paths`, so it is testable: it drains the spool, applies events, ticks, reloads config and rescans extensions when their mtimes change, writes `state.json` when the state changed or 30 s passed, and lists the sources whose limits are due (`PollDue` per provider; "finished since poll" comes from `finished` events with that source). `Run` owns the loop: a `time.Ticker` every 500 ms calls `Step` (ponytail: polling the spool twice a second costs nothing measurable; switch to FSEvents only if profiling says so), plays the sound through `SoundPlayer`, and starts at most one `Read` goroutine per due provider with a 10 s timeout, feeding results back through a channel into `SetLimits`.
- [ ] **Step 1: Tests** (`t.TempDir()` home, fake clock values, fake `SaverWatcher` and `SoundPlayer` and `Provider`): `TestShouldPlaySound` (table of all 16 combinations, true only for `false,true,true,true`); `TestStepDrainsSpoolIntoStateFile`; `TestStateRewrittenEvery30s`; `TestConfigChangeIsPickedUp`; `TestFinishedCardGoneAfterTTL`; `TestSetLimitsAppliedOnNextStep`; `TestCodexDueAfterCodexFinished`; `TestRunPlaysSoundOnceOnNewApproval` (runs `Run` with a cancelable context and fakes, then cancels).
- [ ] **Step 2:** `go test -race ./internal/daemon/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Manual check: run `go run ./cmd/agent-screensaver daemon` in a terminal, start the system screen saver with a hot corner, and confirm the daemon logs `saver active: true` then `false`.
- [ ] **Step 6:** Commit the seven files: `feat: add platform interfaces and daemon loop`.

### Task 3.5: Install and uninstall

**Files:** Create `internal/install/install.go`, `internal/install/install_test.go`, `internal/install/testdata/*.json`, `internal/agents/install_claude.go`, `internal/agents/install_codex.go`, `internal/platform/launchd_darwin.go`; Modify `internal/agents/agents.go`, `cmd/agent-screensaver/main.go`

**Interfaces:**
- Consumes: `paths.Paths`, `config.Config`, `jsonedit.Object` (Task 1.9), `agents.All` (Task 1.3), `platform.ServiceManager` (Task 3.4).
- Produces:
  - A second small interface in `agents.go`, implemented by both adapters (so install loops over `agents.All()` instead of naming tools):
    ```go
    type HookInstaller interface {
        ConfigPath(home string) string                           // ~/.claude/settings.json, ~/.codex/hooks.json
        AddHooks(cfg *jsonedit.Object, command string) error     // append our entries; idempotent
        RemoveHooks(cfg *jsonedit.Object, command string) error  // remove only entries whose command is ours
    }
    ```
    Claude events: `UserPromptSubmit`, `PreToolUse`, `Notification`, `PermissionRequest`, `PostToolUse`, `Stop`, `SessionEnd`, each as `{"matcher": "", "hooks": [{"type": "command", "command": "<bin> hook --tool claude"}]}` appended to that event's array (format: https://code.claude.com/docs/en/hooks; if `PermissionRequest` is not a documented event, skip it). Codex events: `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Stop`, `SessionEnd`; copy the entry structure exactly from https://learn.chatgpt.com/docs/hooks and from the existing entries in the owner's `~/.codex/hooks.json` (read only). The array-append and remove-by-command logic is one shared helper used by both (DRY).
  - Claude only, in `install_claude.go`: `func SwapStatusline(settings *jsonedit.Object, ours string) (previous string, err error)`; `func RestoreStatusline(settings *jsonedit.Object, previous, ours string) error` (only if the current command is still ours).
  - `launchd_darwin.go`: `type Launchd struct { Home string; Run func(name string, args ...string) error }` implements `platform.ServiceManager` (writes `~/Library/LaunchAgents/<label>.plist`, for the daemon `dev.agentscreensaver.daemon` with `ProgramArguments [bin, "daemon"]`, `RunAtLoad`, `KeepAlive`, `ProcessType Background`, `LowPriorityIO`; runs `launchctl bootstrap gui/<uid>` / `bootout`). Tests pass a fake `Run`.
  - `install.go`: `type PlannedStep struct { Name, Description string }`; `func Plan(home string) []PlannedStep`; `func Apply(home, bin, artifacts string, sm platform.ServiceManager) (Record, error)`; `func Uninstall(home string, sm platform.ServiceManager) error`; `type Record struct { Bin string; Agents []string; PreviousStatusline string; Services []string /* labels */; Saver string }` saved as JSON to `InstallRecord`.
  - CLI: `install` prints the plan and asks `Proceed? [y/N]` (`--yes` skips the prompt). Before editing a JSON file it writes `<file>.agent-screensaver.bak`. An unparsable user file aborts the install with a clear message and changes nothing. After install it reminds the user to approve the Codex hooks with `/hooks` and to set "Require password after screen saver begins" to Immediately. `uninstall` reads the record, reverses each step, calls `ServiceManager.Uninstall` for every label in `Services`, deletes the record.
- [ ] **Step 1: Tests** (table-driven where it fits, temp `HOME`, fake `ServiceManager`): `TestInstallIntoMissingFiles` (both files created with only our entries); `TestInstallPreservesForeignHooks` (fixture with a Superset-like hook and unrelated keys: all kept, key order kept); `TestInstallIsIdempotent` (twice → our entries once); `TestStatuslineSwappedAndSaved`; `TestUninstallRestoresEverything` (files equal to the originals after `json.Compact`); `TestUninstallAfterUserEditKeepsUserEntries` (Review Focus 3); `TestUninstallKeepsUserStatuslineIfChanged`; `TestInstallAbortsOnInvalidUserJSON` (nothing written); `TestLaunchdPlistAndCommands` (fake `Run` records `bootstrap` then `bootout`).
- [ ] **Step 2:** `go test ./internal/install/ ./internal/agents/ ./internal/platform/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit the Go files and each testdata file by name: `feat(cli): add install and uninstall`.

### Task 3.6: `theme`, `option`, `privacy`, `sound`, `fps` and `ext` commands

**Files:** Create `internal/settings/settings.go`, `internal/settings/settings_test.go`, `internal/cli/settings.go`; Modify `cmd/agent-screensaver/main.go`

**Interfaces:**
- Consumes: `extensions.*`, `config.Config`, `extensions.ValidateOption`.
- Produces:
  - `internal/settings`, the ONE place for settings logic, used by the CLI now and by the tray and settings window in Phase 5 (DRY):
    ```go
    type Service struct { Paths paths.Paths }
    func (s Service) Snapshot() (Snapshot, error) // config + installed extensions with their option schemas and current values
    func (s Service) SetTheme(id string) error
    func (s Service) SetOption(extID, key string, raw string) error // parsed per OptionKind; "file" copies into MediaDir
    func (s Service) ResetOption(extID, key string) error         // key "" resets all options of extID
    func (s Service) SetPrivacy(on bool) error
    func (s Service) SetSound(on bool, file string) error         // file "" keeps the current one
    func (s Service) SetMaxFPS(fps int) error                     // 0 = display, else 1..240
    func (s Service) InstallExtension(path string) (extensions.Manifest, error)
    func (s Service) RemoveExtension(id string) error
    type Snapshot struct { Config config.Config `json:"config"`; Extensions []ExtensionInfo `json:"extensions"` }
    type ExtensionInfo struct { Manifest extensions.Manifest `json:"manifest"`; Values map[string]any `json:"values"`; Builtin bool `json:"builtin"` }
    ```
  - CLI (thin wrappers in `internal/cli/settings.go` over `settings.Service`): `theme list`, `theme set <id>` (must be an installed theme), `option set <ext-id> <key> <value>` (value parsed per `OptionKind` and validated; `file` kind copies the file into `MediaDir` and stores the copied path, because the saver can only read `DataDir`), `option reset <ext-id> [key]`, `privacy on|off`, `sound on|off`, `fps max <1..240|display>`, `ext install <path>`, `ext list`, `ext remove <id>`. One-line output on success; errors exit 1 with a plain message.
- [ ] **Step 1: Tests** (on `settings.Service`, temp `HOME`): `TestSetThemeUnknownFails`; `TestSetOptionValidatesRange`; `TestFileOptionCopiesIntoMedia`; `TestInstallListRemoveExtension`; `TestPrivacyTogglePersists`; `TestSetMaxFPSRange` (0, 1, 240 ok; -1, 241 fail); `TestSnapshotShowsSchemasAndValues`.
- [ ] **Step 2:** `go test ./internal/settings/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit the three files: `feat: add settings service and CLI commands`.

### Task 3.7: Release build script

**Files:** Create `scripts/build-release.sh`; Modify `README.md`

**Interfaces:**
- Produces: `dist/agent-screensaver-<version>-macos.zip` containing `agent-screensaver` (universal: `CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w"` and the same with `GOARCH=amd64 CC="clang -arch x86_64"`, joined with `lipo`), `AgentScreensaver.saver`, `runtime/`, `extensions/`. `install` (Task 3.5) takes `artifacts` = the folder next to the binary.
- [ ] **Step 1:** Write the script; run it; check `lipo -archs` prints `x86_64 arm64` for the Go binary and the saver binary.
- [ ] **Step 2:** README: install steps, the "Open Anyway" step (spec §5 Signing), approving Codex hooks with `/hooks`, setting the lock to Immediately, `uninstall`, and a link to `docs/extensions.md`.
- [ ] **Step 3:** Commit `scripts/build-release.sh README.md`: `build: add release script and install docs`.

**Phase 3 gate:** `go vet ./... && go test -race ./...` (including integration tests, no `-short`) and `npm test` green; Task 3.1 harness screenshot shows real state from the daemon (run the daemon, feed `hook` with fixture input, run the harness).

---

## Phase 4: More backgrounds (gate: tests green, video CPU measured)

### Task 4.1: Image and video backgrounds

**Files:** Create `web/src/builtins/image.ts`, `web/src/builtins/video.ts`, `extensions/builtin.image/extension.json`, `extensions/builtin.video/extension.json`

**Interfaces:**
- Consumes: `api.ts`; file options from Task 3.6 (paths inside `media_dir`).
- Produces: `builtin.image` (option `file` File, `fit` enum `cover|contain`, `cover`); `builtin.video` (option `file` File, `muted` boolean `true` and not changeable, `loop` boolean `true`). Video uses a `<video>` element with `playsinline muted`; it pauses when the page is hidden.
- [ ] **Step 1:** Implement both; build; set them with `option set` and `theme set` on a test theme.
- [ ] **Step 2:** Measure with the harness and `top` like the spike (spec §8): record CPU and memory for a 4K H.264/HEVC sample video in `docs/decisions/0002-video-cost.md`.
- [ ] **Step 3:** Commit the four files and the decision doc: `feat(web): add image and video backgrounds`.


### Task 4.2: Apple aerials background

Only if Task 0.2 showed the saver can read the aerials folder. If not, skip this task and say so in the README.

**Files:** Create `internal/aerials/aerials.go`, `internal/aerials/aerials_test.go`, `internal/aerials/testdata/entries.json`, `web/src/builtins/aerials.ts`, `extensions/builtin.aerials/extension.json`; Modify `internal/view/view.go`

**Interfaces:**
- Produces: `type Group string` (`landscapes`, `cities`, `space`, `underwater`, from `AerialCategoryLandscapes`, `AerialCategoryCities`, `AerialCategorySpace`, `AerialCategoryUnderwater`); `func Downloaded(entriesJSON, customerDir string, g Group) []string` (assets in the group whose video file exists locally; check the file naming against a real downloaded video during the task and record it in a comment); `view.Resolve` adds `files` (a list of `file://` URLs) to the `builtin.aerials` options. Manifest option `group` enum, default `cities`. The runtime plays the list shuffled; an empty list falls back to `builtin.matrix`, and `option set builtin.aerials group …` prints "Pick it once in System Settings → Screen Saver, and macOS will download the videos." (spec §7). Missing or moved folder → empty list, never an error.
- [ ] **Step 1: Tests** (table): `reads groups from fixture`; `only downloaded files listed`; `missing folder gives empty`.
- [ ] **Step 2:** `go test ./internal/aerials/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Commit the six files: `feat: add Apple aerials background from local downloads`.

---

## Phase 5: Menu bar app (gate: tests green, tray and settings window work on the owner's Mac)

### Task 5.1: Library check (throwaway spike)

**Files:** scratch only, outside the repo; then Create `docs/decisions/0003-tray-and-webview.md`

- [ ] **Step 1:** Check on GitHub that `fyne.io/systray` and `github.com/webview/webview_go` had releases or commits in the last 12 months and have no open issue saying they are broken on macOS 26. Note versions.
- [ ] **Step 2:** In a scratch module: a `tray` program (icon, two menu items, one opens the second program) and a `settings` program (webview window, one `Bind`-ed Go function called from JS that returns JSON). Build both universal (`CGO_ENABLED=1`, both arches, `lipo`). Run them on the owner's Mac.
- [ ] **Step 3:** Measure the tray process idle: CPU (with `top` deltas, as in spec §8) and memory. Expected: about 0% CPU.
- [ ] **Step 4:** Write `docs/decisions/0003-tray-and-webview.md`: versions, results, and any problem. If either library fails, stop and bring it to the owner with an alternative.
- [ ] **Step 5:** Commit the decision doc: `docs: record tray and webview library check`.

### Task 5.2: Tray

**Files:** Create `internal/tray/menu.go`, `internal/tray/menu_test.go`, `internal/tray/tray.go`, `internal/tray/icons/*.png`; Modify `cmd/agent-screensaver/main.go`, `internal/install/install.go`, `internal/install/install_test.go`

**Interfaces:**
- Consumes: `statefile.StateFile`, `settings.Service`, `platform.ServiceManager`.
- Produces:
  ```go
  type Item struct { ID, Label string; Enabled, Checked bool } // ID "" = plain text line
  type Model struct { Attention bool; Items []Item }
  func BuildMenu(f statefile.StateFile, cfg config.Config, nowMs int64) Model // pure
  func Run(ctx context.Context, p paths.Paths, svc settings.Service, openSettings func() error) error
  ```
  Menu lines per spec §7c: summary (`"1 waiting for approval · 2 running"` or `"All quiet"`), one line per limit (`"Claude 62% · resets in 1h40m"`, `"Codex week 55%"`), one item per approval card (`"Codex · api-server · waiting 3m"`), then `preview`, `settings`, `sound` (checked = on), `privacy` (checked = on), `quit`. `Run` rebuilds the menu from `state.json` every 2 s, swaps to the attention icon when `Attention`, and maps clicks: `preview` runs `open -a ScreenSaverEngine`, `settings` calls `openSettings` (starts `agent-screensaver settings` as a child process, one at a time), `sound` and `privacy` call `settings.Service`. Countdown text reuses the same rules as the web `formatResetIn` (Task 2.1); write the Go version once in `internal/tray/menu.go` and test it with the same cases.
  - CLI: `agent-screensaver tray`. Install (Task 3.5) gains one step: `ServiceManager.Install("dev.agentscreensaver.tray", [bin, "tray"])`, recorded in `Record.Services`.
- [ ] **Step 1: Tests** (`TestBuildMenu`, table): `all quiet`; `one approval two running`; `limits lines with reset countdown`; `reset in past says waiting`; `privacy hides project names`; `checked sound and privacy reflect config`. Plus `TestInstallAddsTrayService` in `install_test.go`.
- [ ] **Step 2:** `go test ./internal/tray/ ./internal/install/` → FAIL. **Step 3:** implement. **Step 4:** → PASS.
- [ ] **Step 5:** Manual check: `go run ./cmd/agent-screensaver tray`; feed an approval event with `hook`; the icon changes and the item appears within 2 s.
- [ ] **Step 6:** Commit the Go files, icons by name and the modified install files: `feat: add menu bar tray`.

### Task 5.3: Settings window

**Files:** Create `internal/settingsui/window.go`, `web/settings/index.html`, `web/settings/settings.ts`, `web/settings/form.ts`, `web/test/form.test.ts`, `internal/platform/filepicker_darwin.go`; Modify `internal/platform/platform.go`, `web/build.mjs`, `cmd/agent-screensaver/main.go`

**Interfaces:**
- Consumes: `settings.Service` (Task 3.6), the web runtime (Task 2.3) for the live preview, extension option schemas (Task 1.6).
- Produces:
  - `platform.FilePicker interface { PickFile(prompt string, types []string) (path string, ok bool, err error) }`; macOS: `osascript -e 'choose file ...'`.
  - `window.go`: `func Open(p paths.Paths, svc settings.Service, picker platform.FilePicker) error` opens a webview window (title "Agent Screen Saver", 960x640) on `runtime/settings/index.html` and binds exactly these JS functions, each a thin call into `settings.Service` returning JSON or an error string: `ags_snapshot`, `ags_setTheme`, `ags_setOption`, `ags_resetOption`, `ags_setPrivacy`, `ags_setSound`, `ags_setMaxFps`, `ags_installExtension` (opens `FilePicker` for `.zip` or a folder), `ags_removeExtension`, `ags_state` (current `state.json` for the preview).
  - `form.ts`: `formFor(options: Record<string, OptionSpec>, values: Record<string, unknown>): Field[]` with `type Field = { key: string; label: string; control: "text" | "number" | "toggle" | "color" | "select" | "file"; value: unknown; min?: number; max?: number; choices?: string[] }` (pure; one control per `OptionKind`).
  - `settings.ts`: renders the theme gallery (each theme card has a live preview: an `<iframe>` of `runtime/index.html` fed with `ags_state()` through `setState`), the per-extension forms from `formFor`, the extension list with install and remove, and the general section (sound, sound file, privacy, max fps). Every change calls the bound function, then re-reads `ags_snapshot`.
  - CLI: `agent-screensaver settings`.
- [ ] **Step 1: Tests** (`form.test.ts`): `number option becomes number field with range`; `enum becomes select`; `boolean becomes toggle`; `color becomes color`; `file becomes file`; `current value wins over default`; `label falls back to key`.
- [ ] **Step 2:** `npm test` → FAIL. **Step 3:** implement. **Step 4:** → PASS; `npm run typecheck` → PASS; `go vet ./...` clean.
- [ ] **Step 5:** Manual check: open Settings from the tray; switch theme (preview and the real screen saver change); change Matrix `fps` to 60; install an extension `.zip` from `testdata`; remove it.
- [ ] **Step 6:** Commit all listed files: `feat: add settings window`.

**Phase 5 gate:** `go test -race ./...`, `npm test`, `npm run typecheck` green; all manual checks in Tasks 5.2 and 5.3 pass on the owner's Mac.

---

## Phase 6: End-to-end on the owner's Mac (gate: every checklist line passes)

### Task 6.1: Real-world checklist

**Files:** Create `docs/e2e-checklist.md`; Modify fixtures in `tests/fixtures/` if real payloads differ.

- [ ] **Step 1:** Install with the release zip on the owner's Mac (owner runs `install`, approves the prompt, clicks "Open Anyway", approves Codex hooks with `/hooks`).
- [ ] **Step 2:** Capture one real payload of each hook event for both tools (temporarily set `AGS_RECORD_PAYLOADS=1`, see Task 3.2). Compare with the fixtures from Tasks 1.3 and 1.4. Fix fixtures and mapping where they differ; rerun `go test ./...`. Delete the recorded payloads afterwards (they contain prompts).
- [ ] **Step 3:** Run and tick in `docs/e2e-checklist.md`:
  - Claude task running → bottom card with prompt start and growing time.
  - Claude asks permission while the saver shows → orange card in center, clock moves top-left, one sound.
  - Claude idle "waiting for your input" → no orange card.
  - Codex `PermissionRequest` → orange card; deny the command → card becomes "check the terminal" after 10 min.
  - Finished task → blue card, gone after 30 min.
  - Claude Max and Codex rings match `/usage` (Claude) and `/status` (Codex) within 1%.
  - Mac locks: password asked when coming back.
  - After the saver stops: no `WebContent` process of ours and no `legacyScreenSaver` left after 10 s.
  - CPU while showing Matrix stays near the spike's ~6.5% of one core.
  - Second display shows only background and clock.
  - Existing status line still prints the same as before install.
  - Menu bar icon changes when an agent waits; menu lines match the screen saver.
  - Settings window: theme change shows in the preview and in the real screen saver.
  - `uninstall` leaves `~/.claude/settings.json` and `~/.codex/hooks.json` as they were, and removes both LaunchAgents.
- [ ] **Step 4:** Commit `docs/e2e-checklist.md` and any fixture or mapping fixes (explicit paths): `test: add end-to-end checklist and real payload fixtures`.

---

## Phase 6b: Agent extensions (owner decision 22/10/2026; gate: every task gate below, then Task 6.7 passes)

Spec §7d and `docs/decisions/0006-agent-extensions.md`. Tasks 6.2 (background count) and 6.3 (richer cards) are briefed in `.superpowers/sdd/2026-10-20-agent-screensaver-v1/task-6.2-brief.md` and `task-6.3-brief.md`. Task 6.4 starts only after Task 6.3 is merged and reviewed: it moves the 6.3 readers (status line `session_name`, context and model; transcript titles; Codex session index and rollout context) into manifests, and 6.3 changes the same packages (`agents`, `event`, `state`, `statusline`, `cli`). The order is strict: 6.4 → 6.5 → 6.6 → 6.7 (shared files, no parallel writers).

Rules for every task in this phase:

- The hook stays under 100 ms and always exits 0 (Review Focus 2 holds for every agent, with and without a converter).
- Install and uninstall keep the Global Constraints: only add, remove exactly what was added, key order kept, `internal/jsonedit` only, backups, nothing written when one file is invalid (Review Focus 3).
- Built-in agent manifests go through the same validation and safety checker as outside ones. No `if id == "builtin.claude"` anywhere in Go.
- The mapping language is exactly spec §7d. A case it cannot express uses a converter; the language does not grow in these tasks.

### Task 6.4: Agent extension engine, and move Claude Code and Codex into it

**Files:**
- Modify: `internal/extensions/extensions.go`, `internal/extensions/extensions_test.go` (type `agent`, manifest validation, mark check, unique source).
- Create: `internal/agents/manifest.go` (the `agent` manifest types), `internal/agents/mapping.go` (rules, conditions, values), `internal/agents/transforms.go`, `internal/agents/converter.go`, `internal/agents/load.go` (find agents in the extensions folder); rewrite `internal/agents/agents.go`; tests `internal/agents/mapping_test.go`, `internal/agents/converter_test.go`, `internal/agents/equivalence_test.go`, `internal/agents/testdata/converter/*`.
- Create: `internal/install/recipe.go` (plan and apply for one agent), `internal/install/safety.go`, `internal/install/shape.go` (`grouped`, `flat`; moved from `internal/agents/hookconfig.go`), `internal/install/format.go` (`json`), `internal/install/record.go` (record v2 and v1 upgrade); tests `internal/install/recipe_test.go`, `internal/install/safety_test.go`, `internal/install/record_test.go`; modify `internal/install/install.go`, `internal/install/install_test.go`.
- Create: `extensions/builtin.claude/{extension.json,mark.svg}`, `extensions/builtin.codex/{extension.json,mark.svg}`.
- Modify: `internal/cli/hook.go`, `internal/cli/statusline.go`, their tests; `internal/daemon/daemon.go` (details reader, `agents_off`, last event per source); `internal/state/state.go` (`sources` map with `last_event_ms`, read back on start); `internal/config/config.go` (`agents_off`); `internal/view/view.go` (agent names and marks in `state.json`); `web/src/api.ts` and the card component (mark from state, neutral dot fallback).
- Delete (last step, after the equivalence gate): `internal/agents/claude.go`, `codex.go`, `install_claude.go`, `install_codex.go`, `hookconfig.go`, and the 6.3 source-specific readers that moved into manifests.
- Docs: create `docs/agents.md` (author guide: manifest, mapping language, converter contract, recipe, safety rules, details, a full opencode-shaped example marked "not tested"); modify `docs/extensions.md` (sixth type `agent`, link), `docs/event-format.md` (the kinds table names the built-in manifests instead of Go).

**Interfaces:**
- Consumes: `extensions.Manifest`, `event.Event` (with the 6.3 fields `name`, `model`, `context_percent`, `cwd`, `transcript`, kind `info`), `text.PromptTitle`, `text.ShortTitle`, `text.MaskSecrets`, `agents.DescribeTool`, `jsonedit.Object`, `paths.Paths`.
- Produces:
  ```go
  // package agents
  type Agent struct {
      ID, Name, Source, Dir, Mark string
      Hooks      Mapping
      Statusline *Mapping
      Install    Recipe
      Details    *Details
      Limits     string // provider id, "" for none
      Builtin    bool
  }
  func Load(extensionsDir string) ([]Agent, error)       // every valid agent manifest; invalid ones skipped and logged
  func BySource(agents []Agent, source string) (Agent, bool)
  func (m Mapping) Map(payload []byte, eventArg string, nowMs int64) ([]event.Event, bool) // rules path; false = ignore
  type Converter struct { Run string; Timeout time.Duration }
  func RunConverter(ctx context.Context, c Converter, payload []byte, env []string) ([]event.Event, error) // output parsed, source forced by the caller
  func Normalize(e event.Event, source string, nowMs int64) (event.Event, error) // force source, fill v/at_ms, mask, cut, validate
  ```
  ```go
  // package install
  type Recipe struct { Detect string; Hooks *HookRecipe; Statusline *StatuslineRecipe; Files []FileCopy; Notes []string }
  func CheckRecipe(home, dataDir string, r agents.Recipe) error // the safety rules of spec §7d; used at ext install and before every apply
  func PlanAgent(home, bin string, a agents.Agent, rec Record) ([]PlannedStep, error)
  func AddAgentHooks(home, bin string, a agents.Agent, rec *Record) error
  func RemoveAgentHooks(home string, a agents.Agent, rec *Record) error
  type AgentRecord struct {
      ID, Source, File string
      Events, Commands []string
      CreatedFile     bool
      StatuslineKey   string `json:",omitempty"`
      PreviousCommand string `json:",omitempty"`
      Files           []CopiedFile // path + sha256
  }
  // Record gains Version int and Agents []AgentRecord; LoadRecord upgrades version 1.
  ```
  Hook shape and config format are small unexported interfaces in `internal/install` (spec §7b table), one implementation each in version 1 except shapes (`grouped`, `flat`).

**Part A: manifest and mapping engine.**
- [ ] **Step 1: Tests** (`TestValidateAgentManifest`, table): `valid claude manifest`; `missing mark` → error; `mark over 8 KiB` → error; `mark with script` / `with foreignObject` / `with onload` / `with external href` / `with url(http` → error each; `source with uppercase` → error; `unknown kind in rule` → error; `rule with converter and rules` → error; `unknown transform` → error; `two operators in one condition` → error; `unknown limits provider` → error; `install of second agent with same source` → error.
- [ ] **Step 2: Tests** (`TestMapping`, table over inline payloads): `path`, `array index path`, `first non-empty of paths`, `const`, each transform (`basename`, `prompt_title` skips a `<system-reminder>` block, `short_title`, `model_name`, `count` of `background_tasks`, `percent` clamps 130 to 100, `text` joins a list); `tool value gives "Bash: npm run migrate"`; conditions `equals`, `equals null only when key is null` (missing key → false), `in`, `contains ignores case`, `present true and false`; `first matching rule wins`; `no rule matches → ignore`; `missing session → ignore`; `rule without on matches every event`; `event from --event arg`; `secret in title is masked even without a transform`; `statusline mapping emits limits and info from one payload`; `limits reading without used_percent dropped`. `TestReply` (table): `reply none prints nothing`; `empty_object prints {}`; `per-event ask prints {"permission":"ask"} for that event and the default for others`; `reply comes from --event even when the payload is invalid JSON`; `reply printed when the agent is off`; `reply printed when the converter times out`. `TestValidateAgentManifest` gains: `reply allow` / `deny` / any other value → error.
- [ ] **Step 3: Tests** (`TestConverter`, with tiny shell scripts in `testdata/converter/`, executable): `prints one event`; `prints two lines → two events`; `empty output → ignore`; `five lines → only 4 kept`; `output over 64 KiB → dropped`; `sleeps 2 s → killed at timeout, nothing sent, under 600 ms`; `source in output is replaced by the manifest source`; `invalid event dropped`; `non-executable file refused at validation`; `env holds only HOME, PATH, AGS_SOURCE, AGS_EVENT`.
- [ ] **Step 4:** Implement Part A. Write `builtin.claude` and `builtin.codex` manifests from the Go adapters (hooks, status line mapping, the 6.3 details). Draw the two marks (simple shapes, own colors; spec §7d "The mark").
- [ ] **Step 5: Equivalence gate** (`TestBuiltinManifestsMatchGoAdapters`): for every file in `internal/agents/testdata/{claude,codex}/` and `internal/statusline/testdata/`, the old Go path and the manifest give the same events (compare `event.Event` values). Run it while the old adapters still exist. It must pass before Step 6.
- [ ] **Step 6:** Switch `cli.Hook` and `cli.Statusline` to `agents.Load` + `BySource`. `hook --tool <source>` reads `config.json` (`agents_off`); for an agent that is off it prints the manifest reply and exits at once. Delete the old adapters; turn the equivalence test into a golden test (fixture → expected event JSON in `testdata/golden/`). Add `TestHookFastWithManyExtensions` (50 extension folders, 10 of them agents, one payload → under 100 ms, exit 0) and `TestHookConverterTimeoutStillFast` (converter sleeps → hook returns under 600 ms, exit 0).
- [ ] **Step 7:** Commit (explicit paths): `feat(agents): map hook payloads with agent extension manifests`.

**Part B: install recipe engine.**
- [ ] **Step 8: Tests** (`TestCheckRecipe`, table): `claude, codex and cursor built-ins pass`; `path without ~/` → error; `path with ..` → error; `symlink to outside home` → error (temp home with a symlink); each deny-list entry → error (`~/.zshrc`, `~/.ssh/config`, `~/Library/LaunchAgents/x.plist`, data dir file, `~/.config/fish/config.fish`); `hook file .toml` → error ("toml is not supported yet"); `handler key command` / `type` / `failClosed` → error; `timeout 0 and 31` → error; `matcher not a string` → error; `files target exists and is not ours` → error at apply; `no hooks and no files` → error.
- [ ] **Step 9: Tests** (`TestAgentInstall`, table, temp home): every existing case of `install_test.go` runs unchanged against the built-in manifests, and the written Claude and Codex files are byte for byte the files written before this task (keep copies of today's outputs as `testdata/golden/*.json` before changing code); `TestUpgradeFromRecordV1` (a v1 record and files written by the old code → one install → hook entries unchanged, status line command becomes `statusline --tool claude`, saved previous command kept, record v2, each file written once); `TestUninstallUsesRecordOnly` (manifest edited after install to list another event → uninstall removes only the recorded entries); `TestUninstallAgentWhoseExtensionIsGone` (record path is re-checked by `CheckRecipe`, only our exact commands removed); `TestCopiedFileRemovedOnlyIfHashMatches`; `TestCreatedFileRemovedWhenOnlyNewFileKeysLeft`; `TestPlanNamesOutsideAgentAndConverter` (plan lines say "from acme.x 1.0.0" and "runs its own program", and the plan carries a warning flag the CLI and the settings dialog must show); `TestEventWithReplyGetsEventArg` (an event with its own reply gets `--event <name>` in its command; the record stores that exact command).
- [ ] **Step 10:** Implement Part B. `Apply` loops over `agents.Load` and adds hooks for each built-in agent whose `detect` path exists (`~/.claude`, `~/.codex`, `~/.cursor`). Today install adds Claude and Codex hooks even when the tool is missing; this changes that (owner decision 23/10). Test: `TestInstallSkipsAgentWhoseDetectIsMissing`. `Uninstall` loops over the record. `ext remove <id>` of an agent removes its hooks first and refuses when that fails. `ext install` of an agent runs `CheckRecipe` and the manifest checks, copies the folder, and adds no hooks.
- [ ] **Step 11:** Commit (explicit paths): `feat(install): add and remove hooks from agent install recipes`.

**Part C: details and marks.**
- [ ] **Step 12: Tests** (`TestDetails`, table, temp home, real-shape redacted fixtures from 6.3): `codex name from session_index last matching line`; `codex context from rollout tail ratio`; `claude name from custom-title, then ai-title`; `event name wins over details name`; `file over tail_kb reads only the tail` (10 MiB file, reads at most 256 KiB, under 50 ms); `jsonl first partial line dropped`; `path outside home refused`; `{transcript} placeholder from the card's event`; `details converter output used, timeout respected`; `read at most once per card per 5 s`. `TestStateCarriesAgentMarks` (state.json lists name and mark path per source); `TestLastEventPerSource` (each valid event sets `sources.<source>.last_event_ms`; an ignored-project event counts; an invalid file does not; the map survives a daemon restart). Web: `card shows the agent mark from state`, `card without agent shows a neutral dot and the source name`.
- [ ] **Step 13:** Implement Part C; move the 6.3 readers behind the manifest schema.
- [ ] **Step 14:** Commit (explicit paths): `feat(daemon): read card details declared by agent extensions`.
- [ ] **Step 15:** Write `docs/agents.md` and update `docs/extensions.md` and `docs/event-format.md`. Commit: `docs: describe agent extensions`.

**Gate:** `go vet ./...`, `go test -race ./...` (with integration tests), `npm test`, `npm run typecheck`, build. Review against spec §7b and §7d: no Go code names a built-in agent; the old adapters are gone; the install golden files are unchanged.

### Task 6.5: Cursor as a built-in agent extension

**Files:** Create `extensions/builtin.cursor/{extension.json,mark.svg}`, `internal/agents/testdata/cursor/*.json` (real redacted payloads), `internal/agents/testdata/golden/cursor/*.json`, `internal/install/testdata/cursor_hooks.json` (shaped like the owner's file: `version: 1` and entries of four other tools, all values fake); Modify `internal/agents/mapping_test.go`, `internal/install/recipe_test.go`, `docs/agents.md`, `docs/e2e-checklist.md` (the Cursor behavior table of Step 2), `README.md` (Cursor in the supported list).

**Interfaces:**
- Consumes: everything from Task 6.4. No Go changes are expected. If one is needed, it is a gap in the engine: stop and report it, do not add a Cursor case to Go.
- Produces: the manifest of spec §7d "Example: the built-in Cursor extension" (permission events `beforeShellExecution` and `beforeMCPExecution` with reply `ask`), corrected by real payloads, or the fallback manifest without permission events (Step 3).

- [ ] **Step 1: Payloads.** Install the manifest on the owner's Mac (after the owner agrees), run `agent hooks add builtin.cursor`, record payloads (`AGS_RECORD_PAYLOADS=1` for the Cursor app, or the hook's own recorder), and run one short Cursor agent task with one shell command and one MCP call. Record one payload of each subscribed event. Check: the field names; `workspace_roots`; whether `conversation_id` is stable across a session; whether the non-permission events need `{}` (try default `none` first on a throwaway test; if Cursor shows a hook error or changes behavior, use `empty_object`). Redact, save as fixtures, delete the recordings.
- [ ] **Step 2: Live behavior test (owner decision 23/10: Cursor must ask exactly as before).** On the owner's Mac, run each case twice, first with our hooks removed, then with our hooks added, and write both results in a table in `docs/e2e-checklist.md`:
  - a shell command Cursor asks for (not on its allow list): Cursor shows its approval prompt once in both runs; with our hooks, the saver shows the approval card while Cursor waits, and the card clears after the command runs (`afterShellExecution`);
  - the same command, denied in Cursor: denied in both runs, no command runs; the card clears or turns "unknown" by the 10-minute rule;
  - a shell command Cursor runs without asking (allow-listed, or auto-run mode if the owner uses it): it runs without a prompt in both runs;
  - an MCP tool call that asks, and one that does not ask: same in both runs;
  - Cursor's hooks output panel shows no error from our hook in any case.
- [ ] **Step 3: Fallback.** If any case in Step 2 differs between the two runs, remove `beforeShellExecution`, `afterShellExecution`, `beforeMCPExecution` and `afterMCPExecution` from the recipe events, the rules and `reply.events`, re-add hooks, and re-run Step 2 to show that Cursor now behaves exactly as without our hooks. Record which case failed in `docs/e2e-checklist.md` and in the task report. Cursor cards then show only working and done; tell the owner.
- [ ] **Step 4: Tests:** `TestMapping` rows for each Cursor fixture (`beforeSubmitPrompt → started with prompt title`, `beforeShellExecution → approval with the command as title`, `beforeMCPExecution → approval with the tool name`, `afterShellExecution → resumed`, `postToolUse → resumed`, `stop with status aborted → finished`, `sessionEnd → ended`, `preCompact → info with context_percent`, `afterAgentResponse → ignored`, `missing conversation_id → ignored`); `TestReply` rows: `cursor beforeShellExecution prints {"permission":"ask"}`, `cursor beforeMCPExecution prints ask`, `cursor stop prints the default`, `cursor permission event with garbage stdin still prints ask and exits 0 under 100 ms`; `TestAgentInstall` rows: `cursor into missing file` (file is `{"version": 1, "hooks": {...ours}}`); `cursor keeps other tools' entries and key order`; `cursor permission events get --event in the command`; `cursor uninstall restores the original after re-indent` (compare with `json.Indent` of the original); `cursor uninstall of a file we created removes the file`; `cursor handler has timeout and no failClosed`.
- [ ] **Step 5:** Fix the manifest until all pass. **Step 6:** Commit (explicit paths): `feat(agents): add Cursor as a built-in agent extension`.

**Gate:** `go test -race ./...` green; Step 2 shows identical Cursor behavior with and without our hooks (or Step 3 was applied and re-checked); a live Cursor task shows a working card, an approval card while Cursor asks, then a done card, with the Cursor mark and the project name.

### Task 6.6: Connections tab in the settings window, and `agent` CLI commands

**Files:** Modify `internal/settings/settings.go`, `internal/settings/settings_test.go`, `internal/settingsui/window.go`, `internal/settingsui/window_test.go`, `internal/cli/settings.go`, `cmd/agent-screensaver/main.go`, `web/settings/settings.ts`, `web/settings/index.html`; Create `internal/settings/connections.go`, `internal/settings/connections_test.go`, `web/settings/connections.ts`, `web/test/connections.test.ts`. Load the design skills `emil-design-eng`, `impeccable` and `design-taste-frontend` before the UI step (owner rule).

**Interfaces:**
- Consumes: `agents.Load`, `install.PlanAgent`, `install.AddAgentHooks`, `install.RemoveAgentHooks`, the install record, `config.AgentsOff`, `state.json` `sources.<source>.last_event_ms` (Task 6.4 Part C).
- Produces:
  ```go
  // package settings
  type ConnectionState string // "not_installed" | "no_hooks" | "active" | "idle" | "off"
  const ActiveWindowMs = 10 * 60 * 1000 // spec §7d: an event in the last 10 minutes means active
  type Connection struct {
      ID, Name, Source, Mark, Version string
      Builtin      bool
      State        ConnectionState
      LastEventMs  *int64 // nil: no event yet
      HooksChanged bool   // the record says added, the file no longer holds our entries
      Warnings     []string // converter or plugin file lines, shown in the plan dialog
  }
  // ConnectionStateFor is pure: the order is off, not installed, no hooks, then active or idle.
  func ConnectionStateFor(found, hooksAdded, off bool, lastEventMs *int64, nowMs int64) ConnectionState
  func (s Service) Connections(nowMs int64) ([]Connection, error)
  func (s Service) SetAgentOn(id string, on bool) error
  func (s Service) PlanAgentHooks(id string, add bool) (plan []install.PlannedStep, hash string, err error)
  func (s Service) ApplyAgentHooks(id string, add bool, hash string) error // recomputes the plan; refuses when the hash differs
  ```
  Bridge methods (ids and booleans only, never paths): `connections`, `setAgentOn`, `planAgentHooks`, `applyAgentHooks`. CLI: `agent list` (same states and last event as the tab), `agent on|off <id>`, `agent hooks add|remove <id>` (prints the plan and the warnings, asks `Proceed? [y/N]`, `--yes` skips).
  `connections.ts`: `connectionRows(list: Connection[], nowMs: number): ConnectionRow[]` (pure: state text such as "Active · 4m ago", "Idle · 3h", "Idle · no events yet", "Hooks not added", "Not installed", "Off"; badge "built in" or "from <id> <version>"; last event text; which buttons show: "Add hooks" or "Remove hooks", "Turn off" or "Turn on"). Times reuse the existing duration formatting of the cards (no second formatter).
- [ ] **Step 1: Tests:** `TestConnectionStateFor` (table: `off wins over everything`; `detect missing → not_installed`; `found, no hooks → no_hooks`; `event 9m ago → active`; `event 10m ago → idle`; `no event yet → idle`); `TestConnectionsListsBuiltinsAndOutside`; `TestConnectionsReadLastEventFromState`; `TestHooksChangedWhenUserRemovedOurEntries`; `TestSetAgentOnPersistsAgentsOff`; `TestApplyRefusesStaleHash` (plan, then change the hook file, then apply with the old hash → error, file unchanged); `TestApplyAddsThenRemoves`; `TestPlanOfOutsideConverterAgentHasWarnings`; bridge: `TestBridgeAgentMethodsTakeIdsOnly` (unknown fields ignored, a `path` param has no effect), `TestBridgeRefusesWhilePicking`. Web: `active row says Active and minutes`; `idle row says Idle and hours`; `idle with no events says no events yet`; `not installed row has no hook buttons`; `no hooks row offers Add hooks`; `changed hooks row says so and offers Add hooks`; `off row offers Turn on and keeps hook buttons`; `outside agent shows id and version`; `plan dialog shows warnings and the trust button for an outside converter`.
- [ ] **Step 2:** `go test ./internal/settings/ ./internal/settingsui/` and `npm test` → FAIL. **Step 3:** implement (the tab refreshes every 5 s while open; the plan dialog lists every plan line; buttons "Add hooks" / "Remove hooks" / "Cancel", or "I trust <name>, add hooks" when there are warnings). **Step 4:** → PASS; typecheck.
- [ ] **Step 5:** Manual check on the owner's Mac: the tab shows Claude Code, Codex and Cursor with their marks, states and last event times; a task in Claude makes its row "Active · 0m ago"; turning Codex off shows "Off" and stops new Codex cards, on brings them back; "Remove hooks" for Cursor shows the plan and removes our entries only (row becomes "Hooks not added"); "Add hooks" puts them back; a tool that is not on the Mac shows "Not installed".
- [ ] **Step 6:** Commit (explicit paths): `feat(settings): add the Connections tab`.

**Gate:** `go test -race ./...`, `npm test`, `npm run typecheck` green; the manual check passes.

### Task 6.7: Real-world re-check after agent extensions

These changes touch every hook, the status line and install. Re-run the checks of Task 6.1 that they can break, on the owner's Mac, from a release build. Tick each line in `docs/e2e-checklist.md` under a new heading "After agent extensions (Tasks 6.4 to 6.6)".

- [ ] **Step 1:** Before: copy `~/.claude/settings.json`, `~/.codex/hooks.json`, `~/.cursor/hooks.json` and the install record to the scratchpad (owner agrees first).
- [ ] **Step 2:** Upgrade: the owner runs `install` from the release zip over the existing install. Check: the plan lists only the status line command change for Claude and the new Cursor entries; after it, `diff` of the Claude and Codex files against the copies shows only the status line command; the Cursor file keeps every other tool's entry.
- [ ] **Step 3:** Hooks, each tool, with the saver showing:
  - Claude: running card with title, approval card + sound, idle notification gives no approval, done card with background count, status line still prints the user's own line, Claude rings update, conversation name and context bar shown.
  - Codex: running card, `PermissionRequest` approval card, done card, helper sessions make no card, Codex rings update (provider by manifest id), session name appears after the first turn, context bar.
  - Cursor: working card, approval card + sound while Cursor asks for a shell command (Cursor still asks exactly as before: repeat the Task 6.5 Step 2 table on the release build), done card, context after a compaction.
  - Hook timing: `hook-errors.log` has no new lines; a hook run with a real payload takes under 100 ms (`time` on a recorded payload).
- [ ] **Step 4:** Connections tab: each row shows the right state and last event time; turn each agent off and on; remove and add hooks for each tool; re-run one task per tool.
- [ ] **Step 5:** An outside agent: install a test agent extension from a folder (id `test.echo`, source `echo`, a converter that prints one `started` event, hook file `~/agent-screensaver-test/hooks.json`, so no real tool config is touched). Check: the plan shows "from test.echo" and the warning "runs its own program", and needs the explicit trust confirmation; the hook entry goes into the test file; feeding one payload to `agent-screensaver hook --tool echo` makes a card with source `echo` and the row turns "Active"; `agent off test.echo` stops new cards; `ext remove test.echo` removes its hook entry first, then the folder.
- [ ] **Step 6:** Uninstall: `uninstall` leaves the Claude, Codex and Cursor files equal to the Step 1 copies after `json.Indent` (whitespace may differ only where the original was not two-space indented), restores the status line, removes both LaunchAgents.
- [ ] **Step 7:** Reinstall (the owner's normal state). Delete the copied payloads and any recordings. Commit `docs/e2e-checklist.md`: `test: re-check hooks and install after agent extensions`.

**Gate:** every line in Steps 2 to 6 passes.
