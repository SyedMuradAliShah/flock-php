// Command github applies Flock's pull request, merge queue and issue-triage policy in GitHub Actions.
package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"flock/tools/github/github_client"
	"flock/tools/github/github_issue"
	"flock/tools/github/github_merge_group"
	"flock/tools/github/github_pr"
	"flock/tools/github/github_repository"
	"flock/tools/github/issue_assignment"
	"flock/tools/github/issue_triage"
	"flock/tools/github/merge_gate"
	"flock/tools/github/milestone_transfer"
	"flock/tools/github/pr_assignment"
	"flock/tools/github/pr_gate"
	"flock/tools/github/repository"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "pr" && os.Args[1] != "issue" && os.Args[1] != "merge") {
		fail(errors.New("usage: github pr | issue | merge"))
	}
	owner, repo, err := repository.Parse(os.Getenv("GITHUB_REPOSITORY"))
	if err != nil {
		fail(err)
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fail(errors.New("GITHUB_TOKEN is not set"))
	}
	event, err := os.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		fail(err)
	}
	client, err := github_client.New(apiURL(), token)
	if err != nil {
		fail(err)
	}

	switch os.Args[1] {
	case "pr":
		err = runPullRequestEvent(github_pr.New(client), github_issue.New(client), github_repository.New(client), owner, repo, event)
	case "issue":
		err = runIssueEvent(github_issue.New(client), github_repository.New(client), owner, repo, event)
	case "merge":
		err = runMergeGroupEvent(github_pr.New(client), github_repository.New(client), github_merge_group.New(client), owner, repo, event)
	}
	if err != nil {
		fail(err)
	}
}

type pullRequestEventPullRequestService interface {
	pr_gate.PullRequestGateService
	milestone_transfer.PullRequestMilestoneService
}

type pullRequestEventIssueService interface {
	pr_assignment.IssueClaimer
	pr_gate.IssueReader
}

func runPullRequestEvent(pullRequests pullRequestEventPullRequestService, issues pullRequestEventIssueService, repositories pr_assignment.UserRoleReader, owner, repository string, data []byte) error {
	event, err := github_pr.ParseEvent(data)
	if err != nil {
		return err
	}
	result, err := pr_gate.Execute(pullRequests, issues, owner, repository, pr_gate.Event{
		Action: event.Action, Number: event.Number, BodyEdited: event.BodyEdited, BaseEdited: event.BaseEdited,
	})
	if err != nil {
		return err
	}
	if result.ApprovedIssue == nil {
		return nil
	}
	if result.ApprovedIssue.Repository == repository {
		assignment, err := pr_assignment.Execute(issues, repositories, owner, *result.ApprovedIssue, event.Author)
		if err != nil {
			return err
		}
		if assignment.ClaimedBy != "" {
			return pr_gate.CloseClaimedIssue(pullRequests, owner, repository, event.Number, *result.ApprovedIssue, assignment.ClaimedBy)
		}
	}
	return milestone_transfer.Execute(pullRequests, owner, repository, event.Number, *result.ApprovedIssue)
}

type issueEventIssueService interface {
	issue_assignment.HelpWantedRemover
	issue_triage.IssueLabelAdder
}

func runIssueEvent(issues issueEventIssueService, repositories issue_triage.UserPermissionReader, owner, repository string, data []byte) error {
	event, err := github_issue.ParseEvent(data)
	if err != nil {
		return err
	}
	switch event.Action {
	case "opened":
		return issue_triage.Execute(issues, repositories, owner, repository, &event.Issue)
	case "assigned":
		return issue_assignment.Execute(issues, owner, repository, &event.Issue)
	default:
		return nil
	}
}

func runMergeGroupEvent(pullRequests merge_gate.MergeQueuePullRequestService, repositories merge_gate.RepositoryRoleReader, mergeGroups merge_gate.MergeGroupRunCanceler, owner, repository string, data []byte) error {
	event, err := github_merge_group.ParseEvent(data)
	if err != nil {
		return err
	}
	runID, _ := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), 10, 64)
	return merge_gate.Execute(pullRequests, repositories, mergeGroups, owner, repository, merge_gate.Event{
		HeadRef: event.HeadRef,
		HeadSHA: event.HeadSHA,
		RunID:   runID,
	})
}

func apiURL() string {
	if value := os.Getenv("GITHUB_API_URL"); value != "" {
		return value
	}
	return "https://api.github.com"
}

func fail(err error) {
	_, _ = fmt.Fprintln(os.Stderr, "github:", err)
	os.Exit(1)
}
