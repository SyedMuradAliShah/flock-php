// Command github applies Flock's pull request, merge queue and issue-triage policy in GitHub Actions.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/google/go-github/v93/github"

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
		err = runPullRequestEvent(github_pr.New(client), github_issue.New(client), owner, repo, event)
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

func runPullRequestEvent(pullRequests pullRequestEventPullRequestService, issues pullRequestEventIssueService, owner, repository string, data []byte) error {
	var event github.PullRequestEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	result, err := pr_gate.Execute(pullRequests, issues, owner, repository, pullRequestGateEvent(&event))
	if err != nil {
		return err
	}
	if result.ApprovedIssue == nil {
		return nil
	}
	if result.ApprovedIssue.Repository == repository {
		if err := pr_assignment.Execute(issues, owner, *result.ApprovedIssue, event.GetPullRequest().GetUser().GetLogin()); err != nil {
			return err
		}
	}
	return milestone_transfer.Execute(pullRequests, owner, repository, event.GetPullRequest().GetNumber(), *result.ApprovedIssue)
}

func pullRequestGateEvent(event *github.PullRequestEvent) pr_gate.Event {
	changes := event.GetChanges()
	return pr_gate.Event{
		Action:     event.GetAction(),
		Number:     event.GetPullRequest().GetNumber(),
		BodyEdited: changes != nil && changes.Body != nil,
		BaseEdited: changes != nil && changes.Base != nil,
	}
}

type issueEventIssueService interface {
	issue_assignment.HelpWantedRemover
	issue_triage.IssueLabelAdder
}

func runIssueEvent(issues issueEventIssueService, repositories issue_triage.UserPermissionReader, owner, repository string, data []byte) error {
	var event github.IssuesEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	switch event.GetAction() {
	case "opened":
		return issue_triage.Execute(issues, repositories, owner, repository, event.GetIssue())
	case "assigned":
		return issue_assignment.Execute(issues, owner, repository, event.GetIssue())
	default:
		return nil
	}
}

func runMergeGroupEvent(pullRequests merge_gate.MergeQueuePullRequestService, repositories merge_gate.RepositoryRoleReader, mergeGroups merge_gate.MergeGroupRunCanceler, owner, repository string, data []byte) error {
	var event github.MergeGroupEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	runID, _ := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), 10, 64)
	return merge_gate.Execute(pullRequests, repositories, mergeGroups, owner, repository, merge_gate.Event{
		HeadRef: event.GetMergeGroup().GetHeadRef(),
		HeadSHA: event.GetMergeGroup().GetHeadSHA(),
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
