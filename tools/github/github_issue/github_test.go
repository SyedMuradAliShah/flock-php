package github_issue

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flock/tools/github/github_client"
	"github.com/google/go-github/v93/github"
)

func TestServiceAssignsUserAndRemovesLabel(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.EscapedPath()+" "+string(body))
		switch r.Method + " " + r.URL.EscapedPath() {
		case "POST /repos/flock-php/flock/issues/73/assignees":
			_, _ = io.WriteString(w, `{}`)
		case "DELETE /repos/flock-php/flock/issues/73/labels/help%20wanted":
			_, _ = io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	service := New(client)
	if err := service.AddAssignee("flock-php", "flock", 73, "contributor"); err != nil {
		t.Fatal(err)
	}
	issue := &github.Issue{Number: new(73), Labels: []*github.Label{{Name: HelpWanted}}}
	if err := service.RemoveHelpWanted("flock-php", "flock", issue); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(requests, "\n")
	if !strings.Contains(got, `POST /repos/flock-php/flock/issues/73/assignees {"assignees":["contributor"]}`) ||
		!strings.Contains(got, "DELETE /repos/flock-php/flock/issues/73/labels/help%20wanted ") {
		t.Fatalf("requests = %s", got)
	}
}

func TestServiceIgnoresMissingHelpWantedLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.EscapedPath() != "/repos/flock-php/flock/issues/73/labels/help%20wanted" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Label does not exist"}`)
	}))
	defer server.Close()

	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	issue := &github.Issue{Number: new(73), Labels: []*github.Label{{Name: HelpWanted}}}
	if err := New(client).RemoveHelpWanted("flock-php", "flock", issue); err != nil {
		t.Fatalf("RemoveHelpWanted() = %v, want nil", err)
	}
}

func TestIssueHelpers(t *testing.T) {
	issue := &github.Issue{
		Assignees: []*github.User{{Login: new("contributor")}},
		Labels:    []*github.Label{{Name: HelpWanted}, {Name: "bug"}},
	}
	if !HasHelpWanted(issue) {
		t.Fatal("HasHelpWanted() = false")
	}
	if !HasLabel(issue, "bug") || HasLabel(issue, "feature") {
		t.Fatal("HasLabel() returned an unexpected result")
	}
	if !HasAssignee(issue, "contributor") || HasAssignee(issue, "someone-else") {
		t.Fatal("HasAssignee() returned an unexpected result")
	}
}
