# 0006: Agents are extensions (declarative manifest with a converter escape hatch)

Status: accepted (2026-10-23). The owner approved the direction on 2026-10-22 and the spec (§7d) on 2026-10-23, with the answers listed under "Owner answers".

## Problem

Each coding agent (Claude Code, Codex, Cursor, opencode, pi, Hermes and more) has its own hook events, payload fields and config file. Until now each one was Go code: an `agents.Adapter` for the mapping and a `HookInstaller` for the config file. A user who wants a new agent must change our code and build it.

The owner decided (22/10/2026): users must be able to add and remove support for agents without changing our code. The built-in agents must use the same format, with no private shortcuts (spec §7a).

## What a solution must do

- Map a hook payload to our event (`docs/event-format.md`): session, project, title, tool and command, approval, background count, model, transcript path, context use.
- Add our hooks to the tool's config, and remove exactly those, with the guarantees we have today (key order kept, other entries untouched, backups, install record).
- Keep the hook under 100 ms and always exit 0.
- Be safe when the extension comes from a stranger: it must not be able to write anywhere or run anything without the user seeing it.
- Stay simple: one engine, small data, testable.

## Options

### A. Go adapters only (today)

Each agent is a Go file plus a line in a registry.

- Good: full power, type checked, no new format.
- Bad: no user can add an agent without a build. It breaks the owner's decision. Every new agent is a pull request to us.

### B. Declarative manifest only

The manifest says, as data, which payload fields give each event field, which event names become which kinds, and where the hooks go. One Go engine reads it.

- Good: no outside code runs. It is easy to check, test and show to the user. Claude, Codex and Cursor fit it (spec §7d shows the rules).
- Bad: a tool with a strange payload (a value that needs arithmetic, a key with a dot, a nested list to search) cannot be mapped. Each such case would push us to grow the language, and a growing language becomes a scripting language.

### C. Converter program only

The manifest names a program. Our hook passes it the payload; it prints our event.

- Good: any tool fits. The format stays tiny.
- Bad: every agent, even a simple one, is code from someone else that runs on each hook event. It costs a process start (a few to tens of milliseconds) on the hot path. The user cannot read what it does. The built-ins would be programs too, or would need a private shortcut.

### D. Both: declarative manifest, with a converter as the escape hatch

The manifest is the normal way. A manifest may name a converter instead of rules, for the rare tool that needs it. The install recipe is always data, never a program.

- Good: simple agents are pure data, safe to share and easy to check. Hard cases still work. The built-ins are pure data, so the format is proven by real use.
- Bad: two ways to do the mapping, so two paths to test. The converter path still runs outside code, so it needs its own consent and limits.

## Choice

Option D.

Why:

- The three real agents we know well (Claude Code, Codex, Cursor) fit the small language fully. The two we checked from outside (opencode, pi) fit with a copied plugin file and the event name passed as an argument. So the data path covers the common case, and a converter is rarely needed.
- The install recipe is the dangerous part, because it writes to the user's files. Keeping it data only (and never a program) lets one Go engine apply every safety rule the same way for every agent.
- The owner's code principles (spec §7b) ask for one small engine, not a plugin system. The language has paths, constants, seven named transforms and four match operators. It is not allowed to grow loops, expressions or templates; a case that needs them uses a converter.

What stays Go: the mapping engine, the transforms (card text rules and secret masking), the install engine (formats, shapes, safety checks, record, backups), the status line runner, the details reader and the branch reader, and the Codex limits provider (JSON-RPC is not worth a data language for one user).

## Safety model

The full rules are in spec §7d "Safety rules for recipes". In short:

- **The command is ours.** A manifest never sets the command a tool runs. The engine always writes `'<bin>' hook --tool <source>`. So a manifest alone cannot make a tool run a stranger's program.
- **Writes are narrow.** Only our handler inside `hooks.<event>` of the declared file, the `new_file` keys when we create the file, the declared status line key, and declared plugin files that do not exist yet. Only under the user's home, never in a deny list (shell start files, `~/.ssh`, LaunchAgents, our data dir and others), and the hook file must be `.json`.
- **No decisions for the user.** Our hook prints only a fixed reply from the manifest: nothing, `{}`, or `{"permission": "ask"}`. The engine refuses any other reply, so our hook never allows or denies a tool action. It prints the reply first, also on errors. Handler keys that could make our hook block a tool (`failClosed`) are refused.
- **The user sees everything first.** A converter or a plugin file from an outside extension is allowed only after a clear warning in the plan and the user's explicit confirmation.
- **The plan is the contract.** Install shows a plan: each file, event, command, copied file and converter, and whether it is built in or from outside. Nothing is written before the user confirms, in the CLI or in the settings window.
- **Exact removal.** A version 2 install record lists what was written per agent, with hashes of copied files. Uninstall removes only that.
- **Outside code is visible and bounded.** A converter runs without a shell, with a small environment, a timeout (500 ms at most for hooks), capped output, and a forced `source`. Every text it returns is masked and checked like any event. A copied plugin file is named as "a file that <tool> will run".
- **Same rules for built-ins.** A test runs the checker on every built-in manifest.

What the model does not cover: a converter or a plugin file is code the user chose to trust. We cannot sandbox it on macOS without signing and entitlements we do not have. The plan says so in plain words.

## Costs

- Work: Task 6.4 (engine, recipe engine, move Claude and Codex), 6.5 (Cursor), 6.6 (settings Connections tab), 6.7 (real-world re-check). The move must prove, with the existing fixtures and install tests, that Claude and Codex behave exactly as before.
- The hook now reads manifests from disk on every event. It must stay under 100 ms with 50 installed extensions; a test checks it.
- A new public format to keep stable (`api: 1`). Changing it later needs a version bump and old manifests keep working.
- Cursor approval cards need our hook to answer Cursor's permission hooks with "ask". If a live test shows that "ask" changes Cursor's behavior (for example for commands Cursor runs without asking), the Cursor recipe drops those events and Cursor shows no approval cards.
- TOML configs are not supported in version 1 (no byte-preserving TOML editor in the standard library).

## Alternatives not taken

- A small expression language (JSONata, CEL, jq): powerful, but it is a scripting language with its own dependency, and the owner's rules allow only one third-party Go module.
- WebAssembly converters: safe sandbox, but heavy for this project and a new dependency.
- Letting the manifest set the hook command: simplest for authors, but then a manifest alone can make every tool run any program. Rejected.

## Owner answers (2026-10-23)

1. Cursor approval cards are in version 1. The Cursor recipe subscribes to `beforeShellExecution` and `beforeMCPExecution`, and our hook always answers "ask". The reply is a manifest field per event. The engine allows only "ask" or `{}`. A live test must show Cursor asks the user exactly as before; if not, the recipe drops these events.
2. Install skips a tool whose `detect` path is missing.
3. Outside converters and plugin files are allowed after a clear warning in the plan and explicit confirmation.
4. Protected paths are a deny list.
5. JSON configs only in version 1.
6. Turning an agent off keeps its hooks; the agent is silent. The settings list becomes a "Connections" tab with states (not installed, no hooks, active, idle, off) and the last event time per source, which the daemon records.
7. Hermes is checked later.

## Addendum (2026-10-24): Cursor commands are working steps

The live test (Task 6.5, Cursor 3.26.16, "Run Everything" mode) showed that Cursor behaves the same with our "ask" hooks as without them. It also showed that Cursor sends `beforeShellExecution` and `beforeMCPExecution` before every command, also when it will not ask, and the payload does not say whether it will. An approval card from these events would be false for every command in that mode and would play the approval sound. Owner decision: the two events map to `tool_intent` (the command shows on the working card), and Cursor never makes an approval card. The hook still answers "ask", because Cursor needs an answer and the live test showed it is harmless. This replaces answer 1 of 2026-10-23 for the cards; the "ask" reply stays. Cursor has no usage ring: its usage is on its server only.

## Addendum (2026-10-24, later): Cursor gets no reply

Cursor 3.27.12 does honor our "ask" in "Run Everything" mode. Its hooks log showed `beforeShellExecution` answered `{"permission": "ask"}` by our hook, then no step for 49 minutes, while the card said "Working" and the owner thought the chat was done. As this decision planned, the "ask" goes: `builtin.cursor` has no `reply`, so our hook prints nothing to Cursor (its log shows "produced no output" as normal for every other step), and Cursor decides alone. The two events stay `tool_intent`. Commands written by an older install still carry `--reply ask`; that flag is used only when the manifest cannot be read, and Settings > Connections repairs them.
