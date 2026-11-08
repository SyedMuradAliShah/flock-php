package issue_assignment

import (
	"testing"

	"flock/tools/github/github_service"
	"github.com/google/go-github/v93/github"
)

func TestRunRemovesHelpWantedFromAssignedIssue(t *testing.T) {
	api := &fakeService{}
	issue := &github.Issue{
		Number:    new(73),
		Assignees: []*github.User{{Login: new("contributor")}},
		Labels:    []*github.Label{{Name: github_service.HelpWanted}},
	}

	if err := Run(api, "flock-php", "flock", issue); err != nil {
		t.Fatal(err)
	}
	if !api.removed {
		t.Fatal("help wanted label was not removed")
	}
}

func TestRunSkipsUnassignedIssuesAndMissingLabels(t *testing.T) {
	for _, issue := range []*github.Issue{
		{Number: new(73), Labels: []*github.Label{{Name: github_service.HelpWanted}}},
		{Number: new(73), Assignees: []*github.User{{Login: new("contributor")}}},
	} {
		api := &fakeService{}
		if err := Run(api, "flock-php", "flock", issue); err != nil {
			t.Fatal(err)
		}
		if api.removed {
			t.Fatal("help wanted label was removed")
		}
	}
}

type fakeService struct{ removed bool }

func (f *fakeService) RemoveHelpWanted(_ string, _ string, issue *github.Issue) error {
	if github_service.HasHelpWanted(issue) {
		f.removed = true
	}
	return nil
}
