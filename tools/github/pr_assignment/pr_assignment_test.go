package pr_assignment

import (
	"errors"
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/issue_reference"
)

func TestRunAssignsAuthorAndMarksIssueClaimed(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{
		Number: 73, State: "open",
		Labels: []github_issue.Label{{Name: github_issue.HelpWanted}},
	}}

	if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.claimed || !api.removed {
		t.Fatalf("assignee = %q, claimed = %t, removed = %t", api.assignee, api.claimed, api.removed)
	}
}

func TestExecuteReturnsServiceErrors(t *testing.T) {
	issue := &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}
	for _, api := range []*fakeService{{getErr: errors.New("get")}, {issue: issue, assignErr: errors.New("assign")}, {issue: issue, labelErr: errors.New("label")}, {issue: issue, removeErr: errors.New("remove")}, {issue: &github_issue.Issue{Number: 73, State: "open", Assignees: []github_issue.User{{Login: "another-contributor"}}, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}, roleErr: errors.New("role")}} {
		if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err == nil {
			t.Fatal("Execute() returned nil error")
		}
	}
}

func TestExecuteReportsExistingContributorClaim(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{Number: 73, State: "open", Assignees: []github_issue.User{{Login: "another-contributor"}}, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}}
	result, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor")
	if err != nil || result.ClaimedBy != "another-contributor" {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	if api.assignee != "" || api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestRunRemovesHelpWantedWhenTheAuthorIsAlreadyAssigned(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{
		Number:    73,
		State:     "open",
		Assignees: []github_issue.User{{Login: "contributor"}},
		Labels:    []github_issue.Label{{Name: github_issue.HelpWanted}},
	}}
	if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "" || !api.claimed || !api.removed {
		t.Fatalf("assignee = %q, claimed = %t, removed = %t", api.assignee, api.claimed, api.removed)
	}
}

func TestExecuteAllowsMaintainerAssignments(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{Number: 73, State: "open", Assignees: []github_issue.User{{Login: "maintainer"}}, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}, roles: map[string]string{"maintainer": "maintain"}}
	if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.claimed || !api.removed {
		t.Fatalf("assignee = %q, claimed = %t, removed = %t", api.assignee, api.claimed, api.removed)
	}
}

func TestExecuteMarksAnIssueClaimedWhenGitHubIgnoresAssignment(t *testing.T) {
	api := &fakeService{
		issue: &github_issue.Issue{
			Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}},
		},
		assignedIssue: &github_issue.Issue{
			Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}},
		},
	}

	if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.claimed || !api.removed {
		t.Fatalf("assignee = %q, claimed = %t, removed = %t", api.assignee, api.claimed, api.removed)
	}
}

func TestExecuteReleasesAuthorWhenConcurrentClaimWins(t *testing.T) {
	api := &fakeService{
		issue:         &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
		assignedIssue: &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "zulu"}, {Login: "other-contributor"}}},
	}
	result, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "zulu")
	if err != nil || result.ClaimedBy != "other-contributor" {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	if api.removed || api.unassigned != "zulu" {
		t.Fatalf("removed = %t, unassigned = %q", api.removed, api.unassigned)
	}
}

func TestExecuteUsesTheSameWinnerForConcurrentClaims(t *testing.T) {
	assignedIssue := &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "zulu"}, {Login: "alpha"}}}
	for _, author := range []string{"alpha", "zulu"} {
		api := &fakeService{
			issue:         &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
			assignedIssue: assignedIssue,
		}
		result, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, author)
		if err != nil {
			t.Fatal(err)
		}
		if author == "alpha" && api.unassigned != "" {
			t.Fatalf("winning author %q was unassigned", author)
		}
		if author == "zulu" && (result.ClaimedBy != "alpha" || api.unassigned != author) {
			t.Fatalf("losing author result = %+v, unassigned = %q", result, api.unassigned)
		}
	}
}

func TestExecuteRetriesCleanupAfterVerificationFailure(t *testing.T) {
	initialIssue := &github_issue.Issue{Number: 73, State: "open", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}
	assignedIssue := &github_issue.Issue{
		Number:    73,
		State:     "open",
		Assignees: []github_issue.User{{Login: "zulu"}, {Login: "alpha"}},
		Labels:    []github_issue.Label{{Name: github_issue.HelpWanted}},
	}
	api := &fakeService{
		issue:         initialIssue,
		assignedIssue: assignedIssue,
		roleErr:       errors.New("role"),
		removeErr:     errors.New("remove"),
	}
	if _, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "zulu"); err == nil {
		t.Fatal("Execute() returned nil error when verification and cleanup failed")
	}
	if api.unassigned != "zulu" {
		t.Fatalf("unassigned = %q, want zulu", api.unassigned)
	}

	api.issue = assignedIssue
	api.roleErr = nil
	api.removeErr = nil
	result, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "zulu")
	if err != nil || result.ClaimedBy != "alpha" || api.unassigned != "zulu" {
		t.Fatalf("retry result = %+v, unassigned = %q, err = %v", result, api.unassigned, err)
	}
}

func TestExecuteReportsWithdrawnApprovalWithoutClaiming(t *testing.T) {
	for _, issue := range []*github_issue.Issue{
		nil,
		{Number: 73, State: "closed", Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
		{Number: 73, State: "open"},
	} {
		api := &fakeService{issue: issue}
		result, err := Execute(Dependencies{Issues: api, Repositories: api}, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor")
		if err != nil || !result.ApprovalWithdrawn {
			t.Fatalf("Execute() = %+v, %v; want withdrawn approval", result, err)
		}
		if api.assignee != "" || api.removed || api.unassigned != "" {
			t.Fatalf("unexpected claim side effects: %+v", api)
		}
	}
}

type fakeService struct {
	issue         *github_issue.Issue
	assignedIssue *github_issue.Issue
	assignee      string
	unassigned    string
	removed       bool
	claimed       bool
	roles         map[string]string
	getErr        error
	assignErr     error
	removeErr     error
	labelErr      error
	roleErr       error
}

func (f *fakeService) Get(string, string, int) (*github_issue.Issue, error) { return f.issue, f.getErr }
func (f *fakeService) AddAssignee(_ string, _ string, _ int, username string) (*github_issue.Issue, error) {
	f.assignee = username
	if f.assignedIssue != nil {
		return f.assignedIssue, f.assignErr
	}
	return &github_issue.Issue{Number: f.issue.Number, State: f.issue.State, Assignees: append(f.issue.Assignees, github_issue.User{Login: username}), Labels: f.issue.Labels}, f.assignErr
}
func (f *fakeService) RemoveAssignee(_ string, _ string, _ int, username string) error {
	f.unassigned = username
	return f.removeErr
}
func (f *fakeService) AddLabel(_ string, _ string, _ int, label string) error {
	if label == github_issue.Claimed {
		f.claimed = true
	}
	return f.labelErr
}
func (f *fakeService) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	if github_issue.HasHelpWanted(f.issue) {
		f.removed = true
	}
	return f.removeErr
}
func (f *fakeService) GetUserRole(_ string, _ string, username string) (string, error) {
	return f.roles[username], f.roleErr
}
