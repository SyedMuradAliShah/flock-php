package pr_gate

import (
	"errors"
	"reflect"
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
			if tt.gateClosed {
				want := []string{"pr:get", "pr:gate-closed", "issue:get", "pr:set-state", "pr:create-comment"}
				if !reflect.DeepEqual(api.calls, want) {
					t.Fatalf("calls = %v, want %v", api.calls, want)
				}
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

func TestRunClosesWrongBaseWithCommentBeforeClosing(t *testing.T) {
	api := &fakeService{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "release", Author: github_issue.User{Login: "contributor", Type: "User"},
	}}
	if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "opened", Number: 87}); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get", "pr:create-comment", "pr:set-state"}
	if !reflect.DeepEqual(api.calls, want) {
		t.Fatalf("calls = %v, want %v", api.calls, want)
	}
}

func TestRunRetriesNewUnapprovedPullRequestsBeforeClosing(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("open", ""), files: []github_pr.File{{Filename: "internal/site.go"}}}
	if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", Event{Action: "opened", Number: 87}); err != nil {
		t.Fatal(err)
	}
	if api.getCalls != 3 {
		t.Fatalf("Get() calls = %d, want 3", api.getCalls)
	}
	want := []string{"pr:get", "pr:list-files", "pr:get", "pr:list-files", "pr:get", "pr:list-files", "pr:create-comment", "pr:set-state"}
	if !reflect.DeepEqual(api.calls, want) {
		t.Fatalf("calls = %v, want %v", api.calls, want)
	}
}

func TestRunSkipsIneligibleClosedAndEditedPullRequests(t *testing.T) {
	for _, tt := range []struct {
		name  string
		pr    *github_pr.PullRequest
		event Event
	}{
		{name: "body unchanged", pr: pullRequest("closed", "Fixes #73"), event: Event{Action: "edited", Number: 87}},
		{name: "merged", pr: &github_pr.PullRequest{Number: 87, State: "closed", Merged: true, BaseRef: "main"}, event: Event{Action: "edited", Number: 87, BodyEdited: true}},
		{name: "wrong base", pr: &github_pr.PullRequest{Number: 87, State: "closed", BaseRef: "release"}, event: Event{Action: "edited", Number: 87, BodyEdited: true}},
		{name: "edited body only", pr: pullRequest("open", ""), event: Event{Action: "edited", Number: 87, BodyEdited: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeService{pullRequest: tt.pr}
			if _, err := Execute(api, &fakeIssueService{api}, "flock-php", "flock", tt.event); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(api.calls, []string{"pr:get"}) {
				t.Fatalf("calls = %v, want only Get()", api.calls)
			}
		})
	}
}

func TestRunReturnsPullRequestServiceErrors(t *testing.T) {
	tests := []struct {
		name  string
		api   *fakeService
		event Event
	}{
		{name: "get", api: &fakeService{getErr: errors.New("get")}, event: Event{Action: "opened", Number: 87}},
		{name: "gate closed", api: &fakeService{pullRequest: pullRequest("closed", ""), gateErr: errors.New("gate")}, event: Event{Action: "edited", Number: 87, BodyEdited: true}},
		{name: "list files", api: &fakeService{pullRequest: pullRequest("open", ""), filesErr: errors.New("files")}, event: Event{Action: "synchronize", Number: 87}},
		{name: "comment", api: &fakeService{pullRequest: &github_pr.PullRequest{Number: 87, State: "open", BaseRef: "release"}, commentErr: errors.New("comment")}, event: Event{Action: "opened", Number: 87}},
		{name: "set state", api: &fakeService{pullRequest: &github_pr.PullRequest{Number: 87, State: "open", BaseRef: "release"}, stateErr: errors.New("state")}, event: Event{Action: "opened", Number: 87}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Execute(tt.api, &fakeIssueService{api: tt.api}, "flock-php", "flock", tt.event); err == nil {
				t.Fatal("Execute() returned nil error")
			}
		})
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
	getCalls    int
	calls       []string
	getErr      error
	filesErr    error
	gateErr     error
	commentErr  error
	stateErr    error
}

func (f *fakeService) Get(string, string, int) (*github_pr.PullRequest, error) {
	f.getCalls++
	f.calls = append(f.calls, "pr:get")
	return f.pullRequest, f.getErr
}
func (f *fakeService) ListFiles(string, string, int) ([]github_pr.File, error) {
	f.filesCalls++
	f.calls = append(f.calls, "pr:list-files")
	return f.files, f.filesErr
}
func (f *fakeService) GateClosed(string, string, int) (bool, error) {
	f.calls = append(f.calls, "pr:gate-closed")
	return f.gateClosed, f.gateErr
}
func (f *fakeService) CreateComment(string, string, int, string) error {
	f.calls = append(f.calls, "pr:create-comment")
	f.commented = true
	return f.commentErr
}
func (f *fakeService) SetState(_ string, _ string, _ int, state string) error {
	f.calls = append(f.calls, "pr:set-state")
	f.state = state
	return f.stateErr
}

type fakeIssueService struct{ api *fakeService }

func (f *fakeIssueService) Get(string, string, int) (*github_issue.Issue, error) {
	f.api.issueCalls++
	f.api.calls = append(f.api.calls, "issue:get")
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
