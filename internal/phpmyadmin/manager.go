package phpmyadmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"flock/internal/binaries"
	"flock/internal/config"
)

// Errors of Manager operations; the API maps them to HTTP status codes.
var (
	ErrNotInstalled = errors.New("phpMyAdmin is not installed")
	ErrBusy         = errors.New("a phpMyAdmin job is already running")
	ErrInvalid      = errors.New("invalid request")
)

// Mutate edits config.json and applies it (the daemon's Mutate).
type Mutate func(change func(*config.Config) error) error

// Job phases.
const (
	PhaseDownloading = "downloading"
	PhaseInstalling  = "installing"
)

// Job is a running or failed install.
type Job struct {
	State     string    `json:"state"` // running | failed
	Phase     string    `json:"phase"`
	Progress  float64   `json:"progress"`
	Error     string    `json:"error"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

// Status is what the apps show (GET /v1/phpmyadmin).
type Status struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	// Folder is where the versions are installed, DefaultFolder where they go unless one is chosen.
	Folder        string `json:"folder"`
	DefaultFolder string `json:"default_folder"`
	Name          string `json:"name"` // the site's name: "db" for db.test
	DefaultName   string `json:"default_name"`
	Domain        string `json:"domain"`
	URL           string `json:"url"`
	Estimate      string `json:"estimate"`
	// Latest is the newest release phpmyadmin.net lists ("" until the first check), Update whether it is newer than Version.
	Latest     string     `json:"latest"`
	Update     bool       `json:"update"`
	CheckedAt  *time.Time `json:"checked_at"`
	CheckError string     `json:"check_error"`
	Job        *Job       `json:"job"`
}

// Manager installs, moves and removes phpMyAdmin, one job at a time.
type Manager struct {
	root   string
	mutate Mutate
	// HTTP downloads phpMyAdmin (tests replace it).
	HTTP *http.Client

	mu         sync.Mutex
	job        *jobState
	base       context.Context
	wg         sync.WaitGroup
	latest     string
	checkedAt  time.Time
	checkError string
}

// checkEvery is how often the newest release is looked up.
const checkEvery = 6 * time.Hour

// Check asks phpmyadmin.net for the newest release and remembers it.
func (m *Manager) Check(ctx context.Context) error {
	release, err := Latest(ctx, m.HTTP)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkedAt = time.Now().UTC()
	if err != nil {
		m.checkError = err.Error()
		return err
	}
	m.latest, m.checkError = release.Version, ""
	return nil
}

type jobState struct {
	Job
	cancel context.CancelFunc
}

// NewManager creates a manager for the phpMyAdmin of the Flock directory root.
func NewManager(root string, mutate Mutate) *Manager {
	return &Manager{root: root, mutate: mutate, base: context.Background()}
}

// Run keeps the newest release up to date and cancels a running job when ctx ends.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	m.base = ctx
	m.mu.Unlock()
	go func() {
		for {
			checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = m.Check(checkCtx)
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-time.After(checkEvery):
			}
		}
	}()
	<-ctx.Done()
	m.mu.Lock()
	if m.job != nil && m.job.cancel != nil {
		m.job.cancel()
	}
	m.mu.Unlock()
}

// Wait blocks until the job has finished (for tests and shutdown).
func (m *Manager) Wait() { m.wg.Wait() }

// Status returns the state without waiting on the network.
func (m *Manager) Status() Status {
	cfg, err := config.Load(m.root)
	if err != nil {
		cfg = config.Default()
	}
	s := Status{DefaultFolder: DefaultDir(m.root), Folder: DefaultDir(m.root), Estimate: "about 16 MB download",
		Name: config.PhpMyAdminName, DefaultName: config.PhpMyAdminName}
	if p := cfg.PhpMyAdmin; p != nil {
		s.Installed, s.Version, s.Folder = true, p.Version, Folder(m.root, p.Folder)
		s.Name = cfg.PhpMyAdminSiteName()
		s.Domain = cfg.Domain(config.Site{Name: s.Name})
		s.URL = "http://" + s.Domain
		if cfg.Ports.HTTP != 80 {
			s.URL += fmt.Sprintf(":%d", cfg.Ports.HTTP)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s.Latest, s.CheckError = m.latest, m.checkError
	if !m.checkedAt.IsZero() {
		at := m.checkedAt
		s.CheckedAt = &at
	}
	s.Update = s.Installed && s.Latest != "" && binaries.CompareVersions(s.Latest, s.Version) > 0
	if m.job != nil {
		j := m.job.Job
		s.Job = &j
	}
	return s
}

// Install starts installing the newest phpMyAdmin into folder ("" for the default) in the
// background. Installing again updates it, and keeps the folder when none is given. When the
// installed version is already the newest it does nothing, unless reinstall asks to replace its files.
func (m *Manager) Install(folder string, reinstall bool) error {
	cfg, err := config.Load(m.root)
	if err != nil {
		return err
	}
	custom := ""
	if cfg.PhpMyAdmin != nil {
		custom = cfg.PhpMyAdmin.Folder
	}
	if folder != "" {
		custom = filepath.Clean(folder)
	}
	target := Folder(m.root, custom)
	if cfg.PhpMyAdmin == nil || Folder(m.root, cfg.PhpMyAdmin.Folder) != target {
		if err := CheckFolder(target); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job != nil && m.job.State == "running" {
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(m.base)
	m.job = &jobState{Job: Job{State: "running", Phase: PhaseDownloading, Progress: -1, StartedAt: time.Now().UTC()}, cancel: cancel}
	j := m.job
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		m.run(ctx, j, custom, reinstall)
	}()
	return nil
}

func (m *Manager) run(ctx context.Context, j *jobState, custom string, reinstall bool) {
	set := func(phase string, progress float64) {
		m.mu.Lock()
		j.Phase, j.Progress = phase, progress
		m.mu.Unlock()
	}
	err := m.install(ctx, j, custom, reinstall, set)

	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case err != nil && ctx.Err() != nil && m.base.Err() == nil:
		m.job = nil
	case err != nil:
		j.State, j.Error = "failed", err.Error()
	default:
		m.job = nil
	}
}

func (m *Manager) install(ctx context.Context, j *jobState, custom string, reinstall bool, set func(string, float64)) error {
	release, err := Latest(ctx, m.HTTP)
	if err != nil {
		return err
	}
	m.mu.Lock()
	j.Version = release.Version
	m.latest, m.checkedAt, m.checkError = release.Version, time.Now().UTC(), ""
	m.mu.Unlock()

	folder := Folder(m.root, custom)
	if cfg, err := config.Load(m.root); err == nil && cfg.PhpMyAdmin != nil && !reinstall &&
		cfg.PhpMyAdmin.Version == release.Version && Folder(m.root, cfg.PhpMyAdmin.Folder) == folder {
		return nil
	}
	set(PhaseDownloading, 0)
	dir, err := Install(ctx, m.HTTP, folder, release, func(done, total int64) {
		p := -1.0
		if total > 0 {
			p = float64(done) / float64(total)
		}
		set(PhaseDownloading, p)
	})
	if err != nil {
		return err
	}
	set(PhaseInstalling, -1)

	var previous *config.PhpMyAdmin
	err = m.mutate(func(cfg *config.Config) error {
		previous = cfg.PhpMyAdmin
		name := ""
		if previous != nil {
			name = previous.Name
		}
		cfg.RenameReservedSite(cmpName(name))
		cfg.PhpMyAdmin = &config.PhpMyAdmin{Version: release.Version, Path: dir, Folder: custom, Name: name, PHP: release.PHP}
		return nil
	})
	if err != nil {
		return err
	}
	RemoveOlder(folder, release.Version)
	if previous != nil && Folder(m.root, previous.Folder) != folder {
		_ = Uninstall(Folder(m.root, previous.Folder))
	}
	return nil
}

// SetFolder moves the installed phpMyAdmin to folder ("" for the default).
func (m *Manager) SetFolder(folder string) error {
	m.mu.Lock()
	if m.job != nil && m.job.State == "running" {
		m.mu.Unlock()
		return ErrBusy
	}
	m.mu.Unlock()

	cfg, err := config.Load(m.root)
	if err != nil {
		return err
	}
	p := cfg.PhpMyAdmin
	if p == nil {
		return ErrNotInstalled
	}
	custom := ""
	if folder != "" {
		custom = filepath.Clean(folder)
	}
	from, to := Folder(m.root, p.Folder), Folder(m.root, custom)
	if from == to {
		return nil
	}
	if err := CheckFolder(to); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	// The site's files move with it, and Windows locks files its PHP has open: stop serving them first.
	if err := m.mutate(func(cfg *config.Config) error { cfg.PhpMyAdmin = nil; return nil }); err != nil {
		return err
	}
	dir, err := Move(from, to, p.Version)
	if err != nil {
		restore := *p
		return errors.Join(err, m.mutate(func(cfg *config.Config) error { cfg.PhpMyAdmin = &restore; return nil }))
	}
	return m.mutate(func(cfg *config.Config) error {
		cfg.PhpMyAdmin = &config.PhpMyAdmin{Version: p.Version, Path: dir, Folder: custom, Name: p.Name, PHP: p.PHP}
		return nil
	})
}

func cmpName(name string) string {
	if name == "" {
		return config.PhpMyAdminName
	}
	return name
}

// SetName serves phpMyAdmin under another name ("db" for db.test); an empty name goes back to the
// default. A name another site, proxy or alias has is refused, and nothing else is renamed.
func (m *Manager) SetName(input string) error {
	return m.mutate(func(cfg *config.Config) error {
		if cfg.PhpMyAdmin == nil {
			return ErrNotInstalled
		}
		name := cfg.NormalizePhpMyAdminName(input)
		if name == "" || name == config.PhpMyAdminName {
			name = config.PhpMyAdminName
		}
		if err := cfg.CheckPhpMyAdminName(name); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if name == config.PhpMyAdminName {
			name = ""
		}
		cfg.PhpMyAdmin.Name = name
		return nil
	})
}

// Uninstall removes phpMyAdmin and frees its name.
func (m *Manager) Uninstall() error {
	m.mu.Lock()
	if m.job != nil && m.job.State == "running" {
		m.mu.Unlock()
		return ErrBusy
	}
	m.job = nil
	m.mu.Unlock()

	cfg, err := config.Load(m.root)
	if err != nil {
		return err
	}
	if cfg.PhpMyAdmin == nil {
		return ErrNotInstalled
	}
	folder := Folder(m.root, cfg.PhpMyAdmin.Folder)
	if err := m.mutate(func(cfg *config.Config) error { cfg.PhpMyAdmin = nil; return nil }); err != nil {
		return err
	}
	return Uninstall(folder)
}

// Cancel stops a running install.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job != nil && m.job.State == "running" {
		m.job.cancel()
	}
}

// Dismiss forgets a failed job.
func (m *Manager) Dismiss() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.job != nil && m.job.State != "running" {
		m.job = nil
	}
}

// Servers are the MySQL and MariaDB services phpMyAdmin connects to.
func Servers(cfg *config.Config) []Server {
	var out []Server
	for _, id := range []string{"mysql", "mariadb"} {
		if x, ok := cfg.Services.Extra(id); ok {
			name := map[string]string{"mysql": "MySQL", "mariadb": "MariaDB"}[id]
			out = append(out, Server{Name: name, Port: x.Port})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// Sync writes the configuration of the installed phpMyAdmin, so it follows the services; a problem
// is returned for the daemon to report as a warning.
func Sync(cfg *config.Config, root string) error {
	p := cfg.PhpMyAdmin
	if p == nil {
		return nil
	}
	if _, err := os.Stat(p.Path); err != nil {
		return fmt.Errorf("phpMyAdmin: %s is gone — install it again", p.Path)
	}
	return WriteConfig(Folder(root, p.Folder), p.Version, Servers(cfg))
}
