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
		service := PullRequestEventService{
			PullRequests: github_pr.New(client),
			Issues:       github_issue.New(client),
			Repositories: github_repository.New(client),
		}
		err = service.Run(owner, repo, event)
	case "issue":
		err = runIssueEvent(github_issue.New(client), github_repository.New(client), owner, repo, event)
	case "merge":
		err = runMergeGroupEvent(github_pr.New(client), github_repository.New(client), github_merge_group.New(client), owner, repo, event)
	}
	if err != nil {
		fail(err)
	}
}

const maxApprovalRetries = 3

// PullRequestEventService coordinates contribution policies for pull request events.
type PullRequestEventService struct {
	PullRequests *github_pr.Service
	Issues       *github_issue.Service
	Repositories *github_repository.Service
}

func (s PullRequestEventService) Run(owner, repository string, data []byte) error {
	return runPullRequestEvent(pullRequestEventDependencies{
		PullRequestGate:       s.PullRequests,
		PullRequestMilestones: s.PullRequests,
		IssueReader:           s.Issues,
		IssueClaimer:          s.Issues,
		Repositories:          s.Repositories,
	}, owner, repository, data)
}

type pullRequestEventDependencies struct {
	PullRequestGate       pr_gate.PullRequestGateService
	PullRequestMilestones milestone_transfer.PullRequestMilestoneService
	IssueReader           pr_gate.IssueReader
	IssueClaimer          pr_assignment.IssueClaimer
	Repositories          pr_assignment.UserRoleReader
}

func runPullRequestEvent(dependencies pullRequestEventDependencies, owner, repository string, data []byte) error {
	event, err := github_pr.ParseEvent(data)
	if err != nil {
		return err
	}
	result, err := pr_gate.Execute(dependencies.PullRequestGate, dependencies.IssueReader, owner, repository, pr_gate.Event{
		Action: event.Action, Number: event.Number, BodyEdited: event.BodyEdited, BaseEdited: event.BaseEdited,
	})
	if err != nil {
		return err
	}
	for retries := 0; result.ApprovedIssue != nil; {
		approvedIssue := *result.ApprovedIssue
		if approvedIssue.Repository == repository {
			assignment, err := pr_assignment.Execute(dependencies.IssueClaimer, dependencies.Repositories, owner, approvedIssue, event.Author)
			if err != nil {
				return err
			}
			if assignment.ApprovalWithdrawn {
				if retries == maxApprovalRetries {
					return fmt.Errorf("issue approval changed repeatedly while processing pull request #%d", event.Number)
				}
				retries++
				result, err = pr_gate.Execute(dependencies.PullRequestGate, dependencies.IssueReader, owner, repository, pr_gate.Event{
					Action: "synchronize", Number: event.Number,
				})
				if err != nil {
					return err
				}
				continue
			}
			if assignment.ClaimedBy != "" {
				return pr_gate.CloseClaimedIssue(dependencies.PullRequestGate, owner, repository, event.Number, approvedIssue, assignment.ClaimedBy)
			}
		}
		return milestone_transfer.Execute(dependencies.PullRequestMilestones, owner, repository, event.Number, approvedIssue)
	}
	return nil
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
