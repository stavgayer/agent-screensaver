# Changelog

The release notes of agent-screensaver, newest first. This file is where they are written: the message of each release tag is made from the section of its version, and CI refuses to release a version that has no section here. See the README for how to release.

## v0.5.1 (2026-11-02)

## The command is on your PATH

`agent-screensaver install` now makes the `agent-screensaver` command work in Terminal, with no alias and no admin password:

- It links the command at `~/.local/bin/agent-screensaver`.
- If `~/.local/bin` is not on your PATH, it adds one marked block to `~/.zprofile` (zsh) or `~/.bash_profile` (bash), after a backup. The install plan lists both before you confirm.
- For other shells, like fish, it changes nothing and tells you to add `~/.local/bin` to your PATH yourself.

Open a new Terminal window after installing. `agent-screensaver uninstall` removes the link and only the marked block; the rest of your profile stays as it was.

## Also

- The README's "In Terminal" section exists again, so the links to it from the themes now work.

## v0.5.0 (2026-11-02)

## For theme makers

- **A theme can say which version it needs.** Add `"requires": "0.5.0"` to an extension's `extension.json`. If someone has an older agent-screensaver, the install stops and tells them to update first, instead of installing a theme that does not work. A theme pack with one such part is refused as a whole.
- **Shared helpers.** `window.agentScreensaver.helpers` now gives every extension two helpers that each theme used to copy:
  - `songProgress(track, now)`: how far the playing song is, from 0 to 1, or nothing when the app gives no length.
  - `recoverWebGL(background, canvasOf)`: starts a WebGL background again on a fresh canvas after sleep or a graphics reset.
- **API version 2.** An extension that uses the helpers declares `"api": 2` and `"requires": "0.5.0"`. Older versions of agent-screensaver accept only api 1, so they refuse it cleanly.

See "Shared helpers" in docs/extensions.md.

## For everyone

Nothing changes on your screen. Every installed theme keeps working.

## v0.4.4 (2026-11-02)

## Remove a theme from the settings window, completely

Removing a theme with Remove… under Extensions now works like `agent-screensaver ext remove`: the theme goes together with the parts it brought, such as its layout, its background and its widgets. A part stays when another theme still uses it.

- Removing the theme you use now switches the screen saver back to the Default theme. Before, the setting kept pointing at the removed theme.
- The window says what it removed, for example "Removed Kid A and 5 parts it brought".

## Install

Download `agent-screensaver-v0.4.4-macos.zip`, unzip it and follow the README. Already on 0.3.x or 0.4.x? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

This is the first release built and published by CI.

## v0.4.3 (2026-11-02)

## Safer screen saver, clearer docs

- The screen saver now refuses any navigation away from its own local page, as the settings window already did. An extension's script can no longer take the page to a web address.
- The guides no longer suggest that `agent-screensaver` is on your PATH after install. They say how to run it: `./agent-screensaver` from the unzipped folder, or with a one-line alias for the installed copy.
- `go.mod` now asks for Go 1.24, which the tests need.

## Install

Download `agent-screensaver-v0.4.3-macos.zip`, unzip it and follow the README. Already on 0.3.x or 0.4.x? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.4.2 (2026-10-31)

## See which version you run

The settings window now shows the agent-screensaver version at the end of General, under About. Each extension's version is still under Extensions.

## The menu bar icon comes back when you open the app

Before, "Quit menu bar app" kept the icon away until your next login, and opening Agent Screensaver from Spotlight or Launchpad only opened the settings window. Now opening Agent Screensaver also starts the menu bar app again if it is not running. A running menu bar app is left as it is.

## Install

Download `agent-screensaver-v0.4.2-macos.zip`, unzip it and follow the README. Already on 0.3.x or 0.4.x? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.4.1 (2026-10-31)

## Chrome shows what's playing

The Chrome theme now shows the song that Spotify or Music plays, in its own style: the song in capitals, the artist in the lime tone of the ring captions, and a thin bar like the context bar on the task cards. It stands under the clock and the date. Turn it on in the settings window under General, Music, "Show what's playing".

With this, all three built-in themes show the song. The album themes do too, from [agent-screensaver-themes v1.1.0](https://github.com/stavgayer/agent-screensaver-themes/releases/tag/v1.1.0).

## Also

- The README now says that Calm shows the Matrix background until macOS has downloaded an aerial video. To get one, pick an aerial once in System Settings, Screen Saver.

## Install

Download `agent-screensaver-v0.4.1-macos.zip`, unzip it and follow the README. Already on 0.3.x or 0.4.0? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.4.0 (2026-10-31)

## See what's playing

The idle screen can now show the song that Spotify or Music plays: the song, the artist and a progress bar. Turn it on in the settings window under General, Music, "Show what's playing". It is off by default.

- agent-screensaver asks the Spotify and Music apps on your Mac what they play, every 3 seconds. It never starts them, and it uses no network and no login. macOS asks you once per app to allow it.
- The widget hides when nothing plays, while an agent waits for your approval, and in privacy mode.
- The demo (`./agent-screensaver demo`) shows a made-up song when the setting is on, so recordings can show the widget.

Known limits:

- Only the Spotify and Music apps. Music in a browser, and other music apps, are not shown.
- Songs that Music streams show the song and the artist, but no progress bar: Music gives no position or length for them. Songs in your own library, and Spotify, have the bar.
- No album cover, because that would need the network.
- The Default and Calm themes show it. A theme with its own layout, such as Chrome and the [album themes](https://github.com/stavgayer/agent-screensaver-themes), does not show it yet.

For extension authors: components get the song as `state.nowPlaying`. See `docs/extensions.md`.

## Install

Download `agent-screensaver-v0.4.0-macos.zip`, unzip it and follow the README. Already on 0.3.x? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.3.2 (2026-10-30)

## Drop a theme on the window to install it

You can now install a theme, or any other extension, by dropping its `.zip` file or folder on the settings window. No Terminal needed.

- While you drag a zip or a folder over the window, it shows "Drop to install".
- A drop works from any tab. The window opens Extensions and shows the result there.
- It works the same as the Install… button: theme packs install as a whole, and an upgrade that changes an agent's files still shows its review first.
- Other files, or more than one item at a time, are ignored.

The [themes](https://github.com/stavgayer/agent-screensaver-themes) READMEs now explain how to install from the settings window too.

## Install

Download `agent-screensaver-v0.3.2-macos.zip`, unzip it and follow the README. Already on 0.3.x? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.3.1 (2026-10-30)

## Find it like any other app

Install now also adds **Agent Screensaver** to your Applications folder (`~/Applications`). Search for it in Spotlight or Launchpad, or open it from Finder, to open the settings window. You no longer need the menu bar icon, which a full menu bar can hide behind the notch.

- Opening it again while the settings window is open brings that window to the front. It does not open a second one.
- Install shows the new app in its plan before you confirm. If something else already uses that name, install leaves it alone.
- Uninstall removes the app again, but only if it is the one install made.

## Install

Download `agent-screensaver-v0.3.1-macos.zip`, unzip it and follow the README. Already on 0.3.0? Run `./agent-screensaver install` from the new folder. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.3.0 (2026-10-29)

## Theme packs

A theme is often several extensions: a background, a layout and its widgets. You can now install all of them in one step.

- `agent-screensaver ext install ./my-theme-pack.zip` installs a whole pack, from a folder or a zip. You can also install a pack from the settings window.
- `agent-screensaver ext remove <theme>` removes a theme and the parts it uses. It keeps parts that another installed theme uses, and your own background. If the theme was in use, the settings go back to the default theme.
- `agent-screensaver ext remove --only <id>` removes only that one extension.

Ready-made themes: [agent-screensaver-themes](https://github.com/stavgayer/agent-screensaver-themes).

## App icon

The settings window has an icon in the Dock, a monitor with an attention dot, and the screen saver has a thumbnail in System Settings.

## A lighter settings window

The live preview in the settings window uses less CPU. It draws at 15 frames a second and pauses when the window is in the background or the preview is scrolled out of view.

## Install

Download `agent-screensaver-v0.3.0-macos.zip`, unzip it and follow the README. Universal build (Apple silicon and Intel), macOS 14 or later.

## v0.2.0 (2026-10-27)

What's new since v0.1.0

## Chrome theme, polished

- Task cards and the approval card now use the theme's own fonts and colors
- Finished tasks and warnings each get their own color
- Ring labels are readable from across a room
- The chrome shape moves out of the way of the approval card
- Smoother outline on the shape

## Safer agent upgrades

- Installing a new version of an agent shows every changed file first, as a diff, and asks before it installs anything
- Hidden or invisible characters are shown, never hidden
- Hooks of a new agent run only after you trust it

## Fixes

- Ignored projects match regardless of case
- Zips made with Finder's Compress install
- daemon.log rotates at 1 MB; the menu bar app has its own tray.log
- Settings files keep their own indent when hooks are added or removed
- Session names survive a restart (never stored in privacy mode)
- A new agent's usage ring works without a restart
- Second display verified on a real Mac

Run ./agent-screensaver install again to upgrade

## v0.1.0 (2026-10-26)

First public release.

- A macOS screen saver that shows what your coding agents are doing: Claude Code, Codex and Cursor built in.
- Cards for working, done and "needs approval", plus usage rings for Claude Code and Codex.
- Themes: Default (Matrix), Calm (Apple aerials) and Chrome (3D).
- Works offline. Adds and removes agent hooks safely, with backups.

Install: download the zip below, unzip it, and run ./agent-screensaver install. See the README for details.
Requires macOS 14 or later.
