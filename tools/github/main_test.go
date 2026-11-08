package main

import (
	"errors"
	"reflect"
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/github_pr"
	"flock/tools/github/merge_gate"
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

func TestPolicies(t *testing.T) {
	tests := []struct {
		name        string
		events      []string
		wantTriage  bool
		wantRemoval bool
		permission  string
	}{
		{
			name: "externally created unassigned issues keep the help wanted label",
			events: []string{
				`{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"},"labels":[{"name":"help wanted"}]}}`,
			},
			wantTriage: true,
		},
		{
			name: "externally created assigned issues remove the help wanted label",
			events: []string{
				`{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"},"labels":[{"name":"help wanted"}]}}`,
				`{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`,
			},
			wantTriage:  true,
			wantRemoval: true,
		},
		{
			name: "internally created assigned issues remove the help wanted label without triage",
			events: []string{
				`{"action":"opened","issue":{"number":73,"user":{"login":"maintainer","type":"User"},"author_association":"MEMBER","labels":[{"name":"help wanted"}]}}`,
				`{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`,
			},
			wantRemoval: true,
		},
		{
			name: "bots bypass triage",
			events: []string{
				`{"action":"opened","issue":{"number":73,"user":{"login":"bot","type":"Bot"}}}`,
			},
		},
		{
			name: "writers bypass triage",
			events: []string{
				`{"action":"opened","issue":{"number":73,"user":{"login":"writer","type":"User"}}}`,
			},
			permission: "write",
		},
		{
			name: "assigned issue without assignees keeps the help wanted label",
			events: []string{
				`{"action":"assigned","issue":{"number":73,"labels":[{"name":"help wanted"}]}}`,
			},
		},
		{
			name: "unsupported issue events do not change labels",
			events: []string{
				`{"action":"labeled","issue":{"number":73,"user":{"login":"contributor","type":"User"}}}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &issueEventFakeService{permission: tt.permission}
			for _, event := range tt.events {
				if err := runIssueEvent(api, api, "flock-php", "flock", []byte(event)); err != nil {
					t.Fatal(err)
				}
			}
			if api.triaged != tt.wantTriage {
				t.Fatalf("triaged = %t, want %t", api.triaged, tt.wantTriage)
			}
			if api.removed != tt.wantRemoval {
				t.Fatalf("removed = %t, want %t", api.removed, tt.wantRemoval)
			}
		})
	}
}

func TestRunIssueEventStopsOnParseAndServiceErrors(t *testing.T) {
	tests := []struct {
		name string
		api  issueEventFakeService
		data string
	}{
		{name: "invalid payload", data: `{`},
		{name: "permission", api: issueEventFakeService{permissionErr: errors.New("permission")}, data: `{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"}}}`},
		{name: "label", api: issueEventFakeService{labelErr: errors.New("label")}, data: `{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"}}}`},
		{name: "remove label", api: issueEventFakeService{removeErr: errors.New("remove")}, data: `{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runIssueEvent(&tt.api, &tt.api, "flock-php", "flock", []byte(tt.data)); err == nil {
				t.Fatal("runIssueEvent() returned nil error")
			}
		})
	}
}

func TestRunPullRequestEventClaimsApprovedIssue(t *testing.T) {
	calls := []string{}
	milestone := 3
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, linkedIssues: []github_pr.LinkedIssue{{Number: 73, MilestoneNumber: &milestone}}, calls: &calls}
	issues := &pullRequestIssueFake{issue: &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}}, calls: &calls}
	if err := runPullRequestEvent(pullRequests, issues, "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	if issues.assignee != "contributor" || !issues.removed {
		t.Fatalf("assignee = %q, removed = %t", issues.assignee, issues.removed)
	}
	want := []string{"pr:get", "issue:get", "issue:get", "issue:assign", "issue:remove-help-wanted", "pr:get", "pr:list-linked-issues", "pr:transfer-milestone"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunPullRequestEventSkipsCrossRepositoryAssignmentAndMilestoneTransfer(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes flock-php/binaries#73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, calls: &calls}
	issues := &pullRequestIssueFake{issue: &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}}, calls: &calls}
	if err := runPullRequestEvent(pullRequests, issues, "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get", "issue:get"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunPullRequestEventStopsOnErrors(t *testing.T) {
	tests := []struct {
		name         string
		pullRequests *pullRequestEventFake
		issues       *pullRequestIssueFake
		data         string
	}{
		{name: "invalid payload", pullRequests: &pullRequestEventFake{}, issues: &pullRequestIssueFake{}, data: `{`},
		{name: "gate", pullRequests: &pullRequestEventFake{getErr: errors.New("get")}, issues: &pullRequestIssueFake{}, data: `{"action":"opened","pull_request":{"number":87}}`},
		{name: "assignment", pullRequests: &pullRequestEventFake{pullRequest: &github_pr.PullRequest{Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73"}}, issues: &pullRequestIssueFake{issue: &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}}, assignErr: errors.New("assign")}, data: `{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runPullRequestEvent(tt.pullRequests, tt.issues, "flock-php", "flock", []byte(tt.data)); err == nil {
				t.Fatal("runPullRequestEvent() returned nil error")
			}
		})
	}
}

func TestRunMergeGroupEvent(t *testing.T) {
	api := &mergeGroupEventFake{}
	t.Setenv("GITHUB_RUN_ID", "7")
	if err := runMergeGroupEvent(api, api, api, "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRunMergeGroupEventRejectsInvalidPayloadAndUsesZeroForInvalidRunID(t *testing.T) {
	if err := runMergeGroupEvent(&mergeGroupEventFake{}, &mergeGroupEventFake{}, &mergeGroupEventFake{}, "flock-php", "flock", []byte(`{`)); err == nil {
		t.Fatal("runMergeGroupEvent() returned nil error")
	}

	validRunIDAPI := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "7")
	err := runMergeGroupEvent(validRunIDAPI, validRunIDAPI, validRunIDAPI, "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
	if !errors.Is(err, merge_gate.ErrNotAllowed) || validRunIDAPI.cancelExcept != 7 {
		t.Fatalf("err = %v, except = %d", err, validRunIDAPI.cancelExcept)
	}

	api := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "not-a-number")
	err = runMergeGroupEvent(api, api, api, "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
	if !errors.Is(err, merge_gate.ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}
	if !api.cancelled || api.cancelExcept != 0 {
		t.Fatalf("cancelled = %t, except = %d", api.cancelled, api.cancelExcept)
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
	triaged       bool
	removed       bool
	permission    string
	permissionErr error
	labelErr      error
	removeErr     error
}

func (f *issueEventFakeService) GetUserPermission(string, string, string) (string, error) {
	if f.permission == "" {
		return "read", f.permissionErr
	}
	return f.permission, f.permissionErr
}

func (f *issueEventFakeService) AddLabel(string, string, int, string) error {
	f.triaged = true
	return f.labelErr
}

func (f *issueEventFakeService) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	f.removed = true
	return f.removeErr
}

type pullRequestEventFake struct {
	pullRequest  *github_pr.PullRequest
	linkedIssues []github_pr.LinkedIssue
	calls        *[]string
	getErr       error
}

func (f *pullRequestEventFake) Get(string, string, int) (*github_pr.PullRequest, error) {
	f.record("pr:get")
	return f.pullRequest, f.getErr
}
func (f *pullRequestEventFake) ListFiles(string, string, int) ([]github_pr.File, error) {
	f.record("pr:list-files")
	return nil, nil
}
func (f *pullRequestEventFake) GateClosed(string, string, int) (bool, error) {
	f.record("pr:gate-closed")
	return false, nil
}
func (f *pullRequestEventFake) CreateComment(string, string, int, string) error {
	f.record("pr:create-comment")
	return nil
}
func (f *pullRequestEventFake) SetState(string, string, int, string) error {
	f.record("pr:set-state")
	return nil
}
func (f *pullRequestEventFake) ListLinkedIssues(string, string, int) ([]github_pr.LinkedIssue, error) {
	f.record("pr:list-linked-issues")
	return f.linkedIssues, nil
}
func (f *pullRequestEventFake) TransferIssueMilestone(string, string, int, int) (bool, error) {
	f.record("pr:transfer-milestone")
	return false, nil
}
func (f *pullRequestEventFake) record(call string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, call)
	}
}

type pullRequestIssueFake struct {
	issue     *github_issue.Issue
	assignee  string
	removed   bool
	calls     *[]string
	assignErr error
}

func (f *pullRequestIssueFake) Get(string, string, int) (*github_issue.Issue, error) {
	f.record("issue:get")
	return f.issue, nil
}
func (f *pullRequestIssueFake) AddAssignee(_ string, _ string, _ int, username string) error {
	f.record("issue:assign")
	f.assignee = username
	return f.assignErr
}
func (f *pullRequestIssueFake) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	f.record("issue:remove-help-wanted")
	f.removed = true
	return nil
}
func (f *pullRequestIssueFake) record(call string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, call)
	}
}

type mergeGroupEventFake struct {
	role         string
	cancelled    bool
	cancelExcept int64
}

func (mergeGroupEventFake) GetMergeQueueEnqueuer(string, string, int) (string, error) {
	return "maintainer", nil
}
func (mergeGroupEventFake) CreateComment(string, string, int, string) error { return nil }
func (f *mergeGroupEventFake) GetUserRole(string, string, string) (string, error) {
	if f.role == "" {
		return "maintain", nil
	}
	return f.role, nil
}
func (f *mergeGroupEventFake) CancelOtherRuns(_ string, _ string, _ string, except int64) error {
	f.cancelled = true
	f.cancelExcept = except
	return nil
}
