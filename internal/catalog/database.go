package catalog

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"flock/internal/config"
)

// validDatabase keeps database names to what needs no quoting rules beyond backticks.
var validDatabase = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// CreateDatabase creates database name in the installed version of MySQL or MariaDB (id) on port,
// as root; an existing database is not an error.
func CreateDatabase(ctx context.Context, root, id, version string, port int, name string) error {
	if !validDatabase.MatchString(name) {
		return fmt.Errorf("%w: database names may only use letters, digits, - and _", ErrInvalid)
	}
	return ExecSQL(ctx, root, id, version, port, "CREATE DATABASE IF NOT EXISTS `"+name+"`")
}

// ExecSQL runs sql as root in the installed version of MySQL or MariaDB (id) on port. The error is
// the client's message.
func ExecSQL(ctx context.Context, root, id, version string, port int, sql string) error {
	client, err := Tool(root, id, version, "client")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, client, append(Connect(port), "-e", sql)...)
	cmd.Env = ClientEnv(root, id)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

// Connect are the client arguments that connect to the server on port as root, ignoring the user's
// option files. The password goes through ClientEnv, where other programs can't read it.
func Connect(port int) []string {
	return []string{"--no-defaults", "-uroot", "-h127.0.0.1", fmt.Sprintf("-P%d", port)}
}

// RootPassword is the root password Flock knows for the running line of the MySQL or MariaDB id;
// "" when there is none.
func RootPassword(root, id string) string {
	cfg, err := config.Load(root)
	if err != nil {
		return ""
	}
	x, ok := cfg.Services.Extra(id)
	if !ok {
		return ""
	}
	return cfg.Services.RootPassword(id, majorMinor(x.Version))
}

// ClientEnv is the environment for a MySQL or MariaDB client or admin program run as root.
func ClientEnv(root, id string) []string {
	return clientEnv(RootPassword(root, id))
}

func clientEnv(password string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(kv), "MYSQL_PWD=") {
			env = append(env, kv)
		}
	}
	return append(env, "MYSQL_PWD="+password)
}

// WithRootPassword puts the root password into the .env snippet and connection URL of a MySQL or
// MariaDB service; other services and an empty password leave them as they are.
func WithRootPassword(id, password, env, connection string) (string, string) {
	if password == "" || (id != "mysql" && id != "mariadb") {
		return env, connection
	}
	env = strings.Replace(env, "DB_PASSWORD=", "DB_PASSWORD="+password, 1)
	if u, err := url.Parse(connection); err == nil && u.User != nil {
		u.User = url.UserPassword(u.User.Username(), password)
		connection = u.String()
	}
	return env, connection
}

// Tool finds a program of the installed version of MySQL or MariaDB (id): "client" (mysql, or
// mariadb) or "dump" (mysqldump, or mariadb-dump).
func Tool(root, id, version, tool string) (string, error) {
	names := map[string][]string{"client": {"mysql"}, "dump": {"mysqldump"}}[tool]
	switch id {
	case "mysql":
	case "mariadb":
		names = map[string][]string{"client": {"mariadb", "mysql"}, "dump": {"mariadb-dump", "mysqldump"}}[tool]
	default:
		return "", fmt.Errorf("%w: %s has no databases", ErrInvalid, id)
	}
	for _, name := range names {
		path := filepath.Join(Dir(root, id, version), "bin", exe(runtime.GOOS, name))
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s isn't part of the installed %s %s", names[0], id, version)
}
