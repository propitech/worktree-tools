# worktree-tools

`worktree` — manage git worktrees with isolated dev services. Each worktree
gets a unique **slot** that drives a non-colliding port set (Postgres, Redis,
web, Mailpit), so several worktrees run their own stack at once.

Extracted from the [Fosa](https://github.com/propitech/fosa) Rails template so
it lives in one place and is consumed by every app via [mise](https://mise.jdx.dev),
rather than vendored and hand-synced per repo.

## Install (mise)

Add to a project's `mise.toml`:

```toml
[tools]
# `latest` tracks new releases on `mise install` / `mise up`; pin a version
# (e.g. "1.0.0") instead if you want reproducible, explicit bumps.
"github:propitech/worktree-tools" = { version = "latest", exe = "worktree" }
```

`mise install` shims `worktree` onto PATH. Projects keep a thin
`bin/worktree` binstub so the familiar path still works:

```sh
#!/usr/bin/env sh
exec mise exec -- worktree "$@"
```

## Usage

```sh
bin/worktree add <slug> [<type>] [--no-start] [--prefix <ns>]   # create + boot on own ports
bin/worktree list                                   # slots, slugs, ports, PG health
bin/worktree adopt [<path>] [--start]               # adopt an existing worktree
bin/worktree rm <slug|name|path|slot> [--delete-branch] [--force]
bin/worktree autoadopt                              # SessionStart hook entry point
```

### Branch namespace

`add` names the new branch `<ns>/<type>/<slug>`. The namespace `<ns>` is
**empty by default** (no namespace segment), so `worktree add login` creates
`feat/login`. Set a namespace per-invocation with `--prefix`, or project-wide
via the `WORKTREE_BRANCH_PREFIX` env var (e.g. in `mise.toml` `[env]`):

```sh
bin/worktree add login              # feat/login      (default, no namespace)
bin/worktree add login --prefix ai  # ai/feat/login
WORKTREE_BRANCH_PREFIX=ai bin/worktree add login   # ai/feat/login
```

Precedence: `--prefix` wins, then `WORKTREE_BRANCH_PREFIX`, then empty. AI
agents working in this repo are expected to set the namespace explicitly.

### Claude Code auto-adopt

`worktree autoadopt` is a SessionStart hook: when a Claude Code session opens
inside a `.claude/worktrees/<name>` isolation worktree, it adopts the worktree
into a slot (services stay down). Wire it in `.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear",
        "hooks": [{ "type": "command", "command": "bin/worktree autoadopt" }]
      }
    ]
  }
}
```

There is deliberately no auto-remove counterpart — removal stays explicit
(`bin/worktree rm <name>`).

## Releases

Tag `vX.Y.Z`; CI packages `worktree` into `worktree-vX.Y.Z.tar.gz` and attaches
it to a GitHub release. `ubi` installs that asset.

## License

MIT — see [LICENSE](LICENSE).
