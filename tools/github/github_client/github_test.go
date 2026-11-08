package github_client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewConfiguresAuthenticatedClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if client.BaseURL() != server.URL+"/" {
		t.Fatalf("BaseURL = %q", client.BaseURL())
	}
}

func TestNewRejectsInvalidURL(t *testing.T) {
	if _, err := New("://", "token"); err == nil {
		t.Fatal("New() returned nil error")
	}
}
