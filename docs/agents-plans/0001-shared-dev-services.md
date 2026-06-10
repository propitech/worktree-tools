# 0001 — Shared dev services across worktrees and apps

- **Tracking:** [PRO-119](https://linear.app/propitech/issue/PRO-119/design-shared-dev-services-architecture-plan-decision-record) · Project [Tooling · Shared dev services for worktrees](https://linear.app/propitech/project/tooling-shared-dev-services-for-worktrees-651cb1ed4558)
- **Status:** draft for review (revised after adversarial review — 3-lens panel, 2026-06-10)
- **Notion decision record:** child page of Foundations → Engineering best practices
- **PR cadence:** one PR per Linear issue (PRO-120…127), as decomposed below.

## Goal

Replace the per-worktree Postgres/Redis/Mailpit daemons with **one shared
instance of each per machine**, serving every worktree of every
Fosa-template app. Isolation moves from *port offsets* (`BASE + slot * 10`,
one daemon set and one `.data/` dir per worktree) to *namespaces* (a
database family per app+slot, a Redis DB index per app+slot, one shared
Mailpit inbox). The Rails server stays per-worktree on its slot-derived
`PORT` — it is the only thing that still needs a unique port.

What this buys: `worktree add` stops paying an `initdb` per worktree;
RAM/disk stop scaling with worktrees × apps; the five per-app
`*_PORT_BASE` overrides collapse to one (`WEB_PORT_BASE`); all dev mail
lands in a single Mailpit UI.

## Current state (facts the design rests on)

- `worktree` (this repo) allocates a slot per worktree and writes
  `.env` with five slot-offset ports (`DB_PORT`, `REDIS_PORT`, `PORT`,
  `MAIL_SMTP_PORT`, `MAIL_UI_PORT`). **The primary checkout is never
  provisioned** — it has no `.env` and rides its app's `mise.toml
  [env]` slot-0 defaults.
- Daemon lifecycle lives in each app's `mise.toml`
  (`postgres:*` / `redis:*` / `mailpit:*` tasks), templated from fosa's
  `templates/mise.toml.tt`. Data in `<worktree>/.data/`, sockets in
  `$XDG_RUNTIME_DIR/$SOCK_PREFIX-$slot` (sun_path ≤ 107 bytes).
- Apps consume services purely through env (`database.yml` reads
  `DB_HOST/DB_PORT/DB_USER/DB_PASS`; Sidekiq reads `REDIS_URL`; mailer
  reads `MAIL_SMTP_PORT`). Database **names are hardcoded** per
  environment: `<app>_development` + `_cache` + `_queue` + `_cable`,
  and `<app>_test` — worktree isolation today comes entirely from
  running separate instances.
- Redis serves **Sidekiq only** (cache/queue/cable ride Solid* on
  Postgres).
- **`SOCK_PREFIX` is not the app name everywhere**: property_management
  bakes `SOCK_PREFIX=pm` while its databases are `property_management_*`;
  dance_school has no `SOCK_PREFIX` at all. It cannot identify an app.
- **dance_school diverges from the template**: `DB_PORT` defaults to
  5433 (not 5431), Redis to 6380, its Part-A Mailpit to 1026/8026, and
  its Postgres status check probes localhost TCP. Migration steps that
  assume template defaults do not fire for DS.
- worktree-tools is consumed as `version = "latest"` — **consumers
  cannot hold back a release**; `mise up` / fresh `mise install` pulls
  whatever is newest.
- `mise x` against an untrusted config file hard-errors
  non-interactively; `redis-server --daemonize yes` exits 0 even when
  the bind fails (error goes only to the logfile); Mailpit has no unix
  socket (TCP listeners only, health today = pidfile + `kill -0`).

## Resolved decisions

### D1 — Lifecycle: on-demand idempotent start, explicit stop, one global lock

`worktree services start` is idempotent and callable from anywhere
(worktree add/adopt, an app's `mise run start`, by hand). First call
initializes data dirs and boots the three daemons; later calls are
no-ops. **No auto-stop, no refcounting, no systemd units** (not
portable across WSL2 setups / macOS). `worktree services stop` exists
for the explicit case (upgrades, machine cleanup).

**Machine-global lock.** The repo-scoped slot lock cannot serialize two
apps. All of `services start|stop` and every registry read-modify-write
take an `flock` on `~/.config/propitech-dev/lock` (mkdir fallback where
flock is missing, as the slot lock does today). This is a design
property: without it, concurrent starts from two repos race `initdb`,
the Mailpit pidfile, and — worst — first-come registry allocation could
hand two apps the same Redis range.

**Post-boot verification, per service.** Daemon exit codes are not
trusted: Postgres start is confirmed with `pg_isready` against the
shared socket; Redis with a `PING` over its unix socket **plus**
`CONFIG GET databases` (a wrong answer means a foreign Redis owns the
port); Mailpit by checking the pidfile PID's `/proc/<pid>/comm` (PID
reuse) — Mailpit is the documented carve-out from socket-based probing
since it has no unix socket. `services start` reports per-service
status and exits non-zero if any daemon failed verification.

**Version pinning + mise mechanics.** `~/.config/propitech-dev/` carries
its own `mise.toml` pinning `postgres`/`redis`/`mailpit` — one canonical
server-version set, written on first run, editable. The manager must
(a) `mise trust` that file immediately after writing it and on every
start (idempotent; covers user edits re-triggering the trust gate),
(b) invoke daemons as `mise -C ~/.config/propitech-dev x -- <cmd>`, and
(c) scrub the inherited environment (the manager itself runs under the
app's `mise exec`, so the app's `[env]` — `PG_DATA_DIR`, `SOCK_DIR`,
`REDIS_PORT`… — would otherwise leak into the shared daemons): start
daemons with a curated env, not the caller's. Apps keep
`postgres`/`redis` in their `[tools]` for client binaries.

### D2 — Manager placement: `worktree services` subcommand

The manager lives in this repo as `worktree services
<start|stop|status>`. Rationale: worktree-tools is already the one
mise-distributed, app-agnostic home (plan 0058), so every app gets the
manager for free; a separate repo would add a second distribution
pipeline; per-app mise tasks cannot be shared. Consumers'
`postgres:*`/`redis:*`/`mailpit:*` task families shrink to thin
delegations (`mise run start` depends on `worktree services start`).

### D3 — Redis isolation: DB index per app+slot, registry-allocated, primaries provisioned

`REDIS_DB = app_base + slot`, where `app_base` is allocated per app
(stride 16 → 16 slots per app) in the registry at
`~/.config/propitech-dev/registry`, under the global lock. The shared
Redis starts with `databases 1024` (64 apps' headroom, negligible
cost). The slot `.env` carries `REDIS_DB` and a full
`REDIS_URL=redis://localhost:<port>/<n>`.

**Primaries are provisioned too.** A committed `mise.toml` default
cannot encode a machine-local allocation — with two apps, both
primaries would land on Redis DB 0. So the "never provision the
primary" rule changes: `worktree add`/`adopt`/`services start` (run
from a repo) ensure the **primary's `.env`** exists with slot 0's
contract (`REDIS_DB=app_base`, `WORKTREE_DB_SUFFIX=` empty, shared
ports). `.env` is already gitignored in all consumers; `adopt` still
refuses to *re-slot* the primary.

Rejected: key prefixes (Sidekiq dropped namespace support; invisible
cross-talk on mistakes) and port-per-app Redis (re-creates the port
bookkeeping this plan removes).

### D4 — Shared locations: resolved once, recorded; ports machine-configurable

- Config + registry + lock: `~/.config/propitech-dev/` — the one
  env-independent anchor.
- Data: `${XDG_STATE_HOME:-$HOME/.local/state}/propitech-dev/{postgres,redis,logs}`
- Sockets/pids: `${XDG_RUNTIME_DIR:-/tmp}/propitech-dev` (short →
  sun_path-safe)

XDG vars are **not stable across shells** (login vs IDE/hook/cron
contexts on WSL2), so data and runtime paths are resolved once, at
first `services start`, and **recorded in the registry**; every later
invocation reads the recorded paths instead of re-deriving them — two
shells with different XDG env must not initdb two "shared" clusters.

Ports default to Postgres **5431** (today's slot-0 default — first
app's primary sees no change; deliberately ≠ 5432 so a system Postgres
never collides), Redis **6379**, Mailpit **1025/8025** — but stay
**configurable in one machine-global place** (the services config),
not per app. Killing the per-app `*_PORT_BASE` knobs must not remove
the escape hatch for squatted well-known ports (commit c686ae1 exists
because that hatch was needed; 6379 is the universal Redis default and
Docker/another distro can hold it on WSL2's shared localhost). Since
`provision()` writes resolved values into each `.env`, apps follow the
config automatically.

Probe doctrine: Postgres and Redis via their unix sockets (never
localhost TCP — WSL2 shares localhost across distros); Mailpit via
pidfile + `/proc/<pid>/comm` (no unix socket exists — documented
carve-out).

### D5 — Database naming: slot suffix interpolated, app identity explicit

Worktree `.env` carries `WORKTREE_DB_SUFFIX=_s<slot>` (empty for
slot 0). `database.yml` appends it to every name:

```yaml
database: property_management_development<%= ENV["WORKTREE_DB_SUFFIX"] %>
```

…and likewise `_cache` / `_queue` / `_cable` / `_test`. Properties:
slot-0 names are **unchanged** (primaries migrate data, not names);
cross-app uniqueness is free (base names embed the app); the test
database is per-slot too, so suites run concurrently across worktrees.

**App identity is explicit, never derived.** Each app declares
`WORKTREE_APP` in its `mise.toml [env]` (fosa bakes the real app name
at generation time; PRO-125/126 add it to PM/DS by hand — `SOCK_PREFIX`
is *not* usable: PM's is `pm` while its databases are
`property_management_*`). `provision()` and `rm` **abort** if
`WORKTREE_APP` is missing — no basename fallback for an operation that
drops databases.

Databases are created by `bin/setup` / `rails db:prepare` as today.
`provision()` ensures the developer role exists and keeps creating the
`$USER` convenience database (parity with today's `postgres:setup`, so
bare `psql` keeps working).

**`worktree rm` namespace cleanup, exact and connection-safe:**
- reads `WORKTREE_DB_SUFFIX` and `REDIS_DB` **from the worktree's own
  `.env`** — never recomputed from the registry (a rebuilt registry
  must not aim `rm` at another app's index);
- drops the **exact** five names
  (`<app>_development_s<N>`, `…_development_{cache,queue,cable}_s<N>`,
  `<app>_test_s<N>`) — no glob: `pm_*` patterns could cross app
  boundaries when one app name prefixes another;
- `DROP DATABASE … WITH (FORCE)` (live connections from a forgotten
  console must not strand the drop);
- before `FLUSHDB`, checks `CLIENT LIST` for connections on the target
  index and warns — a zombie Sidekiq would repopulate the index that
  the next worktree on this reused slot inherits.

### D6 — Rollout: capability-gated contract, reprovision for in-flight worktrees

**The new contract is opt-in per app** — this is the load-bearing
rollout decision. Consumers ride `version = "latest"`, so the new
script reaches unmigrated apps the day it ships. `provision()` writes
the shared-services `.env` **only when the app advertises support** via
`WORKTREE_SERVICES=shared` in its `mise.toml [env]` (set by the same
PR that migrates the app's `database.yml` and tasks — PRO-124 for the
template, PRO-125/126 for PM/DS). Without the marker, `provision()`
keeps writing today's legacy slot-offset contract unchanged. No
flag-day, no broken `worktree add` in unmigrated repos, no silent
sharing of an unsuffixed development database.

**In-flight worktrees:** `worktree rm` + `add` does not work for a
branch with WIP (`add` always passes `-b`; the branch already exists),
and a stale `.env` would override migrated `[env]` defaults via mise's
`_.file` precedence with ports nothing listens on. So PRO-121 ships
`worktree reprovision [<target>]`: rewrite an existing worktree's
`.env` to the current contract in place. `worktree list` flags
stale-contract worktrees (`.env` has `REDIS_PORT` / lacks
`WORKTREE_DB_SUFFIX`).

**Primary data migration** (the guide is PRO-123's deliverable, gated
on these points):
1. Stop the old per-app daemons **before** pulling the app's migration
   PR (it deletes the `postgres:stop`/… tasks; alternatively the app
   keeps `legacy:stop` aliases for one release). DS note: its old
   daemons sit on 5433/6380/1026 — no port conflict will force the
   issue; they must be stopped explicitly or they linger.
2. `pg_dumpall` through the old instance's socket → restore into the
   shared instance → delete `<repo>/.data/`.
3. Worktree slot databases are disposable: reprovision + `bin/setup`
   (no initdb — cheap).

**Foreign-instance detection:** the port-conflict guard can only fire
inside `services start` (Postgres refuses via data-dir/`postmaster.pid`
mismatch and bind failure; Redis/Mailpit via post-boot verification,
D1). Rails itself does not go through the guard — an old primary
Postgres still listening on 5431 would happily accept the new
worktrees' connections and strand their databases in the repo-local
cluster. `services status` and `worktree list` therefore detect and
name a foreign listener on the configured PG port (e.g. a
`postmaster.pid` inside some repo's `.data/postgres`) and print the
exact stop command.

### D7 — PRO-107 Part B: re-scope to target shared services

**Do not land Part B as-is.** It would port the per-worktree daemon
model (slot-offset ports, per-worktree data dirs, ~11K
`docs/worktrees.md`) into dance_school only for PRO-126 to rewrite it
weeks later. Part B pauses now; PRO-126 takes DS straight to the
shared model once M2 ships. Part A (Mailpit env/tasks, PRO-93 wiring)
stays — its 1026/8026 ports don't collide with the shared Mailpit, so
it keeps working until PRO-126 consolidates it (DS mail lands in its
own UI until then). M2 is three small shell PRs — the wait is short.

### D8 — Slot semantics: stay per-repo

Slots remain repo-local (allocated under the repo's git common dir
lock, as today). No machine-global slot registry: cross-app collisions
are prevented by namespaces that **explicitly embed the app**
(`WORKTREE_APP` in DB names, registry-allocated `app_base` for Redis),
and the only true machine-global resource — the web port — keeps the
existing per-app `WEB_PORT_BASE` override. The other four
`*_PORT_BASE` knobs move to the machine-global services config (D4).

## New `.env` contract (written by `provision()`, gated by D6)

```sh
WORKTREE_SLOT=3
WORKTREE_DB_SUFFIX=_s3
PORT=3030                                  # WEB_PORT_BASE + slot * 10 (unchanged)
DB_HOST=localhost
DB_PORT=5431                               # from machine-global services config
REDIS_DB=19                                # app_base 16 + slot 3
REDIS_URL=redis://localhost:6379/19
MAIL_SMTP_PORT=1025                        # shared
MAIL_UI_PORT=8025
```

Primary checkouts get the same file with `WORKTREE_SLOT=0`,
`WORKTREE_DB_SUFFIX=` (empty), `REDIS_DB=<app_base>` (D3).

## Deliverables / sequencing

Tracked in Linear (project above); one PR each, in order:

1. **PRO-120** — `worktree services start|stop|status`: global lock,
   trusted pinned-version config dir, env-scrubbed daemon start,
   post-boot verification, recorded canonical paths (D1/D4).
2. **PRO-121** — `provision()` namespace contract behind the
   `WORKTREE_SERVICES=shared` gate; primary `.env` provisioning;
   `worktree reprovision` (D3/D5/D6).
3. **PRO-122** — `rm` exact-name FORCE drops + Redis cleanup; `list`
   shared-instance probe, stale-contract + foreign-listener detection
   (D5/D6).
4. **PRO-123** — release + README rewrite + migration guide (D6 steps).
5. **PRO-124** — fosa: `mise.toml.tt` (drop daemon tasks, `[env]` with
   `WORKTREE_APP`, `WORKTREE_SERVICES=shared`, `WORKTREE_DB_SUFFIX=`
   default), `database.yml.tt` suffix interpolation.
6. **PRO-125 / PRO-126** — PM / DS migration: same env markers +
   `database.yml` + task slimming + `docs/worktrees.md` rewrite +
   primary data migration. DS reconciles PRO-107 (D7).
7. **PRO-127** — `rails-stack:worktree` skill rewrite.

The capability gate (D6) makes the order safe even though consumers
track `latest`: PRO-120…123 are inert for apps without the marker.

## Out of scope

- Production topology, CI services (Medusa), Kamal — untouched.
- Web-port auto-allocation across apps (`WEB_PORT_BASE` stays manual).
- Multi-machine / remote dev services.
- Backup/retention policy for the shared data dir.
- Rails parallel-test database fan-out (`<db>_s3-0`…): no consumer uses
  it today; if adopted, `rm`'s exact-name list needs the worker-suffix
  family added.

## Open questions

None — all eight design questions from PRO-119 are resolved above
(D1–D8). An adversarial review panel (correctness / operational /
rollout lenses) ran against the first draft; its two blockers
(machine-global locking, `latest`-rollout gating) and the primary-/
`SOCK_PREFIX`-identity gaps are folded into D1, D3, D5 and D6. If
human review overturns a decision, record the change here and in the
Notion decision record.
