package main

import (
	"errors"
	"testing"

	"flock/tools/github/merge_gate"
)

func TestMergeGroupEvent(t *testing.T) {
	api := &mergeGroupEventFake{}
	t.Setenv("GITHUB_RUN_ID", "7")
	if err := newMergeGroupEventService(api).Run("flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestMergeGroupEventRejectsInvalidPayloadAndUsesZeroForInvalidRunID(t *testing.T) {
	if err := newMergeGroupEventService(&mergeGroupEventFake{}).Run("flock-php", "flock", []byte(`{`)); err == nil {
		t.Fatal("Run() returned nil error")
	}

	validRunIDAPI := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "7")
	err := newMergeGroupEventService(validRunIDAPI).Run("flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
	if !errors.Is(err, merge_gate.ErrNotAllowed) || validRunIDAPI.cancelExcept != 7 {
		t.Fatalf("err = %v, except = %d", err, validRunIDAPI.cancelExcept)
	}

	api := &mergeGroupEventFake{role: "write"}
	t.Setenv("GITHUB_RUN_ID", "not-a-number")
	err = newMergeGroupEventService(api).Run("flock-php", "flock", []byte(`{"merge_group":{"head_ref":"refs/heads/gh-readonly-queue/main/pr-87-abc","head_sha":"abc"}}`))
	if !errors.Is(err, merge_gate.ErrNotAllowed) {
		t.Fatalf("err = %v, want ErrNotAllowed", err)
	}
	if !api.cancelled || api.cancelExcept != 0 {
		t.Fatalf("cancelled = %t, except = %d", api.cancelled, api.cancelExcept)
	}
}

type mergeGroupEventFake struct {
	role         string
	cancelled    bool
	cancelExcept int64
}

func newMergeGroupEventService(api *mergeGroupEventFake) MergeGroupEventService {
	return MergeGroupEventService{Gate: merge_gate.Dependencies{MergeGroups: api, PullRequests: api, Repositories: api}}
}
func (mergeGroupEventFake) GetMergeQueueEnqueuer(string, string, int) (string, error) {
	return "maintainer", nil
}
func (mergeGroupEventFake) CreateComment(string, string, int, string) error { return nil }
func (f *mergeGroupEventFake) GetUserRole(string, string, string) (string, error) {
	if f.role == "" {
		return "maintain", nil
	}
	return f.role, nil
}
func (f *mergeGroupEventFake) CancelOtherRuns(_ string, _ string, _ string, except int64) error {
	f.cancelled = true
	f.cancelExcept = except
	return nil
}
