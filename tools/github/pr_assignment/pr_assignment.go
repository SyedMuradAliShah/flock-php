// Package pr_assignment claims approved issues for pull request authors.
package pr_assignment

import (
	"flock/tools/github/github_issue"
	"flock/tools/github/issue_reference"
)

type IssueClaimer interface {
	Get(owner, repository string, number int) (*github_issue.Issue, error)
	AddAssignee(owner, repository string, number int, username string) (*github_issue.Issue, error)
	RemoveAssignee(owner, repository string, number int, username string) error
	RemoveHelpWanted(owner, repository string, issue *github_issue.Issue) error
}

type UserRoleReader interface {
	GetUserRole(owner, repository, username string) (string, error)
}

type Result struct {
	ClaimedBy string
}

func Execute(api IssueClaimer, repositories UserRoleReader, owner string, issueReference issue_reference.Reference, author string) (Result, error) {
	issue, err := api.Get(owner, issueReference.Repository, issueReference.Number)
	if err != nil {
		return Result{}, err
	}
	if issue == nil {
		return Result{}, nil
	}
	claimedBy, err := externalAssignee(repositories, owner, issueReference.Repository, issue, author)
	if err != nil {
		return Result{}, err
	}
	if claimedBy != "" {
		return Result{ClaimedBy: claimedBy}, nil
	}
	if github_issue.HasAssignee(issue, author) {
		return Result{}, api.RemoveHelpWanted(owner, issueReference.Repository, issue)
	}

	issue, err = api.AddAssignee(owner, issueReference.Repository, issueReference.Number, author)
	if err != nil {
		return Result{}, err
	}
	claimedBy, err = externalAssignee(repositories, owner, issueReference.Repository, issue, author)
	if err != nil {
		return Result{}, err
	}
	if claimedBy != "" {
		if err := api.RemoveAssignee(owner, issueReference.Repository, issueReference.Number, author); err != nil {
			return Result{}, err
		}
		return Result{ClaimedBy: claimedBy}, nil
	}
	return Result{}, api.RemoveHelpWanted(owner, issueReference.Repository, issue)
}

func externalAssignee(repositories UserRoleReader, owner, repository string, issue *github_issue.Issue, author string) (string, error) {
	for _, assignee := range issue.Assignees {
		if assignee.Login == author {
			continue
		}
		role, err := repositories.GetUserRole(owner, repository, assignee.Login)
		if err != nil {
			return "", err
		}
		if role != "admin" && role != "maintain" {
			return assignee.Login, nil
		}
	}
	return "", nil
}
