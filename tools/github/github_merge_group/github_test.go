package github_merge_group

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"flock/tools/github/github_client"
)

func TestServiceCancelsOnlyOtherUnfinishedRuns(t *testing.T) {
	var canceled []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/flock-php/flock/actions/runs":
			_, _ = io.WriteString(w, `{"workflow_runs":[{"id":1,"status":"queued"},{"id":2,"status":"completed"},{"id":3,"status":"in_progress"}]}`)
		case "POST /repos/flock-php/flock/actions/runs/1/cancel":
			canceled = append(canceled, r.URL.Path)
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if err := New(client).CancelOtherRuns("flock-php", "flock", "abc", 3); err != nil {
		t.Fatal(err)
	}
	if len(canceled) != 1 || canceled[0] != "/repos/flock-php/flock/actions/runs/1/cancel" {
		t.Fatalf("canceled = %v", canceled)
	}
}

func TestServiceReturnsListAndCancelErrors(t *testing.T) {
	for _, name := range []string{"list", "cancel"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "list" || r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_, _ = io.WriteString(w, `{"workflow_runs":[{"id":1,"status":"queued"}]}`)
			}))
			defer server.Close()
			client, err := github_client.New(server.URL, "token")
			if err != nil {
				t.Fatal(err)
			}
			if err := New(client).CancelOtherRuns("flock-php", "flock", "abc", 2); err == nil {
				t.Fatal("CancelOtherRuns() returned nil error")
			}
		})
	}
}
