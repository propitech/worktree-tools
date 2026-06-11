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
bin/worktree services <start|stop|status>           # shared dev daemons (one set per machine)
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

### Port bases

Each slot's ports are `BASE + slot * 10`. The five slot-0 bases default to the
values below and can be overridden per-repo via env (typically the app's
`mise.toml` `[env]`), so an app that must run beside another on the same
machine can shift its whole port range and never collide:

```sh
DB_PORT_BASE         # default 5431  (Postgres)
REDIS_PORT_BASE      # default 6379  (Redis)
WEB_PORT_BASE        # default 3000  (Rails / web)
MAIL_SMTP_PORT_BASE  # default 1025  (Mailpit SMTP)
MAIL_UI_PORT_BASE    # default 8025  (Mailpit UI)
```

For example, a second app that sets `DB_PORT_BASE=5433` (and the rest) gets
slots 5433 / 5443 / 5453 …, never touching the first app's 5431 / 5441 / 5451.

### Shared dev services

`worktree services` runs **one** Postgres, **one** Redis, and **one** Mailpit
per machine, shared by every worktree of every app — the first step of the
shared-services model ([plan 0001](https://linear.app/propitech/document/plan-0001-shared-dev-services-across-worktrees-and-apps-52c0dea97522);
isolation moves from per-worktree port offsets to per-app+slot namespaces).

```sh
bin/worktree services start    # idempotent: init data dirs + boot the three daemons
bin/worktree services status   # per-service health; names a foreign listener on a shared port
bin/worktree services stop     # explicit only — no auto-stop, no refcounting
```

- **Idempotent + machine-global.** `start` is safe to call from anywhere
  (concurrently, from any app repo); a `flock` on `~/.config/propitech-dev/lock`
  serializes boot and registry writes so two repos converge on one daemon set.
- **Post-boot verification.** Daemon exit codes are not trusted — Postgres is
  confirmed with `pg_isready` on its socket, Redis with a socket `PING` plus a
  TCP `CONFIG GET databases` (a `--daemonize` Redis exits 0 even when its bind
  fails), Mailpit by its pidfile + process name. `start` exits non-zero if any
  service fails to come up.
- **Config.** Server versions are pinned in `~/.config/propitech-dev/mise.toml`;
  ports (defaults `5431 / 6379 / 1025 / 8025`) live in
  `~/.config/propitech-dev/config` — the one place to shift a squatted port.
  Data lives under `$XDG_STATE_HOME/propitech-dev/`, sockets under
  `$XDG_RUNTIME_DIR/propitech-dev`; the resolved paths are recorded in the
  registry so every shell agrees on one cluster.

The per-worktree `.env`/namespace contract that consumes these shared services
is rolled out separately (gated, opt-in per app) and does not change existing
worktree behaviour until an app adopts it.

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
