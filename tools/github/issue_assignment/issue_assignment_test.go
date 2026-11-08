package issue_assignment

import (
	"testing"

	"flock/tools/github/github_issue"
)

func TestRunRemovesHelpWantedFromAssignedIssue(t *testing.T) {
	issue := &github_issue.Issue{
		Number:    73,
		Assignees: []github_issue.User{{Login: "contributor"}},
		Labels:    []github_issue.Label{{Name: github_issue.HelpWanted}},
	}
	api := &fakeService{currentIssue: issue}

	if err := Execute(api, "flock-php", "flock", issue); err != nil {
		t.Fatal(err)
	}
	if !api.removed {
		t.Fatal("help wanted label was not removed")
	}
}

func TestRunSkipsUnassignedIssuesAndMissingLabels(t *testing.T) {
	for _, issue := range []*github_issue.Issue{
		{Number: 73, Labels: []github_issue.Label{{Name: github_issue.HelpWanted}}},
		{Number: 73, Assignees: []github_issue.User{{Login: "contributor"}}},
	} {
		api := &fakeService{currentIssue: issue}
		if err := Execute(api, "flock-php", "flock", issue); err != nil {
			t.Fatal(err)
		}
		if api.removed {
			t.Fatal("help wanted label was removed")
		}
	}
}

func TestRunUsesCurrentIssueInsteadOfWebhookSnapshot(t *testing.T) {
	eventIssue := &github_issue.Issue{
		Number:    73,
		Assignees: []github_issue.User{{Login: "contributor"}},
		Labels:    []github_issue.Label{{Name: github_issue.HelpWanted}},
	}
	currentIssue := &github_issue.Issue{
		Number: 73,
		Labels: []github_issue.Label{{Name: github_issue.HelpWanted}},
	}
	api := &fakeService{currentIssue: currentIssue}
	if err := Execute(api, "flock-php", "flock", eventIssue); err != nil {
		t.Fatal(err)
	}
	if api.removed {
		t.Fatal("help wanted label was removed from an issue that is currently unassigned")
	}
}

type fakeService struct {
	currentIssue *github_issue.Issue
	removed      bool
}

func (f *fakeService) Get(string, string, int) (*github_issue.Issue, error) {
	return f.currentIssue, nil
}
func (f *fakeService) RemoveHelpWanted(_ string, _ string, issue *github_issue.Issue) error {
	if github_issue.HasHelpWanted(issue) {
		f.removed = true
	}
	return nil
}
