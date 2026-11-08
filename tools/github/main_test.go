package main

import (
	"encoding/json"
	"testing"

	"github.com/google/go-github/v93/github"
)

func TestPullRequestEventWithoutChangesDoesNotPanic(t *testing.T) {
	var event github.PullRequestEvent
	if err := json.Unmarshal([]byte(`{"action":"opened","pull_request":{"number":87}}`), &event); err != nil {
		t.Fatal(err)
	}

	gateEvent := pullRequestGateEvent(&event)
	if gateEvent.BodyEdited {
		t.Fatal("BodyEdited = true, want false")
	}
	if gateEvent.BaseEdited {
		t.Fatal("BaseEdited = true, want false")
	}
}

func TestRunIssueEventRoutesActions(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantTriage   bool
		wantUnassign bool
	}{
		{
			name:       "opened issues go to triage",
			data:       `{"action":"opened","issue":{"number":73,"user":{"login":"contributor","type":"User"}}}`,
			wantTriage: true,
		},
		{
			name:         "assigned issues lose public availability",
			data:         `{"action":"assigned","issue":{"number":73,"assignees":[{"login":"contributor"}],"labels":[{"name":"help wanted"}]}}`,
			wantUnassign: true,
		},
		{
			name: "other issue actions do nothing",
			data: `{"action":"labeled","issue":{"number":73}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &issueEventFakeService{}
			if err := runIssueEvent(api, "flock-php", "flock", []byte(tt.data)); err != nil {
				t.Fatal(err)
			}
			if api.triaged != tt.wantTriage {
				t.Fatalf("triaged = %t, want %t", api.triaged, tt.wantTriage)
			}
			if api.removed != tt.wantUnassign {
				t.Fatalf("removed = %t, want %t", api.removed, tt.wantUnassign)
			}
		})
	}
}

type issueEventFakeService struct {
	triaged bool
	removed bool
}

func (f *issueEventFakeService) Permission(string, string, string) (string, error) {
	return "read", nil
}

func (f *issueEventFakeService) AddLabel(string, string, int, string) error {
	f.triaged = true
	return nil
}

func (f *issueEventFakeService) RemoveHelpWanted(string, string, *github.Issue) error {
	f.removed = true
	return nil
}
