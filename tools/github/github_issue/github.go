// Package github_issue implements GitHub issue operations used by workflows.
package github_issue

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-github/v93/github"
)

const HelpWanted = "help wanted"

type Service struct {
	client *github.Client
}

func New(client *github.Client) *Service {
	return &Service{client: client}
}

func (s *Service) Get(owner, repository string, number int) (*github.Issue, error) {
	issue, _, err := s.client.Issues.Get(context.Background(), owner, repository, number)
	return issue, err
}

func (s *Service) AddLabel(owner, repository string, number int, label string) error {
	_, _, err := s.client.Issues.AddLabelsToIssue(context.Background(), owner, repository, number, []string{label})
	return err
}

func (s *Service) RemoveLabel(owner, repository string, number int, label string) error {
	_, err := s.client.Issues.RemoveLabelForIssue(context.Background(), owner, repository, number, label)
	return err
}

func (s *Service) RemoveHelpWanted(owner, repository string, issue *github.Issue) error {
	if !HasHelpWanted(issue) {
		return nil
	}
	err := s.RemoveLabel(owner, repository, issue.GetNumber(), HelpWanted)
	var response *github.ErrorResponse
	if errors.As(err, &response) && response.Response != nil && response.Response.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

func (s *Service) AddAssignee(owner, repository string, number int, username string) error {
	_, _, err := s.client.Issues.AddAssignees(context.Background(), owner, repository, number, []string{username})
	return err
}

func HasHelpWanted(issue *github.Issue) bool {
	return HasLabel(issue, HelpWanted)
}

func HasLabel(issue *github.Issue, name string) bool {
	for _, label := range issue.Labels {
		if label.GetName() == name {
			return true
		}
	}
	return false
}

func HasAssignee(issue *github.Issue, username string) bool {
	for _, assignee := range issue.Assignees {
		if assignee.GetLogin() == username {
			return true
		}
	}
	return false
}
