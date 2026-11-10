package main

import (
	"flock/tools/github/github_issue"
	"flock/tools/github/issue_assignment"
	"flock/tools/github/issue_triage"
)

// IssueEventService coordinates contribution policies for issue events.
type IssueEventService struct {
	Assignment issue_assignment.HelpWantedRemover
	Triage     issue_triage.Dependencies
}

func (s IssueEventService) Run(owner, repository string, data []byte) error {
	event, err := github_issue.ParseEvent(data)
	if err != nil {
		return err
	}
	switch event.Action {
	case "opened":
		return issue_triage.Execute(s.Triage, owner, repository, &event.Issue)
	case "assigned":
		return issue_assignment.Execute(s.Assignment, owner, repository, &event.Issue)
	default:
		return nil
	}
}
