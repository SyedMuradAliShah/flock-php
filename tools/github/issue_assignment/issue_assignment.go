// Package issue_assignment removes public availability from assigned issues.
package issue_assignment

import "flock/tools/github/github_issue"

type HelpWantedRemover interface {
	RemoveHelpWanted(owner, repository string, issue *github_issue.Issue) error
}

func Execute(api HelpWantedRemover, owner, repository string, issue *github_issue.Issue) error {
	if issue == nil {
		return nil
	}
	if len(issue.Assignees) == 0 {
		return nil
	}
	return api.RemoveHelpWanted(owner, repository, issue)
}
