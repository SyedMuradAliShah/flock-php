// Package issue_assignment removes public availability from assigned issues.
package issue_assignment

import "flock/tools/github/github_issue"

type HelpWantedRemover interface {
	Get(owner, repository string, number int) (*github_issue.Issue, error)
	RemoveHelpWanted(owner, repository string, issue *github_issue.Issue) error
}

func Execute(issues HelpWantedRemover, owner, repository string, issue *github_issue.Issue) error {
	if issue == nil {
		return nil
	}
	currentIssue, err := issues.Get(owner, repository, issue.Number)
	if err != nil {
		return err
	}
	if currentIssue == nil {
		return nil
	}
	if len(currentIssue.Assignees) == 0 {
		return nil
	}
	return issues.RemoveHelpWanted(owner, repository, currentIssue)
}
