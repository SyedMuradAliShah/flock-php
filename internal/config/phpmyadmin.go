package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// PhpMyAdminName is the site phpMyAdmin is served as, <name>.<tld>, unless the user renames it. While
// phpMyAdmin is installed the name belongs to it: other folders asking for it are served as name-1, name-2, ...
const PhpMyAdminName = "phpmyadmin"

// PhpMyAdmin is the phpMyAdmin Flock installed (see package phpmyadmin).
type PhpMyAdmin struct {
	Version string `json:"version"`
	Path    string `json:"path"`   // the installed version's folder, the site's directory
	Folder  string `json:"folder"` // the folder holding the versions, the one the user can change
	// Name is the site's name when the user chose one ("db" for db.test); empty means PhpMyAdminName.
	Name string `json:"name,omitempty"`
	// PHP is the PHP versions this phpMyAdmin supports (">=7.2,<8.4"), so its site can run on one of them.
	PHP string `json:"php,omitempty"`
}

// PhpMyAdminSiteName is the name phpMyAdmin is served under.
func (c *Config) PhpMyAdminSiteName() string {
	if c.PhpMyAdmin != nil && c.PhpMyAdmin.Name != "" {
		return c.PhpMyAdmin.Name
	}
	return PhpMyAdminName
}

// reserved reports whether name is the one phpMyAdmin is served under.
func (c *Config) reserved(name string) bool {
	return c.PhpMyAdmin != nil && name == c.PhpMyAdminSiteName()
}

// unusedName is the first of base-1, base-2, ... that no site, proxy or entry of byName uses.
func (c *Config) unusedName(base string, byName map[string]Site) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("%s-%d", base, n)
		if _, taken := byName[name]; taken {
			continue
		}
		if slices.ContainsFunc(c.Sites, func(s Site) bool { return s.Name == name }) {
			continue
		}
		if _, proxied := c.FindProxy(name); proxied {
			continue
		}
		return name
	}
}

// LinkName is the name a folder asking for name is linked as: name itself, or name-1 (-2, ...)
// when phpMyAdmin is installed and has the name.
func (c *Config) LinkName(name string) string {
	if !c.reserved(name) {
		return name
	}
	return c.unusedName(name, map[string]Site{})
}

// Link adds or replaces a linked site like Upsert, under LinkName of its name. It returns the site
// as stored.
func (c *Config) Link(site Site) (Site, error) {
	site.Name = c.LinkName(site.Name)
	return site, c.Upsert(site)
}

// RenameReservedSite moves a linked site that has name to a free one, and reports the new name
// ("" when there was nothing to move). Install calls it before phpMyAdmin takes the name.
func (c *Config) RenameReservedSite(name string) string {
	for i, s := range c.Sites {
		if s.Name == name {
			c.Sites[i].Name = c.unusedName(name, map[string]Site{})
			return c.Sites[i].Name
		}
	}
	return ""
}

// NormalizePhpMyAdminName turns what the user typed ("DB", "db.test", " Db.test. ") into a site name.
func (c *Config) NormalizePhpMyAdminName(input string) string {
	name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(input)), ".")
	return strings.TrimSuffix(name, "."+c.TLD)
}

// CheckPhpMyAdminName says why name can't be phpMyAdmin's: it isn't a valid host name, or a site,
// proxy or alias already has it. A site is never renamed to make room: the user picks another name.
func (c *Config) CheckPhpMyAdminName(name string) error {
	if !hostLabel.MatchString(name) {
		return fmt.Errorf("%q is not a valid name: use a-z, 0-9 and inner hyphens, like db", name)
	}
	domain := name + "." + c.TLD
	own := Site{}
	if c.PhpMyAdmin != nil {
		own = c.phpMyAdminSite()
	}
	for _, s := range c.AllSites() {
		if own.Path != "" && s.Name == own.Name && s.Path == own.Path {
			continue
		}
		if s.Name == name {
			return fmt.Errorf("%s is already used by the site %s — choose another name", domain, s.Name)
		}
		for _, e := range c.ExtraDomains(s) {
			if e.Host == domain {
				return fmt.Errorf("%s is already an alias of the site %s — choose another name", domain, s.Name)
			}
		}
	}
	if _, ok := c.FindProxy(name); ok {
		return fmt.Errorf("%s is already a proxy — choose another name", domain)
	}
	return nil
}

// phpMyAdminSite is the site phpMyAdmin is served as.
func (c *Config) phpMyAdminSite() Site {
	return Site{Name: c.PhpMyAdminSiteName(), Path: c.PhpMyAdmin.Path}
}

// addPhpMyAdmin puts phpMyAdmin's site and the parked folders that wanted its name (under free
// ones) into byName.
func (c *Config) addPhpMyAdmin(byName map[string]Site, displaced []Site) {
	if c.PhpMyAdmin == nil {
		return
	}
	name := c.PhpMyAdminSiteName()
	sort.Slice(displaced, func(i, j int) bool { return displaced[i].Path < displaced[j].Path })
	for _, s := range displaced {
		s.Name = c.unusedName(name, byName)
		byName[s.Name] = s
	}
	byName[name] = c.phpMyAdminSite()
}
