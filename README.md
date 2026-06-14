# worktree-tools

`worktree` — manage git worktrees with isolated dev services. Each worktree gets
a unique **slot** N (the primary checkout is slot 0). Two isolation models:

- **Shared services** *(current)* — one Postgres + one Redis + one Mailpit per
  machine, shared by every worktree of every app; worktrees isolate by
  **namespace** (a per-app+slot database suffix and a Redis DB index) instead of
  by port. Opt in per app — see [Shared dev services](#shared-dev-services).
- **Per-worktree port offsets** *(legacy)* — each worktree runs its own daemon
  set on slot-derived ports (`BASE + slot * 10`). Still the default for apps that
  haven't opted in, and byte-identical to before.

Extracted from the [Fosa](https://github.com/propitech/fosa) Rails template so
it lives in one place and is consumed by every app via [mise](https://mise.jdx.dev),
rather than vendored and hand-synced per repo.

## Install (mise)

Add to a project's `mise.toml`:

```toml
[tools]
# `latest` tracks new releases on `mise install` / `mise up`; pin a version
# (e.g. "2.0.1") instead if you want reproducible, explicit bumps.
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
bin/worktree add <slug> [<type>] [--no-start] [--prefix <ns>] [--root <dir>]   # create + boot on own ports
bin/worktree list                                   # slots, slugs, web port, per-worktree status
bin/worktree adopt [<path>] [--start]               # adopt an existing worktree
bin/worktree cd <slug|name|path|slot>               # open a subshell in a worktree
bin/worktree run <slug|name|path|slot> <cmd> [args...]   # run a command in a worktree (via mise)
bin/worktree rm <slug|name|path|slot> [--delete-branch] [--force]
bin/worktree autoadopt                              # SessionStart hook entry point
bin/worktree reprovision [<target>]                 # rewrite a worktree's .env to the current contract
bin/worktree services <start|stop|status>           # shared dev daemons (one set per machine)
bin/worktree config show                            # print the effective configuration
bin/worktree config set <key> <value>               # change a machine-global port or state dir
```

`cd`, `run`, `rm`, and `reprovision` all accept the same target forms: a slot
number (`0` is the primary checkout), the worktree directory name, a legacy
`<repo>-<slug>` slug, or a path.

```sh
bin/worktree cd pro-169            # drops you into a subshell in that worktree; `exit` returns
bin/worktree run 3 rails db:migrate   # runs in slot 3 with its .env loaded (PORT, DB suffix, …)
bin/worktree run pm-login bin/rspec   # name target; command after it
```

`cd` opens an interactive subshell because a child process cannot change its
parent shell's working directory. `run` executes the command through
`mise exec --` from the worktree's directory, so the worktree's slot `.env` is
loaded exactly as it is for `mise run start`; the command's exit code is
propagated.

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

### Legacy: per-worktree port offsets

Apps that have **not** opted into [shared services](#shared-dev-services) (no
`WORKTREE_SERVICES=shared` in their `mise.toml [env]`) keep the original model:
each worktree runs its own Postgres/Redis/Mailpit, and its slot's ports are
`BASE + slot * 10`. The five slot-0 bases default to the values below and can be
overridden per-repo via env (typically the app's `mise.toml` `[env]`), so an app
that must run beside another on the same machine can shift its whole port range
and never collide:

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

#### Opting an app in (the `.env` contract)

The per-worktree `.env` is **capability-gated**, so this is safe for apps that
haven't migrated. An app opts in by setting `WORKTREE_SERVICES=shared` and
`WORKTREE_APP=<name>` in its `mise.toml [env]`:

- **Gated app** → `add`/`adopt` write the namespace contract: fixed shared
  `DB_PORT`/`MAIL_*` from the services config, `WORKTREE_DB_SUFFIX=_s<slot>`
  (empty for the primary), and `REDIS_DB = app_base + slot` (each app gets a
  registry-allocated 16-slot band) plus a full `REDIS_URL`. Only the web `PORT`
  stays slot-derived. The **primary checkout** is provisioned too (slot 0), so
  two apps' primaries don't collide on Redis DB 0.
- **Ungated app** → behaviour is byte-identical to before (slot-offset ports).

`WORKTREE_APP` is required when gated and must name the app explicitly —
`SOCK_PREFIX` is not the app name (e.g. property_management bakes `pm` while its
databases are `property_management_*`). `worktree add` aborts if it's missing.

**`worktree reprovision [<target>]`** rewrites an existing worktree's `.env` to
the current contract in place — for a worktree whose branch carries WIP (so
`rm` + `add` isn't an option) or one written under the old contract. Defaults to
the current worktree; the primary reprovisions as slot 0.

**`worktree config show`** prints the effective configuration the tool resolves,
read-only, in four sections: **Global** (config / data / runtime directories,
branch prefix, default type), **Worktree creation** (the primary checkout, its
parent, and the `<parent>/<repo>-<slug>` path new worktrees land at), **Service
endpoints** (shared Postgres / Redis / Mailpit host:port, with a Postgres
reachability check), and **This worktree** (the current worktree's slot, app,
services contract, DB suffix, web port, Redis DB, and owned databases — or a
note when run outside a managed worktree).

**`worktree config set <key> <value>`** changes one machine-global setting.
Settable keys: the service ports `PG_PORT`, `REDIS_PORT`, `MAIL_SMTP_PORT`,
`MAIL_UI_PORT` (written to `~/.config/propitech-dev/config`, validated as a port
in 1–65535); the shared state locations `SVC_DATA_DIR`, `SVC_RUNTIME_DIR`
(written to the registry, must be an absolute path); and the per-repo
`WORKTREE_ROOT` (see below). Port/dir changes apply to the *next* service start:
if the shared daemons are already running, the command warns that a
`worktree services stop && worktree services start` is needed for the new value
to take effect — it never auto-restarts. Repointing a state dir does not move
existing data.

### Where new worktrees are created

By default `worktree add` places a worktree as a sibling of the primary
checkout: `<parent>/<repo>-<slug>`. To put a repo's worktrees elsewhere, set a
per-repo root:

```sh
worktree config set WORKTREE_ROOT /path/to/worktrees   # run inside the repo
worktree add login --root /tmp/scratch                 # one-off override
```

Resolution precedence is `--root` flag → per-repo `WORKTREE_ROOT` → the sibling
default. The root is created if absent. `WORKTREE_ROOT` is stored in the registry
keyed by the clone's git common dir — **not** the repo name — so two same-named
clones (forks) get independent roots and never collide on a shared
`<repo>-<slug>` path. Worktrees keep the `<repo>-<slug>` basename wherever they
live, so `cd` / `run` / `rm` / `list` resolve them by slug as usual.

On a shared-services app, **`worktree rm`** drops only that worktree's exact
five databases (`<app>_development[_cache|_queue|_cable]<suffix>`, `<app>_test<suffix>`)
with `DROP DATABASE … WITH (FORCE)` — no glob, so a prefix-named sibling app is
never caught — and flushes only its Redis DB index (warning first if a client is
still connected). The shared daemons are left running for every other worktree.
**`worktree list`** reports a per-worktree `STATUS`: `up`, `db-missing` (slot
database absent), `services-down`, `stale-contract` (the app migrated but this
worktree's `.env` hasn't — run `reprovision`), or `legacy:up`/`legacy:down` for
unmigrated apps; it also flags a foreign Postgres squatting the shared port.

### Claude Code auto-adopt

`worktree autoadopt` is a SessionStart hook: when a Claude Code session opens
inside a `.claude/worktrees/<name>` isolation worktree, it adopts the worktree
into a slot (services stay down). Wire it in `.claude/settings.json`:

It is **session-owned**: the Claude session id from the hook's stdin payload is
recorded on first adopt (under the shared git common dir, in
`claude-worktree-owners/<name>`). A later run by a *different* session refuses to
adopt that worktree rather than re-claiming it, so two sessions never end up
sharing — and trampling — one checkout. When no session id is available (a manual
run, or an older hook payload), it falls back to the historical behaviour of
adopting the current worktree.

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

## Migrating an app to shared services

When an app's `mise.toml` + `database.yml` are updated to opt in
(`WORKTREE_SERVICES=shared`, `WORKTREE_APP`, and the `WORKTREE_DB_SUFFIX`
interpolation in `database.yml`), migrate the developer's machine **once**.
Worktree *slot* databases are disposable — only the **primary** checkout's data
is worth moving. Order matters: the shared Postgres defaults to the same port as
the old per-repo primary (`5431`), so the old cluster must be stopped before the
shared one starts.

1. **Dump the primary's data** from the old per-repo cluster while it's still
   running (its socket is under `$XDG_RUNTIME_DIR`, named `<SOCK_PREFIX>-0`):

   ```sh
   pg_dumpall -h "$XDG_RUNTIME_DIR/<sock_prefix>-0" -p 5431 > ~/worktree-migration.sql
   ```

2. **Stop the old per-worktree daemons** (in each worktree, on the
   pre-migration revision — the migration PR deletes these tasks) and drop the
   old per-repo data dir:

   ```sh
   mise run stop
   rm -rf <repo>/.data
   ```

3. **Pull the app's migration PR**, then **start the shared services** (now that
   the old cluster has freed the port):

   ```sh
   bin/worktree services start
   ```

4. **Restore** the primary's data into the shared cluster (its socket is printed
   by `worktree services status`; default port `5431`):

   ```sh
   psql -h "$XDG_RUNTIME_DIR/propitech-dev" -p 5431 -d postgres < ~/worktree-migration.sql
   ```

5. **Reprovision in-flight worktrees** (WIP branches that can't be `rm` + `add`ed)
   and recreate their disposable slot databases:

   ```sh
   bin/worktree reprovision    # per worktree; the primary reprovisions as slot 0
   bin/setup                   # or: bin/rails db:prepare
   ```

`worktree list` flags anything left behind: a `stale-contract` worktree still
needs `reprovision`; a `foreign` Postgres means an old per-repo daemon is still
holding the shared port (stop it, or change `PG_PORT` in
`~/.config/propitech-dev/config`).

> **Backup note.** One shared cluster now holds every app's dev data, so a
> careless wipe loses everything at once. A cheap safety net is a periodic
> `pg_dumpall` of the shared data dir (`~/.local/state/propitech-dev/postgres`) —
> tracked as a follow-up (PRO-133).

## Releases

Tag `vX.Y.Z`; CI packages `worktree` into `worktree-vX.Y.Z.tar.gz` and attaches
it to a GitHub release. `ubi` installs that asset.

The shared-services model is a **behavioural change** and ships as **v2.0.0**.
It is safe for unmigrated apps: the per-worktree `.env` contract is
capability-gated (`WORKTREE_SERVICES=shared`), so an app that hasn't opted in
behaves byte-identically to v1.x even on `version = "latest"`.

## License

MIT — see [LICENSE](LICENSE).
