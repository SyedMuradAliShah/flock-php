package github_pr

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flock/tools/github/github_client"
)

func TestServiceTransfersMilestone(t *testing.T) {
	var writes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.Method + " " + r.URL.Path {
		case "GET /repos/flock-php/flock/issues/73":
			_, _ = io.WriteString(w, `{"number":73,"state":"open","milestone":{"number":3}}`)
		case "PATCH /repos/flock-php/flock/issues/87", "PATCH /repos/flock-php/flock/issues/73":
			writes = append(writes, r.URL.Path+" "+string(body))
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
	transferred, err := New(client).TransferIssueMilestone("flock-php", "flock", 87, 73)
	if err != nil || !transferred {
		t.Fatalf("TransferIssueMilestone() = %t, %v", transferred, err)
	}
	got := strings.Join(writes, "\n")
	if !strings.Contains(got, `/issues/87 {"milestone":3}`) || !strings.Contains(got, `/issues/73 {"milestone":null}`) {
		t.Fatalf("writes = %s", got)
	}
	if strings.Contains(got, `"state"`) {
		t.Fatalf("milestone transfer changed issue state: %s", got)
	}
}

func TestServiceListsDevelopmentLinkedIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "closingIssuesReferences(first: 100)") || strings.Contains(string(body), "userLinkedOnly") {
			t.Fatalf("query does not request all Development links: %s", body)
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"closingIssuesReferences":{"nodes":[{"number":73,"milestone":{"number":3}},{"number":74,"milestone":null}]}}}}}`)
	}))
	defer server.Close()

	client, err := github_client.New(server.URL, "token")
	if err != nil {
		t.Fatal(err)
	}
	issues, err := New(client).ListLinkedIssues("flock-php", "flock", 87)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Number != 73 || issues[0].MilestoneNumber == nil || *issues[0].MilestoneNumber != 3 || issues[1].MilestoneNumber != nil {
		t.Fatalf("ListLinkedIssues() = %+v", issues)
	}
}
