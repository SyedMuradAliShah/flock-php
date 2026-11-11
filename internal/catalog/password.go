package catalog

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"flock/internal/config"
)

// maxRootPassword keeps a password to what every client and URL handles.
const maxRootPassword = 128

// SetRootPassword changes the password of every root account of the running MySQL or MariaDB id to
// password ("" for none). current is the password in use now, "" for the one Flock knows. Flock keeps
// the new one so it can still stop the server and create databases.
func (m *Manager) SetRootPassword(ctx context.Context, id, current, password string) error {
	if id != "mysql" && id != "mariadb" {
		return fmt.Errorf("%w: only MySQL and MariaDB have a root password", ErrInvalid)
	}
	if len(password) > maxRootPassword || strings.ContainsAny(password, "\x00\r\n") {
		return fmt.Errorf("%w: a password may have up to %d characters, on one line", ErrInvalid, maxRootPassword)
	}
	cfg, err := config.Load(m.root)
	if err != nil {
		return err
	}
	x, ok := cfg.Services.Extra(id)
	if !ok || !installed(m.root, id, x.Version) {
		return fmt.Errorf("%w: %s is not installed", ErrInvalid, id)
	}
	line := majorMinor(x.Version)
	if current == "" {
		current = cfg.Services.RootPassword(id, line)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	hosts, err := m.runSQL(ctx, id, x, current, "SELECT Host FROM mysql.user WHERE User='root'", true)
	if err != nil {
		return err
	}
	var script strings.Builder
	for _, host := range strings.Fields(hosts) {
		fmt.Fprintf(&script, "ALTER USER 'root'@'%s' IDENTIFIED BY '%s';\n", sqlEscape(host), sqlEscape(password))
	}
	if script.Len() == 0 {
		return fmt.Errorf("%s has no root account", id)
	}
	script.WriteString("FLUSH PRIVILEGES;\n")
	if _, err := m.runSQL(ctx, id, x, current, script.String(), false); err != nil {
		return err
	}
	return m.mutate(func(cfg *config.Config) error {
		cfg.Services.SetRootPassword(id, line, password)
		return nil
	})
}

// RootPasswordOf is the root password Flock has for a line of MySQL or MariaDB (id), "" for none or when
// line is "" and nothing runs: for the apps to copy it. Every line has its own.
func (m *Manager) RootPasswordOf(id, line string) (string, error) {
	if id != "mysql" && id != "mariadb" {
		return "", fmt.Errorf("%w: only MySQL and MariaDB have a root password", ErrInvalid)
	}
	cfg, err := config.Load(m.root)
	if err != nil {
		return "", err
	}
	if line == "" {
		x, ok := cfg.Services.Extra(id)
		if !ok {
			return "", nil
		}
		line = majorMinor(x.Version)
	}
	return cfg.Services.RootPassword(id, line), nil
}

// runSQL runs sql as root with password; the statements go in through stdin, so a password never
// shows in a process list.
func (m *Manager) runSQL(ctx context.Context, id string, x config.Extra, password, sql string, rows bool) (string, error) {
	client, err := Tool(m.root, id, x.Version, "client")
	if err != nil {
		return "", err
	}
	args := Connect(x.Port)
	if rows {
		args = append(args, "-N", "-B")
	}
	cmd := exec.CommandContext(ctx, client, args...)
	cmd.Env = clientEnv(password)
	cmd.Stdin = strings.NewReader(sql)
	// The client warns on stderr about a login without a password; only stdout is data.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	out := stdout.Bytes()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		switch {
		case strings.Contains(msg, "Access denied"):
			return "", fmt.Errorf("%w: the root password isn't the one Flock has — enter the current password", ErrInvalid)
		case strings.Contains(msg, "Can't connect") || strings.Contains(msg, "Connection refused"):
			return "", fmt.Errorf("%w: %s isn't running — start it first", ErrInvalid, id)
		case msg != "":
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return string(out), nil
}

// sqlEscape escapes s for a single-quoted SQL string.
func sqlEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}
