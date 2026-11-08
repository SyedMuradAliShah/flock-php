// Package github_merge_group implements GitHub merge group operations used by workflows.
package github_merge_group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/go-github/v93/github"
)

// Event is a merge-group webhook event.
type Event struct {
	HeadRef string
	HeadSHA string
}

type Service struct {
	client *github.Client
}

// ParseEvent parses a merge-group webhook payload into owned workflow data.
func ParseEvent(data []byte) (Event, error) {
	var event github.MergeGroupEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, err
	}
	mergeGroup := event.GetMergeGroup()
	return Event{HeadRef: mergeGroup.GetHeadRef(), HeadSHA: mergeGroup.GetHeadSHA()}, nil
}

func New(client *github.Client) *Service {
	return &Service{client: client}
}

func (s *Service) CancelOtherRuns(owner, repository, headSHA string, except int64) error {
	runs, _, err := s.client.Actions.ListRepositoryWorkflowRuns(context.Background(), owner, repository, &github.ListWorkflowRunsOptions{
		Event:   "merge_group",
		HeadSHA: headSHA,
		PerPage: 100,
	})
	if err != nil {
		return err
	}
	for _, run := range runs.WorkflowRuns {
		if run.GetID() == except || run.GetStatus() == "completed" {
			continue
		}
		_, err := s.client.Actions.CancelWorkflowRunByID(context.Background(), owner, repository, run.GetID())
		if _, accepted := errors.AsType[*github.AcceptedError](err); err != nil && !accepted {
			return fmt.Errorf("cancel run %d: %w", run.GetID(), err)
		}
	}
	return nil
}
