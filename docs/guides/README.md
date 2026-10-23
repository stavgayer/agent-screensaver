# How-to guides for extension authors

Short, step by step guides. Each one ends with a working result. Everything in them was run: each sample in [examples/](examples/) installs with the real `agent-screensaver ext install`, and its picture or output was checked.

Read [extensions.md](../extensions.md) for the reference of the manifest and the web API. Read [agents.md](../agents.md) for the reference of agent extensions.

## Before you start

- A Mac with agent-screensaver installed, so that `agent-screensaver` works. See the main README.
- An editor. For widgets and backgrounds, basic JavaScript. You do not need a bundler or Node.
- For the dev preview and the screenshot harness: a checkout of this repository, Node and the Xcode command line tools ([test-and-share.md](test-and-share.md)).

## The guides

| I want to | Guide | Who it is for | Sample |
|---|---|---|---|
| Connect a new coding agent, such as opencode or pi | [add-an-agent.md](add-an-agent.md) | People who know how their tool runs hooks or plugins. No code needed, unless you use a converter. | [acme.quill](examples/acme.quill), [acme.tern](examples/acme.tern) |
| Draw a new piece of the screen | [add-a-widget.md](add-a-widget.md) | People who write JavaScript and CSS. | [acme.agent-count](examples/acme.agent-count) |
| Change what is behind the clock and cards | [add-a-background.md](add-a-background.md) | People who write JavaScript, or who have a picture or a video. | [acme.drift](examples/acme.drift), [acme.still](examples/acme.still) |
| Move things around on the screen | [change-the-layout.md](change-the-layout.md) | Anyone who can edit JSON. | [acme.compact](examples/acme.compact) |
| Make one choice that sets look, layout and colors | [create-a-theme.md](create-a-theme.md) | Anyone who can edit JSON. | [acme.dusk](examples/acme.dusk) |
| Try it, package it, share it | [test-and-share.md](test-and-share.md) | Everyone, last. | |

## Samples

Every folder in [examples/](examples/) is a complete extension. Copy it, change the `id` (the folder name must be the `id`), and install it. [examples/payloads/](examples/payloads/) holds recorded-style hook payloads for the two agent samples. The tools Quill and Tern are made up for the samples.

The samples work together: install them all and run `agent-screensaver theme set acme.dusk` to see a theme that uses the other samples.

## Rules for all extensions

- The id is `author.name`: lowercase letters, digits and dashes, with at least one dot. `builtin.` is reserved.
- `"api": 1`.
- A web extension is one classic script. No `import`, no `export`.
- No network. The page blocks every connection.
- Privacy mode clears card titles, requests, names and branches before your code sees them.
- An extension can never allow or deny an action of an agent.
