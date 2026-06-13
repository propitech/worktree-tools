package ports

import "testing"

func TestForSlot(t *testing.T) {
	t.Parallel()
	b := Bases{DB: 5431, Redis: 6379, Web: 3000, MailSMTP: 1025, MailUI: 8025}
	cases := []struct {
		slot int
		want Ports
	}{
		{0, Ports{5431, 6379, 3000, 1025, 8025}},
		{1, Ports{5441, 6389, 3010, 1035, 8035}},
		{3, Ports{5461, 6409, 3030, 1055, 8055}},
	}
	for _, c := range cases {
		if got := b.ForSlot(c.slot); got != c.want {
			t.Errorf("ForSlot(%d) = %+v, want %+v", c.slot, got, c.want)
		}
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("DB_PORT_BASE", "5500")
	t.Setenv("WEB_PORT_BASE", "notanumber") // bad value → default
	b := Load()
	if b.DB != 5500 {
		t.Errorf("DB override = %d, want 5500", b.DB)
	}
	if b.Web != 3000 {
		t.Errorf("Web bad env = %d, want default 3000", b.Web)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	// Verify the defaults match the shell script constants (spec contract).
	b := Load()
	if b.DB != 5431 {
		t.Errorf("DB default = %d, want 5431", b.DB)
	}
	if b.Redis != 6379 {
		t.Errorf("Redis default = %d, want 6379", b.Redis)
	}
	if b.Web != 3000 {
		t.Errorf("Web default = %d, want 3000", b.Web)
	}
	if b.MailSMTP != 1025 {
		t.Errorf("MailSMTP default = %d, want 1025", b.MailSMTP)
	}
	if b.MailUI != 8025 {
		t.Errorf("MailUI default = %d, want 8025", b.MailUI)
	}
}
