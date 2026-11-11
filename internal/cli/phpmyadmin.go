package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"flock/internal/api"
)

func newPhpMyAdminCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phpmyadmin",
		Short: "Install, move, rename and remove phpMyAdmin, served at phpmyadmin.test",
		Long: `Installs phpMyAdmin from phpmyadmin.net and serves it as the site "phpmyadmin" (change it with
"name"). It connects to Flock's MySQL and MariaDB; log in as root without a password. While it is
installed its name is its own: a folder of that name is served as phpmyadmin-1.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			st, err := s.client().PhpMyAdmin(cmd.Context())
			if err != nil {
				return err
			}
			printPhpMyAdmin(cmd, st)
			return nil
		},
	}

	var folder string
	var reinstall bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Download and install the newest phpMyAdmin (also updates it)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			if folder != "" {
				if folder, err = filepath.Abs(folder); err != nil {
					return err
				}
			}
			client := s.client()
			st, err := client.InstallPhpMyAdmin(cmd.Context(), folder, reinstall)
			if err != nil {
				return err
			}
			p := newProgress(cmd.ErrOrStderr())
			defer p.clear()
			for st.Job != nil {
				if st.Job.State == "failed" {
					p.clear()
					msg := st.Job.Error
					_, _ = client.DismissPhpMyAdminJob(cmd.Context())
					return fmt.Errorf("installing phpMyAdmin failed: %s", msg)
				}
				p.set(phpMyAdminJobText(st.Job))
				select {
				case <-cmd.Context().Done():
					cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_, _ = client.CancelPhpMyAdminJob(cancelCtx)
					return errors.New("canceled")
				case <-time.After(time.Second):
				}
				if st, err = client.PhpMyAdmin(cmd.Context()); err != nil && cmd.Context().Err() == nil {
					return err
				}
			}
			p.clear()
			printPhpMyAdmin(cmd, st)
			return nil
		},
	}
	install.Flags().StringVar(&folder, "folder", "", "install into this folder instead of Flock's data directory (it must be empty or new)")
	install.Flags().BoolVar(&reinstall, "reinstall", false, "replace the files of the installed version even when it is the newest")

	move := &cobra.Command{
		Use:   "folder [path]",
		Short: "Move phpMyAdmin to another folder (without a path: back to Flock's data directory)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			path := ""
			if len(args) > 0 {
				if path, err = filepath.Abs(args[0]); err != nil {
					return err
				}
			}
			st, err := s.client().SetPhpMyAdminFolder(cmd.Context(), path)
			if err != nil {
				return err
			}
			printPhpMyAdmin(cmd, st)
			return nil
		},
	}

	rename := &cobra.Command{
		Use:   "name [name]",
		Short: "Serve phpMyAdmin at another address, e.g. `flock phpmyadmin name db` for db.test (without a name: back to phpmyadmin.test)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			st, err := s.client().SetPhpMyAdminName(cmd.Context(), name)
			if err != nil {
				return err
			}
			printPhpMyAdmin(cmd, st)
			return nil
		},
	}

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove phpMyAdmin",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := load()
			if err != nil {
				return err
			}
			if _, err := s.client().UninstallPhpMyAdmin(cmd.Context()); err != nil {
				return err
			}
			cmd.Println("phpMyAdmin removed")
			return nil
		},
	}

	cmd.AddCommand(install, move, rename, uninstall)
	return cmd
}

func printPhpMyAdmin(cmd *cobra.Command, st api.PhpMyAdmin) {
	if !st.Installed {
		cmd.Printf("phpMyAdmin is not installed (%s) — install it with `flock phpmyadmin install`\n", st.Estimate)
		return
	}
	cmd.Printf("phpMyAdmin %s at %s, in %s\n", st.Version, st.URL, st.Folder)
	if st.Update {
		cmd.Printf("phpMyAdmin %s is available: `flock phpmyadmin install` updates it\n", st.Latest)
	}
}

func phpMyAdminJobText(j *api.PhpMyAdminJob) string {
	target := "phpMyAdmin"
	if j.Version != "" {
		target += " " + j.Version
	}
	if j.Phase == "downloading" {
		if j.Progress >= 0 {
			return fmt.Sprintf("downloading %s (%d%%)…", target, int(j.Progress*100))
		}
		return fmt.Sprintf("downloading %s…", target)
	}
	return fmt.Sprintf("installing %s…", target)
}
