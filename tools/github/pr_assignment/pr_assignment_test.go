package pr_assignment

import (
	"errors"
	"testing"

	"flock/tools/github/github_issue"
	"flock/tools/github/issue_reference"
)

func TestRunAssignsAuthorAndRemovesHelpWanted(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{
		Number: 73,
		Labels: []github_issue.Label{{Name: github_issue.HelpWanted}},
	}}

	if _, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestExecuteReturnsServiceErrors(t *testing.T) {
	issue := &github_issue.Issue{Number: 73, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}
	for _, api := range []*fakeService{{getErr: errors.New("get")}, {issue: issue, assignErr: errors.New("assign")}, {issue: issue, removeErr: errors.New("remove")}, {issue: &github_issue.Issue{Assignees: []github_issue.User{{Login: "another-contributor"}}}, roleErr: errors.New("role")}} {
		if _, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err == nil {
			t.Fatal("Execute() returned nil error")
		}
	}
}

func TestExecuteReportsExistingContributorClaim(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "another-contributor"}}}}
	result, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor")
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
		Assignees: []github_issue.User{{Login: "contributor"}},
		Labels:    []github_issue.Label{{Name: github_issue.HelpWanted}},
	}}
	if _, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "" || !api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestExecuteAllowsMaintainerAssignments(t *testing.T) {
	api := &fakeService{issue: &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "maintainer"}}, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}, roles: map[string]string{"maintainer": "maintain"}}
	if _, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestExecuteReleasesAuthorWhenConcurrentClaimWins(t *testing.T) {
	api := &fakeService{
		issue:         &github_issue.Issue{Number: 73, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
		assignedIssue: &github_issue.Issue{Number: 73, Assignees: []github_issue.User{{Login: "contributor"}, {Login: "other-contributor"}}},
	}
	result, err := Execute(api, api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor")
	if err != nil || result.ClaimedBy != "other-contributor" {
		t.Fatalf("Execute() = %+v, %v", result, err)
	}
	if api.removed || api.unassigned != "contributor" {
		t.Fatalf("removed = %t, unassigned = %q", api.removed, api.unassigned)
	}
}

type fakeService struct {
	issue         *github_issue.Issue
	assignedIssue *github_issue.Issue
	assignee      string
	unassigned    string
	removed       bool
	roles         map[string]string
	getErr        error
	assignErr     error
	removeErr     error
	roleErr       error
}

func (f *fakeService) Get(string, string, int) (*github_issue.Issue, error) { return f.issue, f.getErr }
func (f *fakeService) AddAssignee(string, string, int, string) (*github_issue.Issue, error) {
	f.assignee = "contributor"
	if f.assignedIssue != nil {
		return f.assignedIssue, f.assignErr
	}
	return &github_issue.Issue{Number: f.issue.Number, Assignees: append(f.issue.Assignees, github_issue.User{Login: "contributor"}), Labels: f.issue.Labels}, f.assignErr
}
func (f *fakeService) RemoveAssignee(_ string, _ string, _ int, username string) error {
	f.unassigned = username
	return nil
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
