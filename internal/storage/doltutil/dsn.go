package doltutil

import (
	"fmt"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

// DefaultMaxAllowedPacket is the packet ceiling every Dolt DSN built in this
// repo advertises to go-sql-driver/mysql. It matches the driver's own default
// (64 MiB) and must be set explicitly wherever a mysql.Config is built as a
// struct literal: the zero value is 0, and 0 does NOT mean "driver default" to
// the driver — it means "ask the server", so every new connection pays a
// `SELECT @@max_allowed_packet` round-trip before the first real statement.
// bd opens at least one connection per invocation, so leaving it at 0 is a
// query per command for a value that is constant for the life of a server.
const DefaultMaxAllowedPacket = 64 << 20

// ServerDSN holds connection parameters for building a MySQL DSN to a Dolt server.
// All DSNs built with this struct set parseTime=true and multiStatements=true.
type ServerDSN struct {
	Socket   string // Unix domain socket path; when set, Net="unix" and Host/Port are ignored
	Host     string
	Port     int
	User     string
	Password string        //nolint:gosec // G117: MySQL DSN password field; required by the connection-string builder, not serialized as JSON
	Database string        // optional; empty connects without selecting a database
	Timeout  time.Duration // connect timeout; 0 defaults to 5s
	TLS      bool
}

// String builds the MySQL DSN string. Always sets parseTime=true,
// multiStatements=true, allowNativePasswords=true, and a connect timeout.
func (d ServerDSN) String() string {
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	net := "tcp"
	addr := fmt.Sprintf("%s:%d", d.Host, d.Port)
	if d.Socket != "" {
		net = "unix"
		addr = d.Socket
	}

	cfg := mysql.Config{
		User:            d.User,
		Passwd:          d.Password,
		Net:             net,
		Addr:            addr,
		DBName:          d.Database,
		ParseTime:       true,
		MultiStatements: true,
		// Explicit, not left at the zero value: MaxAllowedPacket=0 makes the
		// driver issue `SELECT @@max_allowed_packet` on every new connection
		// (connector.go, the cfg.MaxAllowedPacket > 0 branch). See
		// DefaultMaxAllowedPacket.
		MaxAllowedPacket: DefaultMaxAllowedPacket,
		// InterpolateParams renders bound parameters into the SQL client-side, so
		// a parameterized query is a single round-trip instead of a server-side
		// PREPARE + EXECUTE pair. Over a high-latency connection (e.g. a remote
		// TLS-fronted Dolt server), a write that issues many parameterized
		// statements — such as creating an issue and its labels, events, and
		// dependencies inside one transaction — otherwise pays a full round-trip
		// per statement for the prepare alone. The driver falls back to a
		// server-side prepare (driver.ErrSkip) whenever it cannot safely
		// interpolate an argument, so results never change; this only removes the
		// extra round-trip when interpolation is safe. Independent of
		// MultiStatements. The driver rejects it only with custom unsafe
		// collations, which this DSN never sets.
		InterpolateParams:    true,
		Timeout:              timeout,
		AllowNativePasswords: true,
	}
	if d.TLS {
		cfg.TLSConfig = "true"
	} else {
		// go-sql-driver/mysql v1.8+ defaults to tls=preferred when TLSConfig
		// is empty. Dolt servers without TLS reject preferred-mode negotiation
		// with "TLS requested but server does not support TLS". Explicitly
		// disable TLS so connections work against non-TLS Dolt instances.
		cfg.TLSConfig = "false"
	}

	return cfg.FormatDSN()
}
