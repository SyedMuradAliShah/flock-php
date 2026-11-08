package github_repository

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"flock/tools/github/github_client"
)

func TestServiceGetsUserPermissionAndRole(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/flock-php/flock/collaborators/contributor/permission" {
			t.Errorf("path = %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"permission":"write","role_name":"maintain"}`)
	}))
	defer server.Close()
	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	service := New(client)
	permission, err := service.GetUserPermission("flock-php", "flock", "contributor")
	if err != nil || permission != "write" {
		t.Fatalf("GetUserPermission() = %q, %v", permission, err)
	}
	role, err := service.GetUserRole("flock-php", "flock", "contributor")
	if err != nil || role != "maintain" {
		t.Fatalf("GetUserRole() = %q, %v", role, err)
	}
}

func TestServiceReturnsGitHubErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	service := New(client)
	if _, err := service.GetUserPermission("flock-php", "flock", "contributor"); err == nil {
		t.Fatal("GetUserPermission() returned nil error")
	}
	if _, err := service.GetUserRole("flock-php", "flock", "contributor"); err == nil {
		t.Fatal("GetUserRole() returned nil error")
	}
}
