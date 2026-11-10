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
		service := IssueEventService{
			Issues:       github_issue.New(client),
			Repositories: github_repository.New(client),
		}
		err = service.Run(owner, repo, event)
	case "merge":
		service := MergeGroupEventService{
			PullRequests: github_pr.New(client),
			Repositories: github_repository.New(client),
			MergeGroups:  github_merge_group.New(client),
		}
		err = service.Execute(owner, repo, event)
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
		Assignment: pr_assignment.Dependencies{
			Issues:       s.Issues,
			Repositories: s.Repositories,
		},
		Gate: pr_gate.Dependencies{
			Issues:       s.Issues,
			PullRequests: s.PullRequests,
		},
		Milestones: s.PullRequests,
	}, owner, repository, data)
}

type pullRequestEventDependencies struct {
	Assignment pr_assignment.Dependencies
	Gate       pr_gate.Dependencies
	Milestones milestone_transfer.PullRequestMilestoneService
}

func runPullRequestEvent(dependencies pullRequestEventDependencies, owner, repository string, data []byte) error {
	event, err := github_pr.ParseEvent(data)
	if err != nil {
		return err
	}
	result, err := pr_gate.Execute(dependencies.Gate, owner, repository, pr_gate.Event{
		Action:     event.Action,
		Number:     event.Number,
		BodyEdited: event.BodyEdited,
		BaseEdited: event.BaseEdited,
	})
	if err != nil {
		return err
	}
	for retries := 0; result.ApprovedIssue != nil; {
		approvedIssue := *result.ApprovedIssue
		if result.ClaimedBy != "" {
			return pr_gate.CloseClaimedIssue(dependencies.Gate.PullRequests, owner, repository, event.Number, approvedIssue, result.ClaimedBy)
		}
		assignment, err := pr_assignment.Execute(dependencies.Assignment, owner, approvedIssue, event.Author)
		if err != nil {
			return err
		}
		if assignment.ClaimedBy != "" {
			return pr_gate.CloseClaimedIssue(dependencies.Gate.PullRequests, owner, repository, event.Number, approvedIssue, assignment.ClaimedBy)
		}
		if !assignment.ApprovalWithdrawn {
			return milestone_transfer.Execute(dependencies.Milestones, owner, repository, event.Number, approvedIssue)
		}
		if retries == maxApprovalRetries {
			return fmt.Errorf("issue approval changed repeatedly while processing pull request #%d", event.Number)
		}
		retries++
		result, err = pr_gate.Execute(dependencies.Gate, owner, repository, pr_gate.Event{
			Action: "synchronize",
			Number: event.Number,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// IssueEventService coordinates contribution policies for issue events.
type IssueEventService struct {
	Issues       *github_issue.Service
	Repositories *github_repository.Service
}

func (s IssueEventService) Run(owner, repository string, data []byte) error {
	return runIssueEvent(issueEventDependencies{
		Assignment: s.Issues,
		Triage: issue_triage.Dependencies{
			Issues:       s.Issues,
			Repositories: s.Repositories,
		},
	}, owner, repository, data)
}

type issueEventDependencies struct {
	Assignment issue_assignment.HelpWantedRemover
	Triage     issue_triage.Dependencies
}

func runIssueEvent(dependencies issueEventDependencies, owner, repository string, data []byte) error {
	event, err := github_issue.ParseEvent(data)
	if err != nil {
		return err
	}
	switch event.Action {
	case "opened":
		return issue_triage.Execute(dependencies.Triage, owner, repository, &event.Issue)
	case "assigned":
		return issue_assignment.Execute(dependencies.Assignment, owner, repository, &event.Issue)
	default:
		return nil
	}
}

// MergeGroupEventService coordinates merge queue policy for merge group events.
type MergeGroupEventService struct {
	PullRequests *github_pr.Service
	Repositories *github_repository.Service
	MergeGroups  *github_merge_group.Service
}

func (s MergeGroupEventService) Execute(owner, repository string, data []byte) error {
	return runMergeGroupEvent(mergeGroupEventDependencies{
		Gate: merge_gate.Dependencies{
			MergeGroups:  s.MergeGroups,
			PullRequests: s.PullRequests,
			Repositories: s.Repositories,
		},
	}, owner, repository, data)
}

type mergeGroupEventDependencies struct {
	Gate merge_gate.Dependencies
}

func runMergeGroupEvent(dependencies mergeGroupEventDependencies, owner, repository string, data []byte) error {
	event, err := github_merge_group.ParseEvent(data)
	if err != nil {
		return err
	}
	runID, _ := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), 10, 64)
	return merge_gate.Execute(dependencies.Gate, owner, repository, merge_gate.Event{
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
