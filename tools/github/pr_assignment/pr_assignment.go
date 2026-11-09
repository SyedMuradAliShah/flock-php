// Package pr_assignment claims approved issues for pull request authors.
package pr_assignment

import (
	"fmt"
	"sort"

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

type Dependencies struct {
	Issues       IssueClaimer
	Repositories UserRoleReader
}

type Result struct {
	ClaimedBy         string
	ApprovalWithdrawn bool
}

func Execute(dependencies Dependencies, owner string, issueReference issue_reference.Reference, author string) (Result, error) {
	issue, err := dependencies.Issues.Get(owner, issueReference.Repository, issueReference.Number)
	if err != nil {
		return Result{}, err
	}
	if !approvedFor(issue, author) {
		return Result{ApprovalWithdrawn: true}, nil
	}
	claimedBy, err := winningAssignee(dependencies.Repositories, owner, issueReference.Repository, issue)
	if err != nil {
		return Result{}, err
	}
	if claimedBy != "" && claimedBy != author {
		if github_issue.HasAssignee(issue, author) {
			if err := dependencies.Issues.RemoveAssignee(owner, issueReference.Repository, issueReference.Number, author); err != nil {
				return Result{}, err
			}
		}
		return Result{ClaimedBy: claimedBy}, nil
	}
	if github_issue.HasAssignee(issue, author) {
		return Result{}, dependencies.Issues.RemoveHelpWanted(owner, issueReference.Repository, issue)
	}

	issue, err = dependencies.Issues.AddAssignee(owner, issueReference.Repository, issueReference.Number, author)
	if err != nil {
		return Result{}, err
	}
	if !github_issue.HasAssignee(issue, author) {
		return Result{}, fmt.Errorf("issue #%d was not assigned to %q", issueReference.Number, author)
	}
	claimedBy, err = winningAssignee(dependencies.Repositories, owner, issueReference.Repository, issue)
	if err != nil {
		if cleanupErr := dependencies.Issues.RemoveAssignee(owner, issueReference.Repository, issueReference.Number, author); cleanupErr != nil {
			return Result{}, fmt.Errorf("verify issue claim: %w; remove unsuccessful assignment: %v", err, cleanupErr)
		}
		return Result{}, err
	}
	if claimedBy != author {
		if err := dependencies.Issues.RemoveAssignee(owner, issueReference.Repository, issueReference.Number, author); err != nil {
			return Result{}, err
		}
		return Result{ClaimedBy: claimedBy}, nil
	}
	return Result{}, dependencies.Issues.RemoveHelpWanted(owner, issueReference.Repository, issue)
}

func approvedFor(issue *github_issue.Issue, author string) bool {
	return issue != nil && !issue.IsPullRequest && issue.State == "open" &&
		(github_issue.HasHelpWanted(issue) || github_issue.HasAssignee(issue, author))
}

func winningAssignee(repositories UserRoleReader, owner, repository string, issue *github_issue.Issue) (string, error) {
	assignees := make([]string, 0, len(issue.Assignees))
	for _, assignee := range issue.Assignees {
		assignees = append(assignees, assignee.Login)
	}
	sort.Strings(assignees)
	for i := 1; i < len(assignees); i++ {
		if assignees[i] == assignees[i-1] {
			assignees = append(assignees[:i], assignees[i+1:]...)
			i--
		}
	}
	for _, username := range assignees {
		role, err := repositories.GetUserRole(owner, repository, username)
		if err != nil {
			return "", err
		}
		if role != "admin" && role != "maintain" {
			return username, nil
		}
	}
	return "", nil
}
