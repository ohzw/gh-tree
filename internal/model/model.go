// Package model holds the types shared by fetching, building, and rendering.
//
// Node and Document are the published JSON contract (docs/prd.md 10章).
// Renaming or removing a JSON field requires bumping SchemaVersion.
package model

import "time"

const SchemaVersion = 1

type Kind string

const (
	KindIssue  Kind = "issue"
	KindPR     Kind = "pr"
	KindBranch Kind = "branch"
)

// Document is the top-level --json output.
type Document struct {
	SchemaVersion int     `json:"schemaVersion"`
	Repo          string  `json:"repo"`
	Roots         []*Node `json:"roots"`
}

// Node is one element of a rendered tree. Children are in display order.
type Node struct {
	Kind Kind `json:"kind"`

	// Repo is "owner/name" of the issue or PR. Empty for branch nodes.
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`

	// State is the GitHub state: issue OPEN|CLOSED, PR OPEN|MERGED|CLOSED.
	State string `json:"state,omitempty"`
	// StateReason is the issue state reason: COMPLETED|NOT_PLANNED|DUPLICATE|REOPENED.
	StateReason string `json:"stateReason,omitempty"`
	IsDraft     bool   `json:"isDraft,omitempty"`

	// Base and Head are PR branch names. HeadOwner is set only for
	// cross-repository (fork) PRs and is displayed as "owner:branch".
	Base      string `json:"base,omitempty"`
	Head      string `json:"head,omitempty"`
	HeadOwner string `json:"headOwner,omitempty"`

	// Name is the branch name of a branch node.
	Name string `json:"name,omitempty"`

	// Labels are only the labels selected with --labels.
	Labels []string `json:"labels,omitempty"`

	// Progress is the direct sub-issue completion of an issue
	// (subIssuesSummary.completed / total). Nil when the issue has no sub-issues.
	Progress *Progress `json:"progress,omitempty"`

	// HiddenClosed counts closed sub-issues folded by --hide-closed.
	HiddenClosed int `json:"hiddenClosed,omitempty"`

	Children []*Node `json:"children"`
}

type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// IssueRef is an issue as seen from a PR's closingIssuesReferences or as the
// scalar part of a fetched Issue.
type IssueRef struct {
	Repo        string
	Number      int
	Title       string
	URL         string
	State       string
	StateReason string
	Labels      []string
}

// Issue is a fully fetched issue tree: every sub-issue is loaded to the
// requested depth, with pagination already resolved.
type Issue struct {
	IssueRef
	// Progress is nil when the issue has no sub-issues.
	Progress *Progress
	// LinkedPRs are closedByPullRequestsReferences(userLinkedOnly:false, includeClosedPrs:true).
	LinkedPRs []PR
	SubIssues []*Issue
}

// PR is a fetched pull request.
type PR struct {
	Repo              string
	Number            int
	Title             string
	URL               string
	State             string // OPEN|MERGED|CLOSED
	IsDraft           bool
	Base              string // baseRefName
	Head              string // headRefName
	IsCrossRepository bool
	HeadOwner         string // headRepositoryOwner.login
	CreatedAt         time.Time
	Labels            []string
	ClosingIssues     []IssueRef
}
