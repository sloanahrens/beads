package util

import (
	"strings"
	"testing"

	mysql "github.com/go-sql-driver/mysql"

	"github.com/steveyegge/beads/internal/storage/doltutil"
)

func TestDoltServerDSN_TLS(t *testing.T) {
	t.Run("config name takes precedence", func(t *testing.T) {
		dsn := DoltServerDSN{Host: "127.0.0.1", Port: 3306, User: "root", TLSConfigName: "beads-external-abc", TLSRequired: true}.String()
		if !strings.Contains(dsn, "tls=beads-external-abc") {
			t.Fatalf("dsn %q missing tls=beads-external-abc", dsn)
		}
	})

	t.Run("required without name emits tls=true", func(t *testing.T) {
		dsn := DoltServerDSN{Host: "127.0.0.1", Port: 3306, User: "root", TLSRequired: true}.String()
		if !strings.Contains(dsn, "tls=true") {
			t.Fatalf("dsn %q missing tls=true", dsn)
		}
	})

	t.Run("default disables tls", func(t *testing.T) {
		dsn := DoltServerDSN{Host: "127.0.0.1", Port: 3306, User: "root"}.String()
		if !strings.Contains(dsn, "tls=false") {
			t.Fatalf("dsn %q missing tls=false", dsn)
		}
	})

	t.Run("socket uses unix network", func(t *testing.T) {
		dsn := DoltServerDSN{Socket: "/var/run/dolt.sock", User: "root"}.String()
		if !strings.Contains(dsn, "unix(/var/run/dolt.sock)") {
			t.Fatalf("dsn %q missing unix socket", dsn)
		}
	})
}

func TestDoltServerDSN_SetsMaxAllowedPacket(t *testing.T) {
	// MaxAllowedPacket=0 makes go-sql-driver/mysql issue
	// `SELECT @@max_allowed_packet` on every new connection; the parsed DSN
	// must carry a positive ceiling instead (see
	// doltutil.DefaultMaxAllowedPacket). FormatDSN elides a value equal to the
	// driver default, so assert on the parsed config, not the string.
	dsn := DoltServerDSN{Host: "127.0.0.1", Port: 3306, User: "root"}.String()
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN(%q): %v", dsn, err)
	}
	if cfg.MaxAllowedPacket != doltutil.DefaultMaxAllowedPacket {
		t.Fatalf("MaxAllowedPacket = %d, want %d; dsn %q", cfg.MaxAllowedPacket, doltutil.DefaultMaxAllowedPacket, dsn)
	}
}
