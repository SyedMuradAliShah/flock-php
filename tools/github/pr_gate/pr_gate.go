// Package pr_gate applies Flock's pull request contribution policy.
package pr_gate

import (
	"fmt"
	"strings"
	"time"

	"flock/tools/github/github_issue"
	"flock/tools/github/github_pr"
	"flock/tools/github/issue_reference"
)

const openToAnyone = "help wanted"

type PullRequestGateService interface {
	Get(owner, repository string, number int) (*github_pr.PullRequest, error)
	ListFiles(owner, repository string, number int) ([]github_pr.File, error)
	GateClosed(owner, repository string, number int) (bool, error)
	CreateComment(owner, repository string, number int, body string) error
	SetState(owner, repository string, number int, state string) error
}

type IssueReader interface {
	Get(owner, repository string, number int) (*github_issue.Issue, error)
}

type Event struct {
	Action     string
	Number     int
	BodyEdited bool
	BaseEdited bool
}

type Result struct {
	ApprovedIssue *issue_reference.Reference
}

type action string

const (
	allow           action = "allow"
	closeWrongBase  action = "close-wrong-base"
	closeUnapproved action = "close-unapproved"
)

func Execute(pullRequests PullRequestGateService, issues IssueReader, owner, repository string, event Event) (Result, error) {
	for attempt := 0; ; attempt++ {
		pr, err := pullRequests.Get(owner, repository, event.Number)
		if err != nil {
			return Result{}, err
		}
		if pr == nil {
			return Result{}, nil
		}
		if bypassesGate(pr.Author, pr.AuthorAssociation) {
			return Result{}, nil
		}
		if pr.State == "closed" {
			if !event.BodyEdited || pr.Merged || pr.BaseRef != "main" {
				return Result{}, nil
			}
			closedByGate, err := pullRequests.GateClosed(owner, repository, pr.Number)
			if err != nil || !closedByGate {
				return Result{}, err
			}
			if issue, approved := firstApprovedIssue(issues, owner, pr); approved {
				if err := pullRequests.SetState(owner, repository, pr.Number, "open"); err != nil {
					return Result{}, err
				}
				if err := pullRequests.CreateComment(owner, repository, pr.Number, fmt.Sprintf("Thanks, @%s! This pull request references approved issue %s, so it has been reopened.", pr.Author.Login, issue)); err != nil {
					return Result{}, err
				}
				return approvedResult(issue), nil
			}
			return Result{}, nil
		}
		if event.Action == "edited" && !event.BaseEdited {
			return Result{}, nil
		}
		if pr.BaseRef != "main" {
			return Result{}, closePullRequest(pullRequests, owner, repository, pr, closeWrongBase)
		}
		if issue, approved := firstApprovedIssue(issues, owner, pr); approved {
			return approvedResult(issue), nil
		}
		files, err := pullRequests.ListFiles(owner, repository, pr.Number)
		if err != nil {
			return Result{}, err
		}
		decision := evaluate(files)
		if decision == allow {
			return Result{}, nil
		}
		if decision == closeUnapproved && event.Action == "opened" && attempt < 2 {
			time.Sleep(3 * time.Second)
			continue
		}
		return Result{}, closePullRequest(pullRequests, owner, repository, pr, decision)
	}
}

func approvedResult(issue issue_reference.Reference) Result {
	return Result{ApprovedIssue: &issue}
}

// CloseClaimedIssue closes a pull request whose approved issue was claimed after the gate read it.
func CloseClaimedIssue(pullRequests PullRequestGateService, owner, repository string, number int, issue issue_reference.Reference, claimant string) error {
	pullRequest, err := pullRequests.Get(owner, repository, number)
	if err != nil || pullRequest == nil || pullRequest.State != "open" {
		return err
	}
	comment := fmt.Sprintf("Thanks for the PR, @%s! Issue %s has already been claimed by @%s, so this pull request is closed. Please check other issues labeled `help wanted`.\n\n%s", pullRequest.Author.Login, issue, claimant, github_pr.GateMarker)
	if err := pullRequests.CreateComment(owner, repository, number, comment); err != nil {
		return err
	}
	return pullRequests.SetState(owner, repository, number, "closed")
}

func evaluate(files []github_pr.File) action {
	if docsOnly(files) {
		return allow
	}
	return closeUnapproved
}

func firstApprovedIssue(issues IssueReader, owner string, pr *github_pr.PullRequest) (issue_reference.Reference, bool) {
	for _, reference := range issue_reference.Parse(pr.Body, "flock") {
		issue, err := issues.Get(owner, reference.Repository, reference.Number)
		if err == nil && isApproved(issue, pr.Author.Login) {
			return reference, true
		}
	}
	return issue_reference.Reference{}, false
}

func closePullRequest(pullRequests PullRequestGateService, owner, repository string, pr *github_pr.PullRequest, decision action) error {
	var reason, nextStep string
	switch decision {
	case closeWrongBase:
		reason = fmt.Sprintf("Pull requests go into `main`, not `%s`, so this one is closed.", pr.BaseRef)
		nextStep = "Please open a new pull request against `main`."
	case closeUnapproved:
		reason = "Flock takes pull requests only for issues a maintainer has approved, so this one is closed."
		nextStep = "Edit this pull request's description to reference an approved issue, then save it to reopen automatically."
	default:
		return fmt.Errorf("cannot close a pull request for %q", decision)
	}
	comment := fmt.Sprintf("Thanks for the PR, @%s! %s %s See [CONTRIBUTING.md](https://github.com/%s/%s/blob/main/CONTRIBUTING.md).\n\n%s", pr.Author.Login, reason, nextStep, owner, repository, github_pr.GateMarker)
	if err := pullRequests.CreateComment(owner, repository, pr.Number, comment); err != nil {
		return err
	}
	return pullRequests.SetState(owner, repository, pr.Number, "closed")
}

func bypassesGate(user github_issue.User, authorAssociation string) bool {
	return user.Type == "Bot" || authorAssociation == "OWNER" || authorAssociation == "MEMBER" || authorAssociation == "COLLABORATOR"
}

func docsOnly(files []github_pr.File) bool {
	for _, file := range files {
		if !strings.HasPrefix(file.Filename, "docs/") || (file.PreviousFilename != "" && !strings.HasPrefix(file.PreviousFilename, "docs/")) {
			return false
		}
	}
	return true
}

func isApproved(issue *github_issue.Issue, author string) bool {
	if issue == nil || issue.IsPullRequest || issue.State != "open" {
		return false
	}
	for _, label := range issue.Labels {
		if label.Name == openToAnyone {
			return true
		}
	}
	for _, assignee := range issue.Assignees {
		if assignee.Login == author {
			return true
		}
	}
	return false
}
