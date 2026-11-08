package github_issue

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flock/tools/github/github_client"
)

func TestServiceAssignsUserAndRemovesLabel(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.EscapedPath()+" "+string(body))
		switch r.Method + " " + r.URL.EscapedPath() {
		case "POST /repos/flock-php/flock/issues/73/assignees":
			_, _ = io.WriteString(w, `{"number":73,"assignees":[{"login":"contributor"}]}`)
		case "DELETE /repos/flock-php/flock/issues/73/assignees":
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
	assigned, err := service.AddAssignee("flock-php", "flock", 73, "contributor")
	if err != nil || !HasAssignee(assigned, "contributor") {
		t.Fatalf("AddAssignee() = %+v, %v", assigned, err)
	}
	if err := service.RemoveAssignee("flock-php", "flock", 73, "contributor"); err != nil {
		t.Fatal(err)
	}
	issue := &Issue{Number: 73, Labels: []Label{{Name: HelpWanted}}}
	if err := service.RemoveHelpWanted("flock-php", "flock", issue); err != nil {
		t.Fatal(err)
	}

	got := strings.Join(requests, "\n")
	if !strings.Contains(got, `POST /repos/flock-php/flock/issues/73/assignees {"assignees":["contributor"]}`) ||
		!strings.Contains(got, `DELETE /repos/flock-php/flock/issues/73/assignees {"assignees":["contributor"]}`) ||
		!strings.Contains(got, "DELETE /repos/flock-php/flock/issues/73/labels/help%20wanted ") {
		t.Fatalf("requests = %s", got)
	}
}

func TestServiceRejectsAssignmentResponseWithoutRequestedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"number":73,"assignees":[]}`)
	}))
	defer server.Close()

	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(client).AddAssignee("flock-php", "flock", 73, "contributor"); err == nil {
		t.Fatal("AddAssignee() returned nil error when GitHub did not assign the requested user")
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
	issue := &Issue{Number: 73, Labels: []Label{{Name: HelpWanted}}}
	if err := New(client).RemoveHelpWanted("flock-php", "flock", issue); err != nil {
		t.Fatalf("RemoveHelpWanted() = %v, want nil", err)
	}
}

func TestServiceGetsIssueAndAddsLabel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/flock-php/flock/issues/73":
			_, _ = io.WriteString(w, `{"number":73}`)
		case "POST /repos/flock-php/flock/issues/73/labels":
			_, _ = io.WriteString(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	service := New(client)
	issue, err := service.Get("flock-php", "flock", 73)
	if err != nil || issue.Number != 73 {
		t.Fatalf("Get() = %+v, %v", issue, err)
	}
	if err := service.AddLabel("flock-php", "flock", 73, "needs triage"); err != nil {
		t.Fatal(err)
	}
}

func TestServicePropagatesIssueErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	service := New(client)
	if _, err := service.Get("flock-php", "flock", 73); err == nil {
		t.Fatal("Get() returned nil error")
	}
	if err := service.AddLabel("flock-php", "flock", 73, "needs triage"); err == nil {
		t.Fatal("AddLabel() returned nil error")
	}
	issue := &Issue{Number: 73, Labels: []Label{{Name: HelpWanted}}}
	if err := service.RemoveHelpWanted("flock-php", "flock", issue); err == nil {
		t.Fatal("RemoveHelpWanted() returned nil error")
	}
}

func TestRemoveHelpWantedSkipsIssuesWithoutTheLabel(t *testing.T) {
	service := New(nil)
	if err := service.RemoveHelpWanted("flock-php", "flock", &Issue{Number: 73}); err != nil {
		t.Fatal(err)
	}
}

func TestIssueHelpers(t *testing.T) {
	issue := &Issue{
		Assignees: []User{{Login: "contributor"}},
		Labels:    []Label{{Name: HelpWanted}, {Name: "bug"}},
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
