// Package pr_assignment claims approved issues for pull request authors.
package pr_assignment

import (
	"github.com/google/go-github/v93/github"

	"flock/tools/github/github_service"
	"flock/tools/github/issue_reference"
)

type Service interface {
	Issue(owner, repository string, number int) (*github.Issue, error)
	AddAssignee(owner, repository string, number int, username string) error
	RemoveHelpWanted(owner, repository string, issue *github.Issue) error
}

func Run(api Service, owner string, issueReference issue_reference.Reference, author string) error {
	issue, err := api.Issue(owner, issueReference.Repository, issueReference.Number)
	if err != nil {
		return err
	}
	if !github_service.HasAssignee(issue, author) {
		if len(issue.Assignees) > 0 || !github_service.HasHelpWanted(issue) {
			return nil
		}
		if err := api.AddAssignee(owner, issueReference.Repository, issueReference.Number, author); err != nil {
			return err
		}
	}
	return api.RemoveHelpWanted(owner, issueReference.Repository, issue)
}
