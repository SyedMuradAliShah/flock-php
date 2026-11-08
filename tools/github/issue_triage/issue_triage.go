// Package issue_triage labels issues that need maintainer review.
package issue_triage

import "flock/tools/github/github_issue"

type UserPermissionReader interface {
	GetUserPermission(owner, repository, username string) (string, error)
}

type IssueLabelAdder interface {
	AddLabel(owner, repository string, number int, label string) error
}

func Execute(issues IssueLabelAdder, repositories UserPermissionReader, owner, repository string, issue *github_issue.Issue) error {
	if issue == nil {
		return nil
	}
	permission, err := repositories.GetUserPermission(owner, repository, issue.User.Login)
	if err != nil {
		return err
	}
	if bypassesTriage(issue, permission) {
		return nil
	}
	return issues.AddLabel(owner, repository, issue.Number, "needs triage")
}

func bypassesTriage(issue *github_issue.Issue, permission string) bool {
	return issue.User.Type == "Bot" || issue.AuthorAssociation == "OWNER" || issue.AuthorAssociation == "MEMBER" || issue.AuthorAssociation == "COLLABORATOR" || permission == "admin" || permission == "write"
}
