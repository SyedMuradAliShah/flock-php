package config

import (
	"os"
	"path/filepath"
	"testing"
)

func names(cfg *Config) map[string]Site {
	out := map[string]Site{}
	for _, s := range cfg.AllSites() {
		out[s.Name] = s
	}
	return out
}

func TestPhpMyAdminSiteAndReservedName(t *testing.T) {
	parked := t.TempDir()
	for _, d := range []string{"phpmyadmin", "phpmyadmin-1", "blog"} {
		if err := os.Mkdir(filepath.Join(parked, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Default()
	cfg.Parked = []string{parked}

	got := names(cfg)
	if got["phpmyadmin"].Path != filepath.Join(parked, "phpmyadmin") {
		t.Errorf("without phpMyAdmin the folder keeps its name: %+v", got["phpmyadmin"])
	}

	cfg.PhpMyAdmin = &PhpMyAdmin{Version: "5.2.3", Path: "/tools/phpmyadmin/5.2.3"}
	got = names(cfg)
	if got["phpmyadmin"].Path != "/tools/phpmyadmin/5.2.3" || got["phpmyadmin"].Parked {
		t.Errorf("phpmyadmin is phpMyAdmin's: %+v", got["phpmyadmin"])
	}
	if got["phpmyadmin-1"].Path != filepath.Join(parked, "phpmyadmin-1") {
		t.Errorf("a folder really named phpmyadmin-1 keeps its name: %+v", got["phpmyadmin-1"])
	}
	if got["phpmyadmin-2"].Path != filepath.Join(parked, "phpmyadmin") || !got["phpmyadmin-2"].Parked {
		t.Errorf("the displaced folder takes the next free name: %+v", got["phpmyadmin-2"])
	}
}

func TestLinkRenamesReservedName(t *testing.T) {
	cfg := Default()
	cfg.PhpMyAdmin = &PhpMyAdmin{Version: "5.2.3", Path: "/tools/phpmyadmin/5.2.3"}
	if err := cfg.Upsert(Site{Name: "phpmyadmin", Path: "/code/x"}); err == nil {
		t.Error("Upsert took phpMyAdmin's name")
	}
	if _, err := cfg.Link(Site{Name: "phpmyadmin-1", Path: "/code/a"}); err != nil {
		t.Fatal(err)
	}
	site, err := cfg.Link(Site{Name: "phpmyadmin", Path: "/code/b"})
	if err != nil || site.Name != "phpmyadmin-2" {
		t.Errorf("Link = %+v, %v; want phpmyadmin-2", site, err)
	}
	if other, _ := cfg.Link(Site{Name: "shop", Path: "/code/shop"}); other.Name != "shop" {
		t.Errorf("other names are untouched: %q", other.Name)
	}
}

func TestPhpMyAdminCustomName(t *testing.T) {
	parked := t.TempDir()
	for _, d := range []string{"db", "shop"} {
		os.Mkdir(filepath.Join(parked, d), 0o755)
	}
	cfg := Default()
	cfg.Parked = []string{parked}
	cfg.PhpMyAdmin = &PhpMyAdmin{Version: "5.2.3", Path: "/tools/pma/5.2.3"}
	cfg.Upsert(Site{Name: "blog", Path: "/code/blog", Domains: []string{"stats.test"}})
	cfg.UpsertProxy(Proxy{Name: "api", Upstream: "http://127.0.0.1:3000"})

	for name, wantErr := range map[string]bool{"database": false, "Db_1": true, "db": true, "blog": true, "stats": true, "api": true, "phpmyadmin": false} {
		err := cfg.CheckPhpMyAdminName(name)
		if (err != nil) != wantErr {
			t.Errorf("CheckPhpMyAdminName(%q) = %v, want error: %v", name, err, wantErr)
		}
	}
	if got := cfg.NormalizePhpMyAdminName(" DB.test. "); got != "db" {
		t.Errorf("Normalize = %q", got)
	}

	cfg.PhpMyAdmin.Name = "database"
	got := names(cfg)
	if got["database"].Path != "/tools/pma/5.2.3" {
		t.Errorf("served as database: %+v", got["database"])
	}
	if _, ok := got["phpmyadmin"]; ok {
		t.Error("the default name is free again")
	}
	if err := cfg.CheckPhpMyAdminName("database"); err != nil {
		t.Errorf("keeping its own name: %v", err)
	}

	cfg.Parked = append(cfg.Parked, t.TempDir())
	os.Mkdir(filepath.Join(cfg.Parked[1], "database"), 0o755)
	got = names(cfg)
	if got["database-1"].Parked != true || got["database"].Path != "/tools/pma/5.2.3" {
		t.Errorf("a folder wanting the custom name is displaced: %+v", got)
	}
}

func TestRenameReservedSiteOnInstall(t *testing.T) {
	cfg := Default()
	cfg.Upsert(Site{Name: "phpmyadmin", Path: "/code/old"})
	if got := cfg.RenameReservedSite("phpmyadmin"); got != "phpmyadmin-1" {
		t.Errorf("RenameReservedSite = %q", got)
	}
	cfg.PhpMyAdmin = &PhpMyAdmin{Version: "5.2.3", Path: "/tools/phpmyadmin/5.2.3"}
	got := names(cfg)
	if got["phpmyadmin-1"].Path != "/code/old" || got["phpmyadmin"].Path != "/tools/phpmyadmin/5.2.3" {
		t.Errorf("sites: %+v", got)
	}
}

func TestRootPasswordsAreKeptPerLine(t *testing.T) {
	var s Services
	s.SetRootPassword("mariadb", "12.3", "124")
	s.SetRootPassword("mariadb", "11.8", "0065")
	s.SetRootPassword("mysql", "8.4", "x")
	if s.RootPassword("mariadb", "12.3") != "124" || s.RootPassword("mariadb", "11.8") != "0065" || s.RootPassword("mariadb", "10.11") != "" {
		t.Errorf("%v", s.RootPasswords)
	}
	s.SetRootPassword("mariadb", "11.8", "")
	if s.RootPassword("mariadb", "11.8") != "" || s.RootPassword("mariadb", "12.3") != "124" {
		t.Errorf("clearing one line leaves the others: %v", s.RootPasswords)
	}
	s.ForgetRootPasswords("mariadb", "12.3")
	if s.RootPassword("mariadb", "12.3") != "" || s.RootPassword("mysql", "8.4") != "x" {
		t.Errorf("forgetting a line: %v", s.RootPasswords)
	}
	s.SetRootPassword("mariadb", "11.8", "a")
	s.ForgetRootPasswords("mariadb", "")
	if s.RootPassword("mariadb", "11.8") != "" || s.RootPassword("mysql", "8.4") != "x" {
		t.Errorf("forgetting a service: %v", s.RootPasswords)
	}
	s.ForgetRootPasswords("mysql", "")
	if s.RootPasswords != nil {
		t.Errorf("an empty map should be dropped from config.json: %v", s.RootPasswords)
	}
}
