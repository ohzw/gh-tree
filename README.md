# gh-tree

English | [日本語](README.ja.md)

A [gh](https://cli.github.com/) extension that shows how GitHub issues, pull requests, and branches relate to each other (sub-issue parents and children, the PRs linked to each issue, and which branch each PR merges into) as a tree in your terminal, along with the state of each one. It does not show the file tree of a repository.

![Output of gh tree issue 1: an epic's sub-issues and the PRs linked to each, shown as a tree with their states](docs/images/issue-tree.png)

## Installation

```sh
gh extension install ohzw/gh-tree
```

## Usage

```
gh tree issue <number>... [flags]   Issue tree (sub-issues and the PRs linked to each issue)
gh tree pr [<number>...] [flags]     PR tree (without numbers, every PR in the selected states)
gh tree branch <name> [flags]        PR tree rooted at the given branch
gh tree <number>                     Detect whether the number is an issue or a PR
gh tree                              Show help
```

| Flag | Applies to | Description |
|---|---|---|
| `--json` | all | Print JSON |
| `--no-color` | all | Disable colors (`NO_COLOR` is also respected) |
| `-d, --depth N` | all | How deep to follow the tree, counting the root as 0 (default: unlimited) |
| `--labels a,b` | all | Labels to show next to each node. Display only; they do not filter |
| `--no-icons` | all | Draw plain symbols (`●` `✓` `PR` and so on) instead of [Nerd Fonts](https://www.nerdfonts.com/) Octicons. For terminals without a Nerd Font |
| `--hide-closed` | issue | Collapse closed children into one line |
| `--no-ancestors` | issue | Do not show the path from the issue's ancestors |
| `--state open\|merged\|all` | pr / branch | Which PRs to show (default: open) |
| `--issues` | pr / branch | Show the issues linked to each PR as its children |

A PR becomes a child of the PR whose head has the same name as its base. A branch with no PR to attach to becomes a root node of its own, marked `（PR なし）` ("no PR") unless it is the default branch. When a PR's parent node is its base (most PRs in a PR tree), only its head is shown; otherwise (for example, PRs in an issue tree) `head → base` is shown.

![Output of gh tree pr --state all: stacked PRs connected through their base and head branches, with an epic branch as a root](docs/images/pr-tree.png)

The PRs linked to an issue are the same ones GitHub shows in the issue's Development section. GitHub does not treat `Fixes #N` in a PR as a link unless the PR targets the default branch, so to show a PR that targets an epic branch (or any other branch) in an issue tree, link it by hand in the Development section of the web UI.

Issue states are shown as `open`, `reopened`, `completed`, `closed` (no reason given), `not planned`, and `duplicate`. Sub-issue progress (`1/4`) appears next to the state.

In a terminal, issue and PR numbers and titles are links to GitHub (OSC 8): Cmd+click (Ctrl+click in some terminals) opens them. Terminals without hyperlink support show plain text. In tmux, set `set -as terminal-features ",*:hyperlinks"`. Output sent to a pipe or a file has no links.

When the tree does not fit the terminal width, nothing is dropped; lines are added instead. Titles first wrap within their column, then each PR's `head → base` moves to a line below the PR. A node whose column would still be too narrow shows its title across the full width with its state and details on the lines below (when most nodes are like that, every node takes this form). A node's continuation lines are indented deeper than its children, so they cannot be mistaken for children. Output sent to a pipe or a file is not wrapped.

When an issue has a parent, the path from the topmost ancestor down to the requested issue (marked `▶`) is shown above it. Children off that path are folded into a `他 N件` ("N more") line.

<img src="docs/images/narrow.png" width="530" alt="gh tree issue 3 in a 52-column terminal: the ancestor path, with wrapped titles and branches">

By default, states, PRs, and branches are drawn with Nerd Fonts icons. The examples below use `--no-icons` so they read anywhere.

```
$ gh tree issue 1244 --no-icons
#1244 [Epic] Rebuild telemetry          ● open 1/4
├─ #1245 Progression funnel for Bricks  ✓ completed
│  └─ PR #1248 Funnel                   merged       feature/1245 → epic/1244
└─ #1254 Unify delete and wait events   ● open
   └─ PR #1257 Unify delete             open         feature/1254 → epic/1244

$ gh tree issue 1244 --no-icons        # in a 40-column terminal
#1244 [Epic] Rebuild         ● open 1/4
│    telemetry
├─ #1245 Progression funnel for Bricks
│  │    ✓ completed
│  └─ PR #1248 Funnel        merged
│          feature/1245 → epic/1244
└─ #1254 Unify delete and wait events
   │    ● open
   └─ PR #1257 Unify delete  open
           feature/1254 → epic/1244

$ gh tree pr --no-icons
main
└─ PR #1260 Release 2.4   open         release/2.4
epic/1244                 （PR なし）
└─ PR #1257 Unify delete  open         feature/1254
```

## JSON

`--json` prints JSON nested the same way as the tree. The leading `schemaVersion` changes only when a field is renamed or changes shape. The fields are defined by `Document` and `Node` in [`internal/model/model.go`](internal/model/model.go).

```
$ GH_REPO=cli/cli gh tree pr 9847 --json
{
  "schemaVersion": 1,
  "repo": "cli/cli",
  "roots": [
    {
      "kind": "branch",
      "name": "trunk",
      "children": [
        {
          "kind": "pr",
          "repo": "cli/cli",
          "number": 9847,
          "title": "[gh search issues] Support multiple author options",
          "url": "https://github.com/cli/cli/pull/9847",
          "state": "OPEN",
          "isDraft": true,
          "base": "trunk",
          "head": "shion/9828",
          "headOwner": "Shion1305",
          "children": []
        }
      ]
    }
  ]
}
```

## Permissions

gh-tree uses your existing `gh` authentication. It needs the `repo` scope; if it is missing, add it with `gh auth refresh -s repo`.

## Target repository

gh-tree works on the git repository in the current directory. The `GH_REPO` environment variable (`owner/name`) takes precedence when set. There is no `-R` flag.

```sh
GH_REPO=cli/cli gh tree pr
```

## License

[MIT](LICENSE)
