package main

import (
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/github_pr"
)

func TestPullRequestEventWithoutChangesDoesNotPanic(t *testing.T) {
	event, err := github_pr.ParseEvent([]byte(`{"action":"opened","pull_request":{"number":87}}`))
	if err != nil {
		t.Fatal(err)
	}

	if event.BodyEdited {
		t.Fatal("BodyEdited = true, want false")
	}
	if event.BaseEdited {
		t.Fatal("BaseEdited = true, want false")
	}
}

func TestRunIssueEventRoutesActions(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantTriage   bool
		wantUnassign bool
	}{
		{
			name:       "opened issues go to triage",
			data:       `{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"}}}`,
			wantTriage: true,
		},
		{
			name:         "assigned issues lose public availability",
			data:         `{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`,
			wantUnassign: true,
		},
		{
			name: "other issue actions do nothing",
			data: `{"action":"labeled","issue":{"number":73}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &issueEventFakeService{}
			if err := runIssueEvent(api, api, "flock-php", "flock", []byte(tt.data)); err != nil {
				t.Fatal(err)
			}
			if api.triaged != tt.wantTriage {
				t.Fatalf("triaged = %t, want %t", api.triaged, tt.wantTriage)
			}
			if api.removed != tt.wantUnassign {
				t.Fatalf("removed = %t, want %t", api.removed, tt.wantUnassign)
			}
		})
	}
}

func TestRunPullRequestEventClaimsApprovedIssue(t *testing.T) {
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}}
	issues := &pullRequestIssueFake{issue: &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}}}
	if err := runPullRequestEvent(pullRequests, issues, "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	if issues.assignee != "contributor" || !issues.removed {
		t.Fatalf("assignee = %q, removed = %t", issues.assignee, issues.removed)
	}
}

func TestRunMergeGroupEvent(t *testing.T) {
	api := &mergeGroupEventFake{}
	t.Setenv("GITHUB_RUN_ID", "7")
	if err := runMergeGroupEvent(api, api, api, "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAPIURLUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "https://github.example")
	if got := apiURL(); got != "https://github.example" {
		t.Fatalf("apiURL() = %q", got)
	}
	t.Setenv("GITHUB_API_URL", "")
	if got := apiURL(); got != "https://api.github.com" {
		t.Fatalf("apiURL() = %q", got)
	}
}

type issueEventFakeService struct {
	triaged bool
	removed bool
}

func (f *issueEventFakeService) GetUserPermission(string, string, string) (string, error) {
	return "read", nil
}

func (f *issueEventFakeService) AddLabel(string, string, int, string) error {
	f.triaged = true
	return nil
}

func (f *issueEventFakeService) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	f.removed = true
	return nil
}

type pullRequestEventFake struct{ pullRequest *github_pr.PullRequest }

func (f *pullRequestEventFake) Get(string, string, int) (*github_pr.PullRequest, error) {
	return f.pullRequest, nil
}
func (f *pullRequestEventFake) ListFiles(string, string, int) ([]github_pr.File, error) {
	return nil, nil
}
func (f *pullRequestEventFake) GateClosed(string, string, int) (bool, error)    { return false, nil }
func (f *pullRequestEventFake) CreateComment(string, string, int, string) error { return nil }
func (f *pullRequestEventFake) SetState(string, string, int, string) error      { return nil }
func (f *pullRequestEventFake) ListLinkedIssues(string, string, int) ([]github_pr.LinkedIssue, error) {
	return nil, nil
}
func (f *pullRequestEventFake) TransferIssueMilestone(string, string, int, int) (bool, error) {
	return false, nil
}

type pullRequestIssueFake struct {
	issue    *github_issue.Issue
	assignee string
	removed  bool
}

func (f *pullRequestIssueFake) Get(string, string, int) (*github_issue.Issue, error) {
	return f.issue, nil
}
func (f *pullRequestIssueFake) AddAssignee(_ string, _ string, _ int, username string) error {
	f.assignee = username
	return nil
}
func (f *pullRequestIssueFake) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	f.removed = true
	return nil
}

type mergeGroupEventFake struct{}

func (mergeGroupEventFake) GetMergeQueueEnqueuer(string, string, int) (string, error) {
	return "maintainer", nil
}
func (mergeGroupEventFake) CreateComment(string, string, int, string) error { return nil }
func (mergeGroupEventFake) GetUserRole(string, string, string) (string, error) {
	return "maintain", nil
}
func (mergeGroupEventFake) CancelOtherRuns(string, string, string, int64) error { return nil }
