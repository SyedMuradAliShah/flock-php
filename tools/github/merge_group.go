package main

import (
	"os"
	"strconv"

	"flock/tools/github/github_merge_group"
	"flock/tools/github/merge_gate"
)

// MergeGroupEventService coordinates merge queue policy for merge group events.
type MergeGroupEventService struct {
	Gate merge_gate.Dependencies
}

func (s MergeGroupEventService) Run(owner, repository string, data []byte) error {
	event, err := github_merge_group.ParseEvent(data)
	if err != nil {
		return err
	}
	runID, _ := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), 10, 64)
	return merge_gate.Execute(s.Gate, owner, repository, merge_gate.Event{
		HeadRef: event.HeadRef,
		HeadSHA: event.HeadSHA,
		RunID:   runID,
	})
}
