package issue_triage

import (
	"errors"
	"testing"

	"flock/tools/github/github_issue"
)

func TestRunLabelsOutsideContributor(t *testing.T) {
	api := &fakeService{}
	issue := &github_issue.Issue{Number: 73, User: github_issue.User{Type: "User"}}
	if err := Execute(api, api, "flock-php", "flock", issue); err != nil {
		t.Fatal(err)
	}
	if !api.labeled {
		t.Fatal("issue was not labeled")
	}
}

func TestExecuteReturnsPermissionAndLabelErrors(t *testing.T) {
	issue := &github_issue.Issue{Number: 73, User: github_issue.User{Type: "User"}}
	for _, api := range []*fakeService{{permissionErr: errors.New("permission")}, {labelErr: errors.New("label")}} {
		if err := Execute(api, api, "flock-php", "flock", issue); err == nil {
			t.Fatal("Execute() returned nil error")
		}
	}
}

type fakeService struct {
	labeled       bool
	permissionErr error
	labelErr      error
}

func (f *fakeService) GetUserPermission(string, string, string) (string, error) {
	return "read", f.permissionErr
}
func (f *fakeService) AddLabel(string, string, int, string) error {
	f.labeled = true
	return f.labelErr
}
