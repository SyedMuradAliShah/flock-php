package pr_assignment

import (
	"testing"

	"flock/tools/github/github_service"
	"flock/tools/github/issue_reference"
	"github.com/google/go-github/v93/github"
)

func TestRunAssignsAuthorAndRemovesHelpWanted(t *testing.T) {
	api := &fakeService{issue: &github.Issue{
		Number: new(73),
		Labels: []*github.Label{{Name: github_service.HelpWanted}},
	}}

	if err := Run(api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
		t.Fatal(err)
	}
	if api.assignee != "contributor" || !api.removed {
		t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
	}
}

func TestRunSkipsAlreadyClaimedIssuesAndMissingLabels(t *testing.T) {
	for _, issue := range []*github.Issue{
		{Number: new(73), Assignees: []*github.User{{Login: new("another-contributor")}}, Labels: []*github.Label{{Name: github_service.HelpWanted}}},
		{Number: new(73)},
	} {
		api := &fakeService{issue: issue}
		if err := Run(api, "flock-php", issue_reference.Reference{Repository: "flock", Number: 73}, "contributor"); err != nil {
			t.Fatal(err)
		}
		if api.assignee != "" || api.removed {
			t.Fatalf("assignee = %q, removed = %t", api.assignee, api.removed)
		}
	}
}

type fakeService struct {
	issue    *github.Issue
	assignee string
	removed  bool
}

func (f *fakeService) Issue(string, string, int) (*github.Issue, error) { return f.issue, nil }
func (f *fakeService) AddAssignee(string, string, int, string) error {
	f.assignee = "contributor"
	return nil
}
func (f *fakeService) RemoveHelpWanted(string, string, *github.Issue) error {
	if github_service.HasHelpWanted(f.issue) {
		f.removed = true
	}
	return nil
}
