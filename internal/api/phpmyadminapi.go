package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"flock/internal/phpmyadmin"
)

// PhpMyAdmin is the state of the phpMyAdmin Flock installs.
type PhpMyAdmin = phpmyadmin.Status

// PhpMyAdminJob is a running or failed phpMyAdmin install.
type PhpMyAdminJob = phpmyadmin.Job

func registerPhpMyAdmin(mux *http.ServeMux, b Backend) {
	manager := func(w http.ResponseWriter) *phpmyadmin.Manager {
		m := b.PhpMyAdmin()
		if m == nil {
			writeError(w, http.StatusNotImplemented, errors.New("phpMyAdmin is not supported here"))
		}
		return m
	}
	respond := func(w http.ResponseWriter, m *phpmyadmin.Manager, err error) {
		if err != nil {
			writeError(w, phpMyAdminStatus(err), err)
			return
		}
		writeJSON(w, http.StatusOK, m.Status())
	}

	mux.HandleFunc("GET /v1/phpmyadmin", func(w http.ResponseWriter, _ *http.Request) {
		if m := manager(w); m != nil {
			respond(w, m, nil)
		}
	})
	// Body (optional): {"folder": "D:\\Tools\\phpmyadmin"}; without one the default (or the current) folder is used.
	mux.HandleFunc("POST /v1/phpmyadmin/install", func(w http.ResponseWriter, r *http.Request) {
		m := manager(w)
		if m == nil {
			return
		}
		var body struct {
			Folder    string `json:"folder"`
			Reinstall bool   `json:"reinstall"`
		}
		if r.ContentLength != 0 {
			if err := decodeBody(r, &body); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}
		respond(w, m, m.Install(body.Folder, body.Reinstall))
	})
	mux.HandleFunc("POST /v1/phpmyadmin/check", func(w http.ResponseWriter, r *http.Request) {
		m := manager(w)
		if m == nil {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
		defer cancel()
		_ = m.Check(ctx) // a failed check is reported in check_error
		respond(w, m, nil)
	})
	// Body: {"folder": "..."}; an empty folder moves it back to the default one.
	mux.HandleFunc("PUT /v1/phpmyadmin/folder", func(w http.ResponseWriter, r *http.Request) {
		m := manager(w)
		if m == nil {
			return
		}
		var body struct {
			Folder string `json:"folder"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		respond(w, m, m.SetFolder(body.Folder))
	})
	// Body: {"name": "db"} serves it at db.<tld>; an empty name goes back to phpmyadmin.
	mux.HandleFunc("PUT /v1/phpmyadmin/name", func(w http.ResponseWriter, r *http.Request) {
		m := manager(w)
		if m == nil {
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if err := decodeBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		respond(w, m, m.SetName(body.Name))
	})
	mux.HandleFunc("POST /v1/phpmyadmin/cancel", func(w http.ResponseWriter, _ *http.Request) {
		if m := manager(w); m != nil {
			m.Cancel()
			respond(w, m, nil)
		}
	})
	mux.HandleFunc("DELETE /v1/phpmyadmin/job", func(w http.ResponseWriter, _ *http.Request) {
		if m := manager(w); m != nil {
			m.Dismiss()
			respond(w, m, nil)
		}
	})
	mux.HandleFunc("DELETE /v1/phpmyadmin", func(w http.ResponseWriter, _ *http.Request) {
		if m := manager(w); m != nil {
			respond(w, m, m.Uninstall())
		}
	})
}

func phpMyAdminStatus(err error) int {
	switch {
	case errors.Is(err, phpmyadmin.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, phpmyadmin.ErrNotInstalled):
		return http.StatusNotFound
	case errors.Is(err, phpmyadmin.ErrBusy):
		return http.StatusConflict
	}
	return statusFor(err)
}

// PhpMyAdmin fetches the state of phpMyAdmin.
func (c *Client) PhpMyAdmin(ctx context.Context) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.get(ctx, "/v1/phpmyadmin", &out)
	return out, err
}

// InstallPhpMyAdmin starts installing (or updating) phpMyAdmin into folder ("" for the default or current one);
// reinstall replaces the files of the installed version too.
func (c *Client) InstallPhpMyAdmin(ctx context.Context, folder string, reinstall bool) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.send(ctx, http.MethodPost, "/v1/phpmyadmin/install", map[string]any{"folder": folder, "reinstall": reinstall}, &out)
	return out, err
}

// CheckPhpMyAdmin looks up the newest phpMyAdmin release now.
func (c *Client) CheckPhpMyAdmin(ctx context.Context) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.slow().do(ctx, http.MethodPost, "/v1/phpmyadmin/check", &out)
	return out, err
}

// SetPhpMyAdminFolder moves the installed phpMyAdmin to folder ("" for the default one).
func (c *Client) SetPhpMyAdminFolder(ctx context.Context, folder string) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.slow().send(ctx, http.MethodPut, "/v1/phpmyadmin/folder", map[string]any{"folder": folder}, &out)
	return out, err
}

// SetPhpMyAdminName serves phpMyAdmin under another name ("db" for db.test); "" goes back to phpmyadmin.
func (c *Client) SetPhpMyAdminName(ctx context.Context, name string) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.slow().send(ctx, http.MethodPut, "/v1/phpmyadmin/name", map[string]any{"name": name}, &out)
	return out, err
}

// UninstallPhpMyAdmin removes phpMyAdmin.
func (c *Client) UninstallPhpMyAdmin(ctx context.Context) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.slow().do(ctx, http.MethodDelete, "/v1/phpmyadmin", &out)
	return out, err
}

// CancelPhpMyAdminJob stops a running install.
func (c *Client) CancelPhpMyAdminJob(ctx context.Context) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.do(ctx, http.MethodPost, "/v1/phpmyadmin/cancel", &out)
	return out, err
}

// DismissPhpMyAdminJob forgets a failed install.
func (c *Client) DismissPhpMyAdminJob(ctx context.Context) (PhpMyAdmin, error) {
	var out PhpMyAdmin
	err := c.do(ctx, http.MethodDelete, "/v1/phpmyadmin/job", &out)
	return out, err
}
