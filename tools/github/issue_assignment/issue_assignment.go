// Package issue_assignment removes public availability from assigned issues.
package issue_assignment

import "github.com/google/go-github/v93/github"

type Service interface {
	RemoveHelpWanted(owner, repository string, issue *github.Issue) error
}

func Run(api Service, owner, repository string, issue *github.Issue) error {
	if len(issue.Assignees) == 0 {
		return nil
	}
	return api.RemoveHelpWanted(owner, repository, issue)
}
