# 0001 — Deferred items (mirror of plan "Out of scope")

Confirmed deferred from [0001-shared-dev-services.md](0001-shared-dev-services.md);
none block M1–M4. Pick up only if/when the trigger fires.

- [ ] **Web-port auto-allocation across apps** — `WEB_PORT_BASE` stays a
  manual per-app override. Trigger: a third app makes manual base
  bookkeeping annoying again.
- [ ] **Multi-machine / remote dev services** — design is machine-local
  by construction (registry, sockets). Trigger: remote dev environments.
- [ ] **Backup/retention for the shared data dir** — shared cluster now
  holds every app's dev data in one place; a stray `rm -rf` of
  `$XDG_STATE_HOME/propitech-dev` loses all of it. Trigger: first time
  someone loses data they cared about (or sooner, cheaply: document a
  `pg_dumpall` cron one-liner in the PRO-123 guide).
- [ ] **Rails parallel-test DB fan-out** (`<db>_s3-0`…) — `rm`'s
  exact-name drop list doesn't cover worker-suffixed databases.
  Trigger: any consumer enables `parallelize`/parallel_tests.
- [ ] **Production/CI topology** — explicitly untouched (Medusa CI and
  Kamal own their own service definitions).
