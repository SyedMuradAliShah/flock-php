// Package pr_assignment claims approved issues for pull request authors.
package pr_assignment

import (
	"fmt"
	"slices"
	"sort"

	"flock/tools/github/github_issue"
	"flock/tools/github/issue_reference"
)

type IssueClaimer interface {
	Get(owner, repository string, number int) (*github_issue.Issue, error)
	AddAssignee(owner, repository string, number int, username string) (*github_issue.Issue, error)
	RemoveAssignee(owner, repository string, number int, username string) error
	AddLabel(owner, repository string, number int, label string) error
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
	claim := issueClaim{
		dependencies:   dependencies,
		owner:          owner,
		issueReference: issueReference,
		author:         author,
	}
	return claim.evaluate()
}

type issueClaim struct {
	dependencies   Dependencies
	owner          string
	issueReference issue_reference.Reference
	author         string
}

func (c issueClaim) evaluate() (Result, error) {
	issue, err := c.dependencies.Issues.Get(c.owner, c.issueReference.Repository, c.issueReference.Number)
	if err != nil {
		return Result{}, err
	}
	if !approvedFor(issue, c.author) {
		return Result{ApprovalWithdrawn: true}, nil
	}
	claimedBy, err := c.winningAssignee(issue)
	if err != nil {
		return Result{}, err
	}
	if claimedBy != "" && claimedBy != c.author {
		return c.releaseAuthor(issue, claimedBy)
	}
	if github_issue.HasAssignee(issue, c.author) {
		return c.markClaimed(issue)
	}
	return c.assignAuthor()
}

func (c issueClaim) assignAuthor() (Result, error) {
	issue, err := c.dependencies.Issues.AddAssignee(c.owner, c.issueReference.Repository, c.issueReference.Number, c.author)
	if err != nil {
		return Result{}, err
	}
	claimedBy, err := c.winningAssignee(issue)
	if err != nil {
		return c.cleanupFailedAssignment(issue, err)
	}
	if claimedBy != c.author {
		if err := c.removeAuthor(issue); err != nil {
			return Result{}, err
		}
		if claimedBy != "" {
			return Result{ClaimedBy: claimedBy}, nil
		}
	}
	return c.markClaimed(issue)
}

func (c issueClaim) releaseAuthor(issue *github_issue.Issue, claimedBy string) (Result, error) {
	if err := c.removeAuthor(issue); err != nil {
		return Result{}, err
	}
	return Result{ClaimedBy: claimedBy}, nil
}

func (c issueClaim) cleanupFailedAssignment(issue *github_issue.Issue, claimErr error) (Result, error) {
	if cleanupErr := c.removeAuthor(issue); cleanupErr != nil {
		return Result{}, fmt.Errorf("verify issue claim: %w; remove unsuccessful assignment: %v", claimErr, cleanupErr)
	}
	return Result{}, claimErr
}

func (c issueClaim) removeAuthor(issue *github_issue.Issue) error {
	if !github_issue.HasAssignee(issue, c.author) {
		return nil
	}
	return c.dependencies.Issues.RemoveAssignee(c.owner, c.issueReference.Repository, c.issueReference.Number, c.author)
}

func (c issueClaim) markClaimed(issue *github_issue.Issue) (Result, error) {
	return Result{}, markClaimed(c.dependencies.Issues, c.owner, c.issueReference.Repository, issue)
}

func (c issueClaim) winningAssignee(issue *github_issue.Issue) (string, error) {
	return winningAssignee(c.dependencies.Repositories, c.owner, c.issueReference.Repository, issue)
}

func markClaimed(issues IssueClaimer, owner, repository string, issue *github_issue.Issue) error {
	if err := issues.AddLabel(owner, repository, issue.Number, github_issue.Claimed); err != nil {
		return err
	}
	return issues.RemoveHelpWanted(owner, repository, issue)
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
	assignees = slices.Compact(assignees)
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
