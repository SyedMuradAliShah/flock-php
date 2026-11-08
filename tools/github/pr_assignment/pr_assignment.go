// Package pr_assignment claims approved issues for pull request authors.
package pr_assignment

import (
	"github.com/google/go-github/v93/github"

	"flock/tools/github/github_issue"
	"flock/tools/github/issue_reference"
)

type IssueClaimer interface {
	Get(owner, repository string, number int) (*github.Issue, error)
	AddAssignee(owner, repository string, number int, username string) error
	RemoveHelpWanted(owner, repository string, issue *github.Issue) error
}

func Execute(api IssueClaimer, owner string, issueReference issue_reference.Reference, author string) error {
	issue, err := api.Get(owner, issueReference.Repository, issueReference.Number)
	if err != nil {
		return err
	}
	if !github_issue.HasAssignee(issue, author) {
		if len(issue.Assignees) > 0 || !github_issue.HasHelpWanted(issue) {
			return nil
		}
		if err := api.AddAssignee(owner, issueReference.Repository, issueReference.Number, author); err != nil {
			return err
		}
	}
	return api.RemoveHelpWanted(owner, issueReference.Repository, issue)
}
