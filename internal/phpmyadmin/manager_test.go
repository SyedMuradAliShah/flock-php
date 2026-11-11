package phpmyadmin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flock/internal/config"
)

func testManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	mutate := func(change func(*config.Config) error) error {
		cfg, err := config.Load(root)
		if err != nil {
			return err
		}
		if err := change(cfg); err != nil {
			return err
		}
		return cfg.Save(root)
	}
	return NewManager(root, mutate), root
}

func TestManagerInstallMoveUninstall(t *testing.T) {
	server(t, "5.2.3", false)
	m, root := testManager(t)

	cfg, _ := config.Load(root)
	cfg.Upsert(config.Site{Name: "phpmyadmin", Path: "/code/mine"})
	cfg.Services.SetExtra("mariadb", config.Extra{Version: "11.8.1", Port: 3307})
	cfg.Save(root)

	if st := m.Status(); st.Installed || st.Folder != DefaultDir(root) {
		t.Fatalf("before: %+v", st)
	}

	custom := filepath.Join(t.TempDir(), "tools", "pma")
	if err := m.Install(custom, false); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	st := m.Status()
	if !st.Installed || st.Version != "5.2.3" || st.Folder != custom || st.Job != nil || st.Domain != "phpmyadmin.test" {
		t.Fatalf("after install: %+v", st)
	}

	cfg, _ = config.Load(root)
	if cfg.PhpMyAdmin.Path != filepath.Join(custom, "5.2.3") {
		t.Errorf("path %s", cfg.PhpMyAdmin.Path)
	}
	got := map[string]config.Site{}
	for _, s := range cfg.AllSites() {
		got[s.Name] = s
	}
	if got["phpmyadmin-1"].Path != "/code/mine" || got["phpmyadmin"].Path != cfg.PhpMyAdmin.Path {
		t.Errorf("the site that had the name moved to phpmyadmin-1: %+v", got)
	}

	if err := Sync(cfg, root); err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(filepath.Join(custom, "5.2.3", "config.inc.php"))
	if !strings.Contains(string(conf), "['port'] = 3307;") || !strings.Contains(string(conf), "'MariaDB'") {
		t.Errorf("config.inc.php follows the installed services:\n%s", conf)
	}

	cfg, _ = config.Load(root)
	cfg.Upsert(config.Site{Name: "shop", Path: "/code/shop"})
	cfg.Save(root)
	if err := m.SetName("shop"); err == nil {
		t.Error("a name another site uses was accepted")
	}
	if err := m.SetName("DB.test"); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Name != "db" || st.Domain != "db.test" || st.URL != "http://db.test" {
		t.Errorf("after rename: %+v", st)
	}

	target := filepath.Join(t.TempDir(), "moved")
	if err := m.SetFolder(target); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Folder != target || !st.Installed || st.Name != "db" {
		t.Errorf("after move (the name stays): %+v", st)
	}
	if _, err := os.Stat(filepath.Join(target, "5.2.3", "index.php")); err != nil {
		t.Error("files moved")
	}
	if _, err := os.Stat(custom); err == nil {
		t.Error("the old folder is gone")
	}

	other := t.TempDir()
	os.WriteFile(filepath.Join(other, "keep.txt"), nil, 0o644)
	if err := m.SetFolder(other); err == nil {
		t.Error("moving into a folder with other files was allowed")
	}
	if st := m.Status(); st.Folder != target {
		t.Errorf("a refused move changes nothing: %+v", st)
	}

	if err := m.Install("", false); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.Name != "db" || st.Folder != target || st.Latest != "5.2.3" || st.Update || st.Job != nil {
		t.Errorf("already the newest: nothing to do, and the name and folder stay: %+v", st)
	}

	// A reinstall replaces the files of the folder that is being served, even with a file added by the user.
	user := filepath.Join(target, "5.2.3", "config.user.inc.php")
	os.WriteFile(user, []byte("<?php // mine"), 0o644)
	os.WriteFile(filepath.Join(target, "5.2.3", "index.php"), []byte("broken"), 0o644)
	if err := m.Install("", true); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.Job != nil || st.Version != "5.2.3" {
		t.Fatalf("after reinstall: %+v", st)
	}
	if data, _ := os.ReadFile(filepath.Join(target, "5.2.3", "index.php")); string(data) != "<?php echo 1;" {
		t.Errorf("index.php was not replaced: %q", data)
	}
	if data, _ := os.ReadFile(user); string(data) != "<?php // mine" {
		t.Error("the user's config.user.inc.php must survive a reinstall")
	}
	if err := m.SetName(""); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Name != "phpmyadmin" {
		t.Errorf("empty name resets: %+v", st)
	}

	if err := m.Uninstall(); err != nil {
		t.Fatal(err)
	}
	if st := m.Status(); st.Installed {
		t.Error("still installed")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("uninstall removes the folder it made")
	}
	if err := m.Uninstall(); err == nil {
		t.Error("uninstalling twice should say it is not installed")
	}
}

func TestManagerFailedJobStaysUntilDismissed(t *testing.T) {
	server(t, "5.2.3", true)
	m, _ := testManager(t)
	if err := m.Install("", false); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	st := m.Status()
	if st.Installed || st.Job == nil || st.Job.State != "failed" {
		t.Fatalf("%+v", st)
	}
	m.Dismiss()
	if m.Status().Job != nil {
		t.Error("job should be gone")
	}
}

func TestUpdateFlagFollowsTheNewestRelease(t *testing.T) {
	m, root := testManager(t)
	cfg, _ := config.Load(root)
	cfg.PhpMyAdmin = &config.PhpMyAdmin{Version: "5.2.3", Path: "/x/5.2.3"}
	cfg.Save(root)

	for latest, want := range map[string]bool{"": false, "5.2.3": false, "5.2.2": false, "5.2.4": true, "6.0.0": true} {
		m.latest = latest
		if got := m.Status().Update; got != want {
			t.Errorf("latest %q: Update = %v, want %v", latest, got, want)
		}
	}
}
