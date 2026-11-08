// Package github_merge_group implements GitHub merge group operations used by workflows.
package github_merge_group

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v93/github"
)

type Service struct {
	client *github.Client
}

func New(client *github.Client) *Service {
	return &Service{client: client}
}

func (s *Service) CancelOtherRuns(owner, repository, headSHA string, except int64) error {
	runs, _, err := s.client.Actions.ListRepositoryWorkflowRuns(context.Background(), owner, repository, &github.ListWorkflowRunsOptions{
		Event:       "merge_group",
		HeadSHA:     headSHA,
		ListOptions: github.ListOptions{PerPage: 100},
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
