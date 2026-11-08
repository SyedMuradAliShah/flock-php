package milestone_transfer

import (
	"errors"
	"testing"

	"flock/tools/github/github_pr"
	"flock/tools/github/issue_reference"
)

func TestRunUsesFirstUserLinkedIssueWithAMilestone(t *testing.T) {
	milestone := 3
	api := &fakeService{
		pullRequest: pullRequest("open"),
		issues: []github_pr.LinkedIssue{
			{Number: 72, MilestoneNumber: &milestone},
			{Number: 73, MilestoneNumber: &milestone},
		},
	}
	if err := Execute(api, "flock-php", "flock", 87, issue_reference.Reference{Repository: "flock", Number: 73}); err != nil {
		t.Fatal(err)
	}
	if api.issueNumber != 73 {
		t.Fatalf("issue number = %d, want 73", api.issueNumber)
	}
}

func TestRunDoesNotTransferWithoutAMilestonedUserLinkedIssue(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("open"), issues: []github_pr.LinkedIssue{{Number: 73}}}
	if err := Execute(api, "flock-php", "flock", 87, issue_reference.Reference{Repository: "flock", Number: 73}); err != nil {
		t.Fatal(err)
	}
	if api.issueNumber != 0 {
		t.Fatalf("issue number = %d, want none", api.issueNumber)
	}
}

func TestRunSkipsClosedPullRequests(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("closed")}
	if err := Execute(api, "flock-php", "flock", 87, issue_reference.Reference{Repository: "flock", Number: 73}); err != nil {
		t.Fatal(err)
	}
	if api.issueNumber != 0 {
		t.Fatalf("issue number = %d, want none", api.issueNumber)
	}
}

func TestRunSkipsCrossRepositoryIssues(t *testing.T) {
	api := &fakeService{pullRequest: pullRequest("open")}
	if err := Execute(api, "flock-php", "flock", 87, issue_reference.Reference{Repository: "binaries", Number: 73}); err != nil {
		t.Fatal(err)
	}
	if api.issueNumber != 0 {
		t.Fatalf("issue number = %d, want none", api.issueNumber)
	}
}

func TestRunReturnsServiceErrors(t *testing.T) {
	milestone := 3
	for _, api := range []*fakeService{
		{getErr: errors.New("get")},
		{pullRequest: pullRequest("open"), listErr: errors.New("list")},
		{pullRequest: pullRequest("open"), issues: []github_pr.LinkedIssue{{Number: 73, MilestoneNumber: &milestone}}, transferErr: errors.New("transfer")},
	} {
		if err := Execute(api, "flock-php", "flock", 87, issue_reference.Reference{Repository: "flock", Number: 73}); err == nil {
			t.Fatal("Execute() returned nil error")
		}
	}
}

type fakeService struct {
	pullRequest *github_pr.PullRequest
	issues      []github_pr.LinkedIssue
	issueNumber int
	getErr      error
	listErr     error
	transferErr error
}

func (f *fakeService) Get(string, string, int) (*github_pr.PullRequest, error) {
	return f.pullRequest, f.getErr
}
func (f *fakeService) ListLinkedIssues(string, string, int) ([]github_pr.LinkedIssue, error) {
	return f.issues, f.listErr
}
func (f *fakeService) TransferIssueMilestone(_ string, _ string, _ int, issueNumber int) (bool, error) {
	f.issueNumber = issueNumber
	return true, f.transferErr
}

func pullRequest(state string) *github_pr.PullRequest {
	return &github_pr.PullRequest{State: state}
}
