package config

import "slices"

// Custom is a MySQL or MariaDB the user already has, run by Flock in place from the folders it is
// in instead of from a build Flock downloaded. Flock never deletes these folders.
type Custom struct {
	Service string `json:"service"` // "mysql" or "mariadb"
	// Line names it next to the versions Flock installs: "custom-10.11.2". It is also the version
	// of the service's entry while it is the active one.
	Line    string `json:"line"`
	Version string `json:"version"` // the server's own version, "10.11.2"
	BaseDir string `json:"basedir"` // the base directory: the folder that holds bin/ (mariadbd, mariadb-admin, ...)
	DataDir string `json:"datadir"` // the data directory, used as it is
}

// FindCustom looks up the custom version of service id with the line.
func (s Services) FindCustom(id, line string) (Custom, bool) {
	i := slices.IndexFunc(s.Custom, func(c Custom) bool { return c.Service == id && c.Line == line })
	if i < 0 {
		return Custom{}, false
	}
	return s.Custom[i], true
}

// AddCustom registers a custom version.
func (s *Services) AddCustom(c Custom) {
	s.Custom = append(s.Custom, c)
}

// RemoveCustom forgets the custom version of service id with the line ("" for every one of the
// service); the files stay where they are.
func (s *Services) RemoveCustom(id, line string) {
	s.Custom = slices.DeleteFunc(s.Custom, func(c Custom) bool { return c.Service == id && (line == "" || c.Line == line) })
	if len(s.Custom) == 0 {
		s.Custom = nil
	}
}
