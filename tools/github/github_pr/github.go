// Package github_pr implements GitHub pull request operations used by workflows.
package github_pr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
)

const GateMarker = "<!-- pr-gate:closed -->"

type Service struct {
	client *github.Client
}

type LinkedIssue struct {
	Number          int
	MilestoneNumber *int
}

func New(client *github.Client) *Service {
	return &Service{client: client}
}

func (s *Service) Get(owner, repository string, number int) (*github.PullRequest, error) {
	pullRequest, _, err := s.client.PullRequests.Get(context.Background(), owner, repository, number)
	return pullRequest, err
}

func (s *Service) GetMergeQueueEnqueuer(owner, repository string, number int) (string, error) {
	request, err := s.client.NewRequest(context.Background(), "POST", "graphql", map[string]any{
		"query": `query($owner: String!, $repository: String!, $pullRequest: Int!) {
  repository(owner: $owner, name: $repository) {
    pullRequest(number: $pullRequest) {
      mergeQueueEntry { enqueuer { login } }
    }
  }
}`,
		"variables": map[string]any{
			"owner":       owner,
			"repository":  repository,
			"pullRequest": number,
		},
	})
	if err != nil {
		return "", err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					MergeQueueEntry *struct {
						Enqueuer struct{ Login string }
					}
				}
			}
		}
	}
	if _, err := s.client.Do(request, &response); err != nil {
		return "", err
	}
	entry := response.Data.Repository.PullRequest.MergeQueueEntry
	if entry == nil || entry.Enqueuer.Login == "" {
		return "", fmt.Errorf("pull request #%d is not in the merge queue", number)
	}
	return entry.Enqueuer.Login, nil
}

func (s *Service) ListFiles(owner, repository string, number int) ([]*github.CommitFile, error) {
	var files []*github.CommitFile
	options := &github.ListOptions{PerPage: 100}
	for {
		items, response, err := s.client.PullRequests.ListFiles(context.Background(), owner, repository, number, options)
		if err != nil {
			return nil, err
		}
		files = append(files, items...)
		if response.NextPage == 0 {
			return files, nil
		}
		options.Page = response.NextPage
	}
}

func (s *Service) ListLinkedIssues(owner, repository string, pullRequestNumber int) ([]LinkedIssue, error) {
	request, err := s.client.NewRequest(context.Background(), "POST", "graphql", map[string]any{
		"query": `query($owner: String!, $repository: String!, $pullRequest: Int!) {
  repository(owner: $owner, name: $repository) {
    pullRequest(number: $pullRequest) {
      closingIssuesReferences(first: 100) {
        nodes { number milestone { number } }
      }
    }
  }
}`,
		"variables": map[string]any{
			"owner":       owner,
			"repository":  repository,
			"pullRequest": pullRequestNumber,
		},
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ClosingIssuesReferences struct {
						Nodes []struct {
							Number    int
							Milestone *struct{ Number int }
						}
					}
				}
			}
		}
	}
	if _, err := s.client.Do(request, &response); err != nil {
		return nil, err
	}
	issues := make([]LinkedIssue, 0, len(response.Data.Repository.PullRequest.ClosingIssuesReferences.Nodes))
	for _, issue := range response.Data.Repository.PullRequest.ClosingIssuesReferences.Nodes {
		linkedIssue := LinkedIssue{Number: issue.Number}
		if issue.Milestone != nil {
			linkedIssue.MilestoneNumber = &issue.Milestone.Number
		}
		issues = append(issues, linkedIssue)
	}
	return issues, nil
}

func (s *Service) GateClosed(owner, repository string, number int) (bool, error) {
	var latestMarker time.Time
	commentOptions := &github.IssueListCommentsOptions{PerPage: 100}
	for {
		comments, response, err := s.client.Issues.ListComments(context.Background(), owner, repository, number, commentOptions)
		if err != nil {
			return false, err
		}
		for _, comment := range comments {
			if comment.GetUser().GetLogin() == "github-actions[bot]" && strings.Contains(comment.GetBody(), GateMarker) && comment.GetCreatedAt().After(latestMarker) {
				latestMarker = comment.GetCreatedAt().Time
			}
		}
		if response.NextPage == 0 {
			break
		}
		commentOptions.Page = response.NextPage
	}
	if latestMarker.IsZero() {
		return false, nil
	}

	var latestTransition *github.IssueEvent
	var latestReopen time.Time
	eventOptions := &github.ListOptions{PerPage: 100}
	for {
		events, response, err := s.client.Issues.ListIssueEvents(context.Background(), owner, repository, number, eventOptions)
		if err != nil {
			return false, err
		}
		for _, current := range events {
			if current.GetEvent() == "reopened" && current.GetCreatedAt().After(latestReopen) {
				latestReopen = current.GetCreatedAt().Time
			}
			if (current.GetEvent() == "closed" || current.GetEvent() == "reopened") &&
				(latestTransition == nil || !current.GetCreatedAt().Before(latestTransition.GetCreatedAt().Time)) {
				latestTransition = current
			}
		}
		if response.NextPage == 0 {
			break
		}
		eventOptions.Page = response.NextPage
	}

	return latestTransition != nil &&
		latestTransition.GetEvent() == "closed" &&
		latestTransition.GetActor().GetLogin() == "github-actions[bot]" &&
		!latestMarker.Before(latestReopen) &&
		!latestMarker.After(latestTransition.GetCreatedAt().Time), nil
}

func (s *Service) CreateComment(owner, repository string, number int, body string) error {
	_, _, err := s.client.Issues.CreateComment(context.Background(), owner, repository, number, github.IssueCommentRequest{Body: body})
	return err
}

func (s *Service) SetState(owner, repository string, number int, state string) error {
	_, _, err := s.client.PullRequests.Edit(context.Background(), owner, repository, number, &github.PullRequest{State: new(state)})
	return err
}

func (s *Service) TransferIssueMilestone(owner, repository string, pullRequestNumber, issueNumber int) (bool, error) {
	issue, _, err := s.client.Issues.Get(context.Background(), owner, repository, issueNumber)
	if err != nil {
		return false, err
	}
	if issue.IsPullRequest() || issue.Milestone == nil {
		return false, nil
	}
	milestone := issue.Milestone.GetNumber()
	if _, _, err := s.client.Issues.Update(context.Background(), owner, repository, pullRequestNumber, github.UpdateIssueRequest{Milestone: &milestone}); err != nil {
		return false, fmt.Errorf("set pull request milestone: %w", err)
	}
	if _, _, err := s.client.Issues.RemoveMilestone(context.Background(), owner, repository, issueNumber); err != nil {
		return false, fmt.Errorf("remove issue milestone: %w", err)
	}
	return true, nil
}
