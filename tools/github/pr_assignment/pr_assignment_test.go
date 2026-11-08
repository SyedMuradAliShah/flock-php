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

	if err := Execute(api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestExecuteReturnsServiceErrors(t *testing.T) {
	issue := &github_issue.Issue{Number: 73, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}}
	for _, api := range []*fakeService{{getErr: errors.New("get")}, {issue: issue, assignErr: errors.New("assign")}, {issue: issue, removeErr: errors.New("remove")}} {
		if err := Execute(api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err == nil {
			t.Fatal("Execute() returned nil error")
		}
	}
}

func TestRunSkipsAlreadyClaimedIssuesAndMissingLabels(t *testing.T) {
	for _, issue := range []*github_issue.Issue{
		{Number: 73, Assignees: []github_issue.User{{Login: "another-contributor"}}, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
		{Number: 73},
	} {
		api := &fakeService{issue: issue}
		if err := Execute(api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
			t.Fatal(err)
		}
		if api.assignee != "" || api.removed {
			t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
		}
	}
}

type fakeService struct {
	issue     *github_issue.Issue
	assignee  string
	removed   bool
	getErr    error
	assignErr error
	removeErr error
}

func (f *fakeService) Get(string, string, int) (*github_issue.Issue, error) { return f.issue, f.getErr }
func (f *fakeService) AddAssignee(string, string, int, string) error {
	f.assignee = "contributor"
	return f.assignErr
}
func (f *fakeService) RemoveHelpWanted(string, string, *github_issue.Issue) error {
	if github_issue.HasHelpWanted(f.issue) {
		f.removed = true
	}
	return f.removeErr
}
