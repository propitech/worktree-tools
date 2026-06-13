// Package ports maps slot numbers to service ports.
//
// Each worktree owns a unique slot N; its ports are BASE + N * stride. The
// five base ports match the shell script defaults and are overridable via the
// same env vars (DB_PORT_BASE, REDIS_PORT_BASE, WEB_PORT_BASE,
// MAIL_SMTP_PORT_BASE, MAIL_UI_PORT_BASE) so a repo can shift its entire port
// range without touching the tool.
package ports

import (
	"os"
	"strconv"
)

const stride = 10

// Bases holds the slot-0 base ports. Load from the environment with Load;
// construct directly in tests.
type Bases struct {
	DB, Redis, Web, MailSMTP, MailUI int
}

// Load reads the five base-port env vars, falling back to the standard slot-0
// defaults when a var is absent or non-numeric.
func Load() Bases {
	return Bases{
		DB:       intEnv("DB_PORT_BASE", 5431),
		Redis:    intEnv("REDIS_PORT_BASE", 6379),
		Web:      intEnv("WEB_PORT_BASE", 3000),
		MailSMTP: intEnv("MAIL_SMTP_PORT_BASE", 1025),
		MailUI:   intEnv("MAIL_UI_PORT_BASE", 8025),
	}
}

// Ports holds a slot's five resolved service ports.
type Ports struct {
	DB, Redis, Web, MailSMTP, MailUI int
}

// ForSlot returns the port set for slot number n.
func (b Bases) ForSlot(n int) Ports {
	return Ports{
		DB:       b.DB + n*stride,
		Redis:    b.Redis + n*stride,
		Web:      b.Web + n*stride,
		MailSMTP: b.MailSMTP + n*stride,
		MailUI:   b.MailUI + n*stride,
	}
}

func intEnv(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
