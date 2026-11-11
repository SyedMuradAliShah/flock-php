package catalog

import (
	"context"
	"errors"
	"flock/internal/config"
	"strings"
	"testing"
)

func TestWithRootPassword(t *testing.T) {
	env := "DB_CONNECTION=mysql\nDB_USERNAME=root\nDB_PASSWORD="
	e, u := WithRootPassword("mysql", "p@ss w/rd", env, "mysql://root@127.0.0.1:3306")
	if !strings.Contains(e, "DB_PASSWORD=p@ss w/rd") {
		t.Errorf("env: %q", e)
	}
	if u != "mysql://root:p%40ss%20w%2Frd@127.0.0.1:3306" {
		t.Errorf("url: %q", u)
	}
	if e, u := WithRootPassword("mysql", "", env, "mysql://root@127.0.0.1:3306"); e != env || u != "mysql://root@127.0.0.1:3306" {
		t.Error("an empty password leaves both as they are")
	}
	if e, _ := WithRootPassword("valkey", "x", "REDIS_PASSWORD=null", ""); e != "REDIS_PASSWORD=null" {
		t.Error("other services are untouched")
	}
}

func TestSqlEscape(t *testing.T) {
	if got := sqlEscape("a'b\\c"); got != "a\\'b\\\\c" {
		t.Errorf("sqlEscape = %q", got)
	}
}

func TestSetRootPasswordRefusesWhatCannotWork(t *testing.T) {
	m := NewManager(t.TempDir(), Kinds("windows"), nil, nil)
	for name, call := range map[string]func() error{
		"valkey":    func() error { return m.SetRootPassword(context.Background(), "valkey", "", "x") },
		"not there": func() error { return m.SetRootPassword(context.Background(), "mariadb", "", "x") },
		"newline":   func() error { return m.SetRootPassword(context.Background(), "mariadb", "", "a\nb") },
		"too long":  func() error { return m.SetRootPassword(context.Background(), "mariadb", "", strings.Repeat("x", 200)) },
	} {
		if err := call(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestEveryLineKeepsItsOwnRootPassword(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Services.SetExtra("mariadb", config.Extra{Version: "12.3.3", Port: 3306})
	cfg.Services.SetRootPassword("mariadb", "12.3", "124")
	cfg.Services.SetRootPassword("mariadb", "11.8", "0065")
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	m := NewManager(root, Kinds("windows"), nil, nil)

	for _, c := range []struct{ line, want string }{{"", "124"}, {"12.3", "124"}, {"11.8", "0065"}, {"10.11", ""}} {
		if got, err := m.RootPasswordOf("mariadb", c.line); err != nil || got != c.want {
			t.Errorf("RootPasswordOf(%q) = %q, %v; want %q", c.line, got, err, c.want)
		}
	}
	if got := RootPassword(root, "mariadb"); got != "124" {
		t.Errorf("the running line's password: %q", got)
	}

	cfg.Services.SetExtra("mariadb", config.Extra{Version: "11.8.9", Port: 3306})
	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}
	if got := RootPassword(root, "mariadb"); got != "0065" {
		t.Errorf("after switching to 11.8: %q", got)
	}
	if _, err := m.RootPasswordOf("valkey", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("valkey: %v", err)
	}
}
