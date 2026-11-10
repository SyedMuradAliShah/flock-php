package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/github_pr"
	"flock/tools/github/issue_reference"
	"flock/tools/github/issue_triage"
	"flock/tools/github/merge_gate"
	"flock/tools/github/pr_assignment"
	"flock/tools/github/pr_gate"
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
			useEventSnapshot := api.currentIssue == nil
			for _, event := range tt.events {
				if useEventSnapshot {
					parsed, err := github_issue.ParseEvent([]byte(event))
					if err != nil {
						t.Fatal(err)
					}
					api.currentIssue = &parsed.Issue
				}
				if err := runIssueEvent(newIssueEventDependencies(api), "flock-php", "flock", []byte(event)); err != nil {
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
		{name: "remove label", api: issueEventFakeService{currentIssue: &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "contributor"}}, Labels: []github_issue.Label{{Name: "help wanted"}}}, removeErr: errors.New("remove")}, data: `{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := runIssueEvent(newIssueEventDependencies(&tt.api), "flock-php", "flock", []byte(tt.data)); err == nil {
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
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	if issues.assignee != "contributor" || !issues.claimed || !issues.removed {
		t.Fatalf("assignee = %q, claimed = %t, removed = %t", issues.assignee, issues.claimed, issues.removed)
	}
	want := []string{"pr:get", "issue:get", "pr:lowest-reference", "issue:ensure-claimed-label", "issue:add-label", "issue:get", "issue:assign", "repository:role", "issue:add-label", "issue:remove-help-wanted", "pr:get", "pr:list-linked-issues", "pr:transfer-milestone"}
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
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get", "pr:list-files"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunPullRequestEventClosesIssueClaimedByAnotherContributor(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, calls: &calls}
	issues := &pullRequestIssueFake{issue: &github_issue.Issue{
		Number: 73, State: "open", Assignees: []github_issue.User{{Login: "other-contributor"}}, Labels: []github_issue.Label{{Name: "help wanted"}},
	}, calls: &calls}
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get", "issue:get", "issue:get", "repository:role", "pr:get", "pr:create-comment", "pr:set-state"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if !strings.Contains(pullRequests.comment, "Issue flock#73 has already been claimed by @other-contributor") ||
		!strings.Contains(pullRequests.comment, "other issues labeled `help wanted`") {
		t.Fatalf("comment = %q", pullRequests.comment)
	}
}

func TestRunPullRequestEventKeepsClaimedPullRequestOpen(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Author: github_issue.User{Login: "contributor", Type: "User"},
		Labels: []github_issue.Label{{Name: github_issue.Claimed}},
	}, calls: &calls}
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, &pullRequestIssueFake{calls: &calls}), "flock-php", "flock", []byte(`{"action":"synchronize","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunPullRequestEventClosesAssignedIssueForAnotherAuthor(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, calls: &calls}
	issues := &pullRequestIssueFake{
		issue: &github_issue.Issue{Number: 73, State: "open", Assignees: []github_issue.User{{Login: "other"}}, Labels: []github_issue.Label{{Name: "help wanted"}}},
		calls: &calls,
	}
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr:get", "issue:get", "issue:get", "repository:role", "pr:get", "pr:create-comment", "pr:set-state"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestRunPullRequestEventReevaluatesWhenIssueApprovalIsWithdrawn(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "closed", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, gateClosed: true, files: []github_pr.File{{Filename: "tools/github/main.go"}}, calls: &calls}
	issues := &pullRequestIssueFake{
		issue: &github_issue.Issue{Number: 74, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
		issueReads: []*github_issue.Issue{
			{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: "help wanted"}}},
		},
		calls: &calls,
	}
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"edited","changes":{"body":{}},"pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pullRequests.comment, "Flock takes pull requests only for issues a maintainer has approved") {
		t.Fatalf("re-evaluation comment = %q", pullRequests.comment)
	}
	want := []string{"pr:get", "pr:gate-closed", "issue:get", "pr:lowest-reference", "issue:ensure-claimed-label", "issue:add-label", "pr:set-state", "pr:create-comment", "issue:get", "pr:get", "issue:get", "pr:list-files", "pr:create-comment", "pr:set-state"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if issues.assignee != "" || issues.removed {
		t.Fatalf("withdrawn approval had claim side effects: assignee = %q, removed = %t", issues.assignee, issues.removed)
	}
}

func TestRunPullRequestEventProcessesReplacementApprovedIssue(t *testing.T) {
	calls := []string{}
	milestone := 4
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73\nFixes #74", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, linkedIssues: []github_pr.LinkedIssue{{Number: 74, MilestoneNumber: &milestone}}, calls: &calls}
	issues := &pullRequestIssueFake{
		issue: &github_issue.Issue{Number: 74, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
		issueReads: []*github_issue.Issue{
			{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 74, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
			{Number: 74, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}},
		},
		calls: &calls,
	}
	if err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"pr:get", "issue:get", "pr:lowest-reference", "issue:ensure-claimed-label", "issue:add-label", "issue:get", "pr:get", "issue:get", "issue:get", "pr:lowest-reference",
		"issue:ensure-claimed-label", "issue:add-label", "issue:get", "issue:assign", "repository:role", "issue:add-label", "issue:remove-help-wanted", "pr:get", "pr:list-linked-issues", "pr:transfer-milestone",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	if issues.assignedIssue != 74 || pullRequests.transferredIssue != 74 {
		t.Fatalf("assigned issue = %d, transferred issue = %d; want 74 for both", issues.assignedIssue, pullRequests.transferredIssue)
	}
}

func TestRunPullRequestEventBoundsApprovalRetries(t *testing.T) {
	calls := []string{}
	pullRequests := &pullRequestEventFake{pullRequest: &github_pr.PullRequest{
		Number: 87, State: "open", BaseRef: "main", Body: "Fixes #73", Author: github_issue.User{Login: "contributor", Type: "User"},
	}, calls: &calls}
	approved := &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: "help wanted"}}}
	withdrawn := &github_issue.Issue{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: "help wanted"}}}
	issues := &pullRequestIssueFake{
		issue: approved,
		issueReads: []*github_issue.Issue{
			approved, withdrawn, approved, withdrawn, approved, withdrawn, approved, withdrawn,
		},
		calls: &calls,
	}
	err := runPullRequestEvent(newPullRequestEventDependencies(pullRequests, issues), "flock-php", "flock", []byte(`{"action":"opened","pull_request":{"number":87,"user":{"login":"contributor"}}}`))
	if err == nil || !strings.Contains(err.Error(), "issue approval changed repeatedly") {
		t.Fatalf("runPullRequestEvent() error = %v, want bounded-retry error", err)
	}
	if got := strings.Count(strings.Join(calls, ","), "issue:get"); got != 8 {
		t.Fatalf("issue reads = %d, want 8", got)
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
			if err := runPullRequestEvent(newPullRequestEventDependencies(tt.pullRequests, tt.issues), "flock-php", "flock", []byte(tt.data)); err == nil {
				t.Fatal("runPullRequestEvent() returned nil error")
			}
		})
	}
}

func TestRunMergeGroupEvent(t *testing.T) {
	api := &mergeGroupEventFake{}
	t.Setenv("GITHUB_RUN_ID", "7")
	if err := runMergeGroupEvent(newMergeGroupEventDependencies(api), "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRunMergeGroupEventRejectsInvalidPayloadAndUsesZeroForInvalidRunID(t *testing.T) {
	if err := runMergeGroupEvent(newMergeGroupEventDependencies(&mergeGroupEventFake{}), "flock-php", "flock", []byte(`{`)); err == nil {
		t.Fatal("runMergeGroupEvent() returned nil error")
	}

	validRunIDAPI := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "7")
	err := runMergeGroupEvent(newMergeGroupEventDependencies(validRunIDAPI), "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
	if !errors.Is(err, merge_gate.ErrNotAllowed) || validRunIDAPI.cancelExcept != 7 {
		t.Fatalf("err = %v, except = %d", err, validRunIDAPI.cancelExcept)
	}

	api := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "not-a-number")
	err = runMergeGroupEvent(newMergeGroupEventDependencies(api), "flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
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
	currentIssue  *github_issue.Issue
	permission    string
	permissionErr error
	labelErr      error
	removeErr     error
}

func newIssueEventDependencies(issues *issueEventFakeService) issueEventDependencies {
	return issueEventDependencies{
		Assignment: issues,
		Triage: issue_triage.Dependencies{
			Issues:       issues,
			Repositories: issues,
		},
	}
}

func (f *issueEventFakeService) Get(string, string, int) (*github_issue.Issue, error) {
	return f.currentIssue, nil
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
	pullRequest      *github_pr.PullRequest
	linkedIssues     []github_pr.LinkedIssue
	files            []github_pr.File
	calls            *[]string
	getErr           error
	comment          string
	gateClosed       bool
	transferredIssue int
}

func newPullRequestEventDependencies(pullRequests *pullRequestEventFake, issues *pullRequestIssueFake) pullRequestEventDependencies {
	return pullRequestEventDependencies{
		Assignment: pr_assignment.Dependencies{
			Issues:       issues,
			Repositories: issues,
		},
		Gate: pr_gate.Dependencies{
			Issues:       issues,
			PullRequests: pullRequests,
		},
		Milestones: pullRequests,
	}
}

func (f *pullRequestEventFake) Get(string, string, int) (*github_pr.PullRequest, error) {
	f.record("pr:get")
	return f.pullRequest, f.getErr
}
func (f *pullRequestEventFake) LowestOpenReference(string, string, issue_reference.Reference) (*github_pr.PullRequest, error) {
	f.record("pr:lowest-reference")
	return f.pullRequest, nil
}
func (f *pullRequestEventFake) ListFiles(string, string, int) ([]github_pr.File, error) {
	f.record("pr:list-files")
	return f.files, nil
}
func (f *pullRequestEventFake) GateClosed(string, string, int) (bool, error) {
	f.record("pr:gate-closed")
	return f.gateClosed, nil
}
func (f *pullRequestEventFake) CreateComment(_ string, _ string, _ int, comment string) error {
	f.record("pr:create-comment")
	f.comment = comment
	return nil
}
func (f *pullRequestEventFake) SetState(_, _ string, _ int, state string) error {
	f.record("pr:set-state")
	f.pullRequest.State = state
	return nil
}
func (f *pullRequestEventFake) ListLinkedIssues(string, string, int) ([]github_pr.LinkedIssue, error) {
	f.record("pr:list-linked-issues")
	return f.linkedIssues, nil
}
func (f *pullRequestEventFake) TransferIssueMilestone(_, _ string, _ int, issueNumber int) (bool, error) {
	f.record("pr:transfer-milestone")
	f.transferredIssue = issueNumber
	return false, nil
}
func (f *pullRequestEventFake) record(call string) {
	if f.calls != nil {
		*f.calls = append(*f.calls, call)
	}
}

type pullRequestIssueFake struct {
	issue         *github_issue.Issue
	issueReads    []*github_issue.Issue
	readIndex     int
	assignee      string
	assignedIssue int
	removed       bool
	claimed       bool
	calls         *[]string
	assignErr     error
	role          string
}

func (f *pullRequestIssueFake) Get(string, string, int) (*github_issue.Issue, error) {
	f.record("issue:get")
	if f.issueReads != nil {
		issue := f.issueReads[f.readIndex]
		if f.readIndex < len(f.issueReads)-1 {
			f.readIndex++
		}
		return issue, nil
	}
	return f.issue, nil
}
func (f *pullRequestIssueFake) AddAssignee(_ string, _ string, number int, username string) (*github_issue.Issue, error) {
	f.record("issue:assign")
	f.assignee = username
	f.assignedIssue = number
	return &github_issue.Issue{Assignees: append(f.issue.Assignees, github_issue.User{Login: username}), Labels: f.issue.Labels}, f.assignErr
}
func (f *pullRequestIssueFake) EnsureClaimedLabel(string, string) error {
	f.record("issue:ensure-claimed-label")
	return nil
}
func (f *pullRequestIssueFake) RemoveAssignee(_ string, _ string, _ int, username string) error {
	f.record("issue:remove-assignee")
	f.assignee = username
	return nil
}
func (f *pullRequestIssueFake) AddLabel(_ string, _ string, _ int, label string) error {
	f.record("issue:add-label")
	if label == github_issue.Claimed {
		f.claimed = true
	}
	return nil
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
func (f *pullRequestIssueFake) GetUserRole(string, string, string) (string, error) {
	f.record("repository:role")
	return f.role, nil
}

type mergeGroupEventFake struct {
	role         string
	cancelled    bool
	cancelExcept int64
}

func newMergeGroupEventDependencies(api *mergeGroupEventFake) mergeGroupEventDependencies {
	return mergeGroupEventDependencies{
		Gate: merge_gate.Dependencies{
			MergeGroups:  api,
			PullRequests: api,
			Repositories: api,
		},
	}
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
