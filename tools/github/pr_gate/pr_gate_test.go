package pr_gate

import (
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/github_pr"
)

func TestRunReopensOnlyGateClosedPullRequests(t *testing.T) {
	tests := []struct {
		name       string
		gateClosed bool
		wantState  string
	}{
		{name: "reopens gate-closed pull request", gateClosed: true, wantState: "open"},
		{name: "does not reopen manually closed pull request", gateClosed: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeService{
				pullRequest: pullRequest("closed", "Fixes #73"),
				gateClosed:  tt.gateClosed,
				issue:       approvedIssue(),
			}
			result, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "edited", Number: 87, BodyEdited: true})
			if err != nil {
				t.Fatal(err)
			}
			if api.state != tt.wantState {
				t.Fatalf("state = %q, want %q", api.state, tt.wantState)
			}
			if tt.gateClosed && (result.ApprovedIssue == nil || result.ApprovedIssue.Number != 73) {
				t.Fatalf("approved issue = %+v, want flock#73", result.ApprovedIssue)
			}
		})
	}
}

func TestRunClosesUnapprovedPullRequest(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("open", "Fixes #73"), files: []github_pr.File{{Filename: "internal/site.go"}}, issue: &github_issue.Issue{State: "open"}}
	if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "synchronize", Number: 87}); err != nil {
		t.Fatal(err)
	}
	if api.state != "closed" || !api.commented {
		t.Fatalf("state = %q, commented = %t", api.state, api.commented)
	}
}

func TestRunDoesNotListFilesForApprovedIssue(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("open", "Fixes #73"), issue: approvedIssue()}
	result, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "opened", Number: 87})
	if err != nil {
		t.Fatal(err)
	}
	if result.ApprovedIssue == nil || result.ApprovedIssue.Number != 73 {
		t.Fatalf("approved issue = %+v, want flock#73", result.ApprovedIssue)
	}
	if api.filesCalls != 0 {
		t.Fatalf("Files() calls = %d, want 0", api.filesCalls)
	}
}

func TestRunAllowsDocsOnlyPullRequestsWithoutAnApprovedIssue(t *testing.T) {
	api := &fakeService{
		pullRequest: pullRequest("open", ""),
		files:       []github_pr.File{{Filename: "docs/contributing.md"}},
	}
	if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "opened", Number: 87}); err != nil {
		t.Fatal(err)
	}
	if api.filesCalls != 1 {
		t.Fatalf("Files() calls = %d, want 1", api.filesCalls)
	}
	if api.state != "" || api.commented {
		t.Fatalf("state = %q, commented = %t", api.state, api.commented)
	}
}

func TestRunBypassesMaintainersWithoutCheckingIssuesOrFiles(t *testing.T) {
	pr := pullRequest("open", "")
	pr.AuthorAssociation = "MEMBER"
	api := &fakeService{pullRequest: pr}
	if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "opened", Number: 87}); err != nil {
		t.Fatal(err)
	}
	if api.issueCalls != 0 || api.filesCalls != 0 {
		t.Fatalf("Issue() calls = %d, Files() calls = %d, want 0", api.issueCalls, api.filesCalls)
	}
}

type fakeService struct {
	pullRequest *github_pr.PullRequest
	files       []github_pr.File
	issue       *github_issue.Issue
	gateClosed  bool
	state       string
	commented   bool
	filesCalls  int
	issueCalls  int
}

func (f *fakeService) Get(string, string, int) (*github_pr.PullRequest, error) {
	return f.pullRequest, nil
}
func (f *fakeService) ListFiles(string, string, int) ([]github_pr.File, error) {
	f.filesCalls++
	return f.files, nil
}
func (f *fakeService) GateClosed(string, string, int) (bool, error) { return f.gateClosed, nil }
func (f *fakeService) CreateComment(string, string, int, string) error {
	f.commented = true
	return nil
}
func (f *fakeService) SetState(_ string, _ string, _ int, state string) error {
	f.state = state
	return nil
}

type fakeIssueService struct{ api *fakeService }

func (f *fakeIssueService) Get(string, string, int) (*github_issue.Issue, error) {
	f.api.issueCalls++
	return f.api.issue, nil
}

func pullRequest(state, body string) *github_pr.PullRequest {
	return &github_pr.PullRequest{
		Number:  87,
		State:   state,
		BaseRef: "main",
		Body:    body,
		Author:  github_issue.User{Login: "contributor", Type: "User"},
	}
}

func approvedIssue() *github_issue.Issue {
	return &github_issue.Issue{State: "open", Labels: []github_issue.Label{{Name: openToAnyone}}}
}
