# 受け入れ確認（PRD 11章）

PRD（`docs/prd.md`）11章の受け入れ条件と主なフラグを、GitHub 上に作った fixture リポジトリで実際に動かして確認した記録です。fixture は使い回せるので、番号と役割を下にまとめます。

- 実施日: 2026-10-15
- バイナリ: `go build -o /tmp/ght-acc .`（作業ツリーの最新で再ビルドしてから全項目を流し直した。下の出力は再ビルド後のもの。出力中の `gh tree` は `/tmp/ght-acc` のこと）
- 実行方法: 特記がなければ `GH_REPO=ohzw/gh-tree-fixture`。あわせて、`GH_REPO` を外してローカルの clone（`/tmp/gh-tree-fixture`、`/tmp/gh-tree-fixture-other`）の中からも実行した
- 色の確認: 実行環境が `NO_COLOR=1` と `TERM=dumb` を export していたので、色の項目は `env -u NO_COLOR TERM=xterm-256color script -q /dev/null …` で TTY を用意して確認した

## 結果のまとめ

| # | 受け入れ条件 | 結果 |
|---|---|---|
| 1 | `issue` で Sub-issue の tree・状態・紐づいた PR・完了数が色付きで出る | PASS |
| 2 | 手で紐づけた PR、closing keyword の PR、merge なしで close された PR が出る | PARTIAL（手で紐づけた PR は API で作れず、未確認） |
| 3 | 別のリポジトリの Sub-issue が `owner/repo#N` で出る | PASS |
| 4 | `pr` で base / head の tree、Epic PR のない Epic branch が独立した根、fork は親にならない | PARTIAL（fork が親にならない点は実データで再現できず、unit test だけで担保） |
| 5 | `branch epic/…` でその branch を根にした PR tree | PASS |
| 6 | `pr <N>` で根の branch までの経路と、その PR より下の木だけ | PASS |
| 7 | `gh tree <N>` で Issue か PR かを判定 | PASS |
| 8 | `--json` が tree 表示と同じ情報・同じ入れ子で `schemaVersion` を含む | PASS |
| 9 | `--no-color` と `NO_COLOR` で色が消える | PASS |
| 10 | 2つ以上のリポジトリで設定なしで動く | PASS |

PASS 8、PARTIAL 2、FAIL 0。追加で確認したフラグ（`--depth`、`--hide-closed`、`--labels`、`--issues`、`--state merged|all`、複数の根、引数なしのヘルプ、エラー時の表示）は、すべて PRD どおりに動いた。gh-tree の不具合は見つからなかった。ただし PRD の前提に影響する GitHub 側の挙動を2つ見つけた（「所見」の F1、F2）。

## fixture の一覧

### ohzw/gh-tree-fixture（private、既定 branch `main`）

ラベル: `epic`、`analytics`（`bug` は既定で存在）

| 番号 | 種類 | 役割 | 状態 |
|---|---|---|---|
| #1 | Issue | Epic（根）。ラベル `epic` | open。子 8件、完了 2/8 |
| #2 | Issue | #1 の子。merge 済み PR #21 が紐づく。ラベル `analytics` | closed (completed) |
| #3 | Issue | #1 の子。孫を持つ。open の PR #13 が紐づく。ラベル `analytics`,`bug` | open。完了 1/2 |
| #4 | Issue | #1 の子。draft の PR #14 が紐づく | open |
| #5 | Issue | #1 の子。一度 close してから reopen | open (reopened) |
| #6 | Issue | #1 の子。#2 の重複として close | closed (duplicate) |
| #7 | Issue | #3 の子（2段目）。`main` 向けの PR #20 が `Fixes #7` で紐づく | open。完了 0/1 |
| #8 | Issue | #7 の子（3段目） | open |
| #9 | Issue | #3 の子（2段目） | closed (not planned) |
| #10 | Issue | #1 の子。merge なしで close された PR #15 が紐づく | open |
| #11 | Issue | #1 の子。手での紐づけ用。`createLinkedBranch` で branch `feature/11-manual-link` を紐づけ済み（PR #19 は紐づいていない。F2） | open |
| other#1 | Issue | #1 の子（別リポジトリ `ohzw/gh-tree-fixture-other#1`） | open |
| #12 | PR | `feature/2-funnel → epic/1-telemetry`、本文 `Fixes #2`。merge 済みで head branch は削除済み。base が既定 branch でないので **#2 に紐づいていない**（F1 の証拠） | merged |
| #13 | PR | `feature/3-unify-events → epic/1-telemetry`、`Fixes #3` | open |
| #14 | PR | `feature/4-reward-revert → epic/1-telemetry`、`Fixes #4` | draft |
| #15 | PR | `feature/10-retention → epic/1-telemetry`、`Fixes #10` | closed（merge なし） |
| #16 | PR | stack の1段目 `stack/a → develop` | open |
| #17 | PR | stack の2段目 `stack/b → stack/a` | open |
| #18 | PR | stack の3段目 `stack/c → stack/b` | open |
| #19 | PR | `feature/11-manual-link → epic/1-telemetry`。closing keyword なし。#11 に紐づけた branch から出した PR | open |
| #20 | PR | `feature/7-schema → main`、`Fixes #7`（既定 branch 向けなので keyword がそのまま効く） | open |
| #21 | PR | `feature/2-funnel-v2 → epic/1-telemetry`、`Fixes #2`。merge 済みで head branch は削除済み | merged |

branch: `main`（既定）、`develop`（`main` から）、`epic/1-telemetry`（`develop` から。Epic PR なし）、`feature/*`（`epic/1-telemetry` から）、`stack/a`（`develop` から）→ `stack/b` → `stack/c`。

#16 → #17 → #18 は、2026-10-17 に Stacks API（`POST /repos/{owner}/{repo}/stacks`）で stack #22（base `develop`）として登録した。GraphQL の `PullRequest.stack` は `{number: 22, size: 3, baseRefName: "develop"}`、`stackEntry.position` は #16 から順に 1、2、3。登録の直後は数秒だけ `null` が返った。ほかの PR はどの stack にも属さない。

#13、#14、#15、#21 の紐づけは、いったん base を `main` にして keyword を効かせ、その後で base を `epic/1-telemetry` に戻して作った。base を戻しても紐づけは残る（`closingIssuesReferences` で確認）。#15 は reopen → base 変更 → close、#21 は `main` 向けで作成 → base 変更 → merge の順。

### ohzw/gh-tree-fixture-other（private、既定 branch `main`）

| 番号 | 種類 | 役割 | 状態 |
|---|---|---|---|
| #1 | Issue | `ohzw/gh-tree-fixture#1` の Sub-issue（別リポジトリの子） | open |
| #2 | PR | `feature/1-sdk-bump → main`、`Fixes #1` | open |

ローカルの clone: `/tmp/gh-tree-fixture`、`/tmp/gh-tree-fixture-other`（作り直すときは `gh repo clone`）。

## 受け入れ条件ごとの記録

### 1. `gh tree issue` — PASS

```
$ gh tree issue 1
#1 [Epic] Telemetry rebuild                                ● open                  2/8
├─ #2 Progression funnel                                   ✓ closed (completed)
│  └─ PR #21 Progression funnel v2                         merged                  feature/2-funnel-v2 → epic/1-telemetry
├─ #3 Unify delete and wait events                         ● open                  1/2
│  ├─ PR #13 Unify delete and wait events                  open                    feature/3-unify-events → epic/1-telemetry
│  ├─ #7 Delete event schema                               ● open                  0/1
│  │  ├─ PR #20 Delete event schema                        open                    feature/7-schema → main
│  │  └─ #8 Delete event migration script                  ● open
│  └─ #9 Wait event sampling                               ⊘ closed (not planned)
├─ #4 MatchBricksReward sender revert                      ● open
│  └─ PR #14 MatchBricksReward sender revert               draft                   feature/4-reward-revert → epic/1-telemetry
├─ #5 KO effect equipment tracking                         ● open (reopened)
├─ #6 Progression funnel (dup)                             ⊘ closed (duplicate)
├─ #10 Retention dashboard                                 ● open
│  └─ PR #15 Retention dashboard                           closed                  feature/10-retention → epic/1-telemetry
├─ #11 Manual link target                                  ● open
└─ ohzw/gh-tree-fixture-other#1 Shared telemetry SDK bump  ● open
   └─ PR ohzw/gh-tree-fixture-other#2 Bump telemetry SDK   open                    feature/1-sdk-bump → main
```

- 4段（#1 → #3 → #7 → #8）をたどれている。closed の理由（completed / not planned / duplicate）と `open (reopened)` が出る。
- 完了数は `subIssuesSummary` のとおり。#1 は 2/8（#2 completed と #6 duplicate を完了に数える）、#3 は 1/2（#9 not planned を完了に数える）。
- 紐づいた PR は、Issue の子の行として子の Issue より先に並ぶ（#3 の下で PR #13 が #7 より先）。
- 色（TTY で確認。SGR コード）: Issue の open は緑 `32`、closed (completed) は紫 `35`、closed (not planned / duplicate) は灰 `90`。PR の open は緑 `32`、draft は灰 `90`、merged は紫 `35`、closed は赤 `31`。罫線は灰 `90`。抜粋:

```
$ env -u NO_COLOR TERM=xterm-256color script -q /dev/null gh tree issue 1 | cat -v
#1 [Epic] Telemetry rebuild   ^[[32m● open^[[0m   2/8
├─ #2 Progression funnel      ^[[35m✓ closed (completed)^[[0m
│  └─ PR #21 …                ^[[35mmerged^[[0m   feature/2-funnel-v2 → epic/1-telemetry
│  └─ #9 Wait event sampling  ^[[90m⊘ closed (not planned)^[[0m
│  └─ PR #14 …                ^[[90mdraft^[[0m    feature/4-reward-revert → epic/1-telemetry
$ env -u NO_COLOR TERM=xterm-256color script -q /dev/null gh tree pr --state all | cat -v
├─ PR #15 Retention dashboard ^[[31mclosed^[[0m   feature/10-retention → epic/1-telemetry
```
（罫線の `^[[90m…^[[0m` と UTF-8 のバイト表記は省いた）

### 2. 紐づいた PR の種類 — PARTIAL

| 種類 | fixture | 結果 |
|---|---|---|
| closing keyword | #3 ← PR #13（open）、#7 ← PR #20（open, `main` 向け）、#4 ← PR #14（draft）、#2 ← PR #21（merged） | 条件1の出力のとおり出る。PASS |
| merge なしで close | #10 ← PR #15 | `PR #15 Retention dashboard  closed` が出る。PASS |
| 手で紐づけた PR | #11 ← PR #19 を想定 | 未確認。公開 API で PR を Issue の Development 欄に紐づける手段がない（F2）。`gh tree issue 11` は `#11 Manual link target  ● open` だけを出し、これは API の返す内容（紐づきなし）と一致する |

手で紐づけた PR を確認するには、Web UI で #11 の Development 欄から PR #19 を紐づけ、`GH_REPO=ohzw/gh-tree-fixture gh tree issue 11` に `PR #19 Manual link PR  open  feature/11-manual-link → epic/1-telemetry` が出ることを見る。

### 3. 別のリポジトリの Sub-issue — PASS

条件1の出力の末尾。`ohzw/gh-tree-fixture-other#1` と、その Issue に紐づいた `PR ohzw/gh-tree-fixture-other#2` が `owner/repo#N` の形で出る。

### 4. `gh tree pr` — PARTIAL

```
$ gh tree pr
develop                                    （PR なし）
└─ PR #16 Stack A                          open         stack/a → develop
   └─ PR #17 Stack B                       open         stack/b → stack/a
      └─ PR #18 Stack C                    open         stack/c → stack/b
epic/1-telemetry                           （PR なし）
├─ PR #13 Unify delete and wait events     open         feature/3-unify-events → epic/1-telemetry
├─ PR #14 MatchBricksReward sender revert  draft        feature/4-reward-revert → epic/1-telemetry
└─ PR #19 Manual link PR                   open         feature/11-manual-link → epic/1-telemetry
main
└─ PR #20 Delete event schema              open         feature/7-schema → main
```

- stack（#16 → #17 → #18）が base と head でつながる。Epic PR のない `epic/1-telemetry` は独立した根になる。PASS。
- 既定 branch（`main`）の節点には `（PR なし）` が付かず、それ以外の PR のない branch には付く。PRD 7章の例（`develop` に注記なし）と同じ扱い。
- fork の PR: 個人アカウント1つでは自分の private リポジトリを fork できないため、fixture では作れなかった。公開リポジトリ `cli/go-gh` で、fork の head が `owner:branch` と出ることは確認した（open の PR のうち fork 15件、子を持つ fork の PR 0件）。「fork の head 名が base と同じでも親にならない」点は実データで再現できず、`internal/build/build_test.go` の `fork head named like the base never becomes a parent` だけで担保している。

```
$ GH_REPO=cli/go-gh gh tree pr
trunk
├─ PR #177 Support filtering before applying template                    open  heaths:issue8415 → trunk
├─ PR #209 config: allow overriding data directory via `GH_DATA_DIR`     open  amaanq:gh-data-dir → trunk
├─ PR #298 feat(config): add in-place Reload for refreshed reads         open  babakks/add-refresh-token-suppo… → trunk
…（20件。表示 1.07 秒）
```

### 5. `gh tree branch epic/…` — PASS

```
$ gh tree branch epic/1-telemetry --issues
epic/1-telemetry                           （PR なし）
├─ PR #13 Unify delete and wait events     open         feature/3-unify-events → epic/1-telemetry
│  └─ #3 Unify delete and wait events      ● open
├─ PR #14 MatchBricksReward sender revert  draft        feature/4-reward-revert → epic/1-telemetry
│  └─ #4 MatchBricksReward sender revert   ● open
└─ PR #19 Manual link PR                   open         feature/11-manual-link → epic/1-telemetry

$ gh tree branch develop
develop                  （PR なし）
└─ PR #16 Stack A        open         stack/a → develop
   └─ PR #17 Stack B     open         stack/b → stack/a
      └─ PR #18 Stack C  open         stack/c → stack/b
```

`develop` の tree に `epic/1-telemetry` は出ない（PRD 7章「Epic PR がない branch は独立した根」のとおり）。

### 6. `gh tree pr <N>` — PASS

```
$ gh tree pr 17
develop                  （PR なし）
└─ PR #16 Stack A        open         stack/a → develop
   └─ PR #17 Stack B     open         stack/b → stack/a
      └─ PR #18 Stack C  open         stack/c → stack/b

$ gh tree pr 13
epic/1-telemetry                        （PR なし）
└─ PR #13 Unify delete and wait events  open         feature/3-unify-events → epic/1-telemetry

$ gh tree pr 12
epic/1-telemetry              （PR なし）
└─ PR #12 Progression funnel  merged       feature/2-funnel → epic/1-telemetry
```

- #17 は根まで（develop → #16）と、下の #18 を出す。#13 では兄弟の #14、#19 を出さない。
- `--state` の外にある merged の #12（head branch は削除済み）も、単独で取得して正しい位置に置く。

### 7. 番号から自動判定 — PASS

```
$ gh tree 13
epic/1-telemetry                        （PR なし）
└─ PR #13 Unify delete and wait events  open         feature/3-unify-events → epic/1-telemetry

$ gh tree 1 --depth 1
#1 [Epic] Telemetry rebuild                                ● open                2/8
├─ #2 Progression funnel                                   ✓ closed (completed)
├─ #3 Unify delete and wait events                         ● open                1/2
…
└─ ohzw/gh-tree-fixture-other#1 Shared telemetry SDK bump  ● open

$ gh tree 999
gh-tree: issue or pull request #999 not found in ohzw/gh-tree-fixture
[exit=1]
```

### 8. `--json` — PASS

```
$ gh tree issue 1 --json --depth 2 --labels epic
{
  "schemaVersion": 1,
  "repo": "ohzw/gh-tree-fixture",
  "roots": [
    {
      "kind": "issue",
      "repo": "ohzw/gh-tree-fixture",
      "number": 1,
      "title": "[Epic] Telemetry rebuild",
      "url": "https://github.com/ohzw/gh-tree-fixture/issues/1",
      "state": "OPEN",
      "labels": ["epic"],
      "progress": {"completed": 2, "total": 8},
      "children": [
        {
          "kind": "issue", "number": 2, "state": "CLOSED", "stateReason": "COMPLETED",
          "children": [
            {"kind": "pr", "number": 21, "state": "MERGED",
             "base": "epic/1-telemetry", "head": "feature/2-funnel-v2", "children": []}
…
```

JSON の節点を jq で深さ付きに平らにし、tree 表示の行と突き合わせた。行数（JSON は節点数、`hiddenClosed` > 0 の節点は畳んだ行を1行足す）と並びと入れ子が、すべてのコマンドで一致した。

| コマンド | tree の行数 | JSON の節点数 | `hiddenClosed` の行 |
|---|---|---|---|
| `issue 1` | 18 | 18 | 0 |
| `issue 1 --hide-closed` | 16 | 14 | 2 |
| `issue 1 --depth 1` | 9 | 9 | 0 |
| `issue 3 7` | 9 | 9 | 0 |
| `pr` | 10 | 10 | 0 |
| `pr --state all --issues` | 18 | 18 | 0 |
| `pr 17` | 4 | 4 | 0 |
| `branch epic/1-telemetry --issues` | 6 | 6 | 0 |
| `13` | 2 | 2 | 0 |

`--hide-closed` の JSON では、#1 が `hiddenClosed: 2`、#3 が `hiddenClosed: 1` を持つ。draft の #14 は `state: "OPEN"` と `isDraft: true`。

### 9. `--no-color` / `NO_COLOR` — PASS

ESC を含む行の数（`gh tree issue 1`、全18行）:

| 実行条件 | ESC を含む行 |
|---|---|
| TTY、色の指定なし | 18 |
| TTY、`--no-color` | 0 |
| TTY、`NO_COLOR=1` | 0 |
| パイプ（TTY でない） | 0 |

### 10. 2つ以上のリポジトリで設定なしに動く — PASS

`GH_REPO` を外して、各 clone の中から実行した。

```
$ cd /tmp/gh-tree-fixture && gh tree pr        # 条件4と同じ出力
$ cd /tmp/gh-tree-fixture && gh tree 18        # 条件6の pr 17 と同じ stack
$ cd /tmp/gh-tree-fixture-other && gh tree 1
#1 Shared telemetry SDK bump  ● open
└─ PR #2 Bump telemetry SDK   open    feature/1-sdk-bump → main
$ cd /tmp/gh-tree-fixture-other && gh tree pr --issues
main
└─ PR #2 Bump telemetry SDK         open    feature/1-sdk-bump → main
   └─ #1 Shared telemetry SDK bump  ● open
$ GH_REPO=cli/go-gh gh tree pr                 # 条件4。公開リポジトリでも動く
```

リポジトリを決められないときは、理由と対処を表示して exit 1 で終わる。

```
$ cd /tmp && gh tree pr
gh-tree: could not determine the repository (run inside a GitHub repository or set GH_REPO=owner/name): failed to run git: fatal: not a git repository (or any of the parent directories): .git
. error: exit status 128
$ cd ~/Documents/Github/gh-tree && gh tree pr    # remote 未設定
gh-tree: could not determine the repository (run inside a GitHub repository or set GH_REPO=owner/name): unable to determine current repository, no git remotes configured for this repository
```

## フラグの確認（すべて PASS）

`--depth`（根が 0、紐づいた PR の行も1段）:

```
$ gh tree issue 1 --depth 0
#1 [Epic] Telemetry rebuild  ● open  2/8

$ gh tree issue 1 --depth 2      # #7 は出るが、3段目の PR #20 と #8 は出ない
├─ #3 Unify delete and wait events                         ● open                  1/2
│  ├─ PR #13 Unify delete and wait events                  open                    feature/3-unify-events → epic/1-telemetry
│  ├─ #7 Delete event schema                               ● open                  0/1
│  └─ #9 Wait event sampling                               ⊘ closed (not planned)
```

`--depth 1` は直下の子だけ（PR の行も出ない。条件7の出力）。`-d 3` で全体が出る（条件1と同じ）。

`--hide-closed`:

```
$ gh tree issue 1 --hide-closed
#1 [Epic] Telemetry rebuild                                ● open             2/8
├─ #3 Unify delete and wait events                         ● open             1/2
│  ├─ PR #13 Unify delete and wait events                  open               feature/3-unify-events → epic/1-telemetry
│  ├─ #7 Delete event schema                               ● open             0/1
│  │  ├─ PR #20 Delete event schema                        open               feature/7-schema → main
│  │  └─ #8 Delete event migration script                  ● open
│  └─ … closed 1件
├─ #4 MatchBricksReward sender revert                      ● open
…
├─ ohzw/gh-tree-fixture-other#1 Shared telemetry SDK bump  ● open
│  └─ PR ohzw/gh-tree-fixture-other#2 Bump telemetry SDK   open               feature/1-sdk-bump → main
└─ … closed 2件
```

`--labels`（表示だけ。絞り込みはしない）:

```
$ gh tree issue 1 --labels epic,analytics,bug --depth 1
#1 [Epic] Telemetry rebuild [epic]                         ● open                2/8
├─ #2 Progression funnel [analytics]                       ✓ closed (completed)
├─ #3 Unify delete and wait events [bug] [analytics]       ● open                1/2
├─ #4 MatchBricksReward sender revert                      ● open
…
```

`--state merged|all`:

```
$ gh tree pr --state merged
epic/1-telemetry                 （PR なし）
├─ PR #12 Progression funnel     merged       feature/2-funnel → epic/1-telemetry
└─ PR #21 Progression funnel v2  merged       feature/2-funnel-v2 → epic/1-telemetry

$ gh tree pr --state all          # open・draft・merged・closed がすべて出る
epic/1-telemetry                           （PR なし）
├─ PR #12 Progression funnel               merged       feature/2-funnel → epic/1-telemetry
├─ PR #13 Unify delete and wait events     open         feature/3-unify-events → epic/1-telemetry
├─ PR #14 MatchBricksReward sender revert  draft        feature/4-reward-revert → epic/1-telemetry
├─ PR #15 Retention dashboard              closed       feature/10-retention → epic/1-telemetry
├─ PR #19 Manual link PR                   open         feature/11-manual-link → epic/1-telemetry
└─ PR #21 Progression funnel v2            merged       feature/2-funnel-v2 → epic/1-telemetry
…（develop と main の木は `pr` と同じ）

$ gh tree pr --state bogus
gh-tree: invalid --state "bogus": want open, merged, or all
[exit=1]
```

`--issues`:

```
$ gh tree pr --state merged --issues
epic/1-telemetry                 （PR なし）
├─ PR #12 Progression funnel     merged                feature/2-funnel → epic/1-telemetry
└─ PR #21 Progression funnel v2  merged                feature/2-funnel-v2 → epic/1-telemetry
   └─ #2 Progression funnel      ✓ closed (completed)
```

`pr --issues` と `branch … --issues` は条件4・5を参照。

複数の根:

```
$ gh tree issue 3 7
#3 Unify delete and wait events         ● open                  1/2
├─ PR #13 Unify delete and wait events  open                    feature/3-unify-events → epic/1-telemetry
├─ #7 Delete event schema               ● open                  0/1
│  ├─ PR #20 Delete event schema        open                    feature/7-schema → main
│  └─ #8 Delete event migration script  ● open
└─ #9 Wait event sampling               ⊘ closed (not planned)
#7 Delete event schema                  ● open                  0/1
├─ PR #20 Delete event schema           open                    feature/7-schema → main
└─ #8 Delete event migration script     ● open
```

`gh tree pr 13 18` も、2つの木を引数の順に並べる。

引数なし:

```
$ gh tree
Show GitHub issues (sub-issues and linked pull requests), pull requests
(stacked by base and head branch), and branches as a colored tree.
…
Usage:
  gh tree [<number>] [flags]
  gh tree [command]
[exit=0]
```

エラー時の表示:

```
$ gh tree issue abc
gh-tree: invalid number "abc"
$ gh tree issue 13
gh-tree: issue #13 not found in ohzw/gh-tree-fixture
$ gh tree pr 1
gh-tree: pull request #1 not found in ohzw/gh-tree-fixture
$ gh tree branch nope
gh-tree: branch "nope" not found: no pull request targets it
```

## stack の印（2026-10-18 に追加で確認）

上の記録より後に入れた機能なので、作業ツリーの最新で `/tmp/ght-acc` を再ビルドして確認した。fixture の stack #22（#16 → #17 → #18）を使う。

```
$ gh tree pr 17 --no-icons
develop                  （PR なし）
└─ PR #16 Stack A        open         stack/a  ⧉ 1/3
   └─ PR #17 Stack B     open         stack/b  ⧉ 2/3
      └─ PR #18 Stack C  open         stack/c  ⧉ 3/3

$ gh tree pr --no-icons        # stack に属さない PR には印が付かない
develop                                    （PR なし）
└─ PR #16 Stack A                          open         stack/a                  ⧉ 1/3
   └─ PR #17 Stack B                       open         stack/b                  ⧉ 2/3
      └─ PR #18 Stack C                    open         stack/c                  ⧉ 3/3
epic/1-telemetry                           （PR なし）
├─ PR #13 Unify delete and wait events     open         feature/3-unify-events
├─ PR #14 MatchBricksReward sender revert  draft        feature/4-reward-revert
└─ PR #19 Manual link PR                   open         feature/11-manual-link
main
└─ PR #20 Delete event schema              open         feature/7-schema

$ gh tree pr 17 --no-icons     # 幅 36 のターミナル。head と一緒に印も下の行に移る
develop                  （PR なし）
└─ PR #16 Stack A        open
   │    stack/a  ⧉ 1/3
   └─ PR #17 Stack B     open
      │    stack/b  ⧉ 2/3
      └─ PR #18 Stack C  open
              stack/c  ⧉ 3/3
```

- 位置と本数は `stackEntry.position` と `stack.size` のとおり。印は head の後ろで1列にそろう。
- アイコン表示では `⧉` の代わりに Octicons の versions が出る。
- `--json` では、#16〜#18 が `"stack": {"number": 22, "size": 3, "position": 1〜3, "base": "develop"}` を持ち、stack に属さない #13 には `stack` がない。

## 性能（参考）

`/usr/bin/time -p` の real。fixture の規模は小さい。

| コマンド | 秒 |
|---|---|
| `issue 1`（4段、子 8件） | 1.94 |
| `pr` | 0.61 |
| `pr --state all` | 0.65 |
| `13`（自動判定 + PR の取得） | 1.30 |
| `branch epic/1-telemetry --issues` | 0.71 |
| `GH_REPO=cli/go-gh pr`（open 20件） | 1.07 |

## 所見

### F1. 既定 branch 以外を base にした PR では closing keyword が紐づかない（GitHub の挙動、PRD への影響あり）

- 手順: PR #12（`feature/2-funnel → epic/1-telemetry`、本文 `Fixes #2`）を作成して merge した。
- 結果: PR #12 の `closingIssuesReferences` は空で、#2 の `closedByPullRequestsReferences(includeClosedPrs:true)` にも出ない。PR #13 も、`epic/1-telemetry` 向けで作った直後は空だった。base を `main` に変えると `[3]` になり、`epic/1-telemetry` に戻しても `[3]` のまま残った。
- 影響: PRD 7章の例は、Epic branch 向けの feature PR が Issue に紐づいている前提になっている。実際には、Epic branch 向けの PR は本文に `Fixes #N` を書いても紐づかない。Web UI で手で紐づけるか、上のように一度既定 branch に向ける必要がある。gh-tree は API が返すものをそのまま出しているので、不具合ではない。README か PRD に、この GitHub の仕様を書いておくとよい。

### F2. PR を Issue に手で紐づける公開 API がない

- GraphQL の mutation のうち、紐づけに関係するのは `createLinkedBranch` と `deleteLinkedBranch` だけ（スキーマを検索して確認）。
- `createLinkedBranch` で #11 に `feature/11-manual-link` を紐づけ、その branch から PR #19 を出した。#11 の `linkedBranches` には branch が出るが、`closedByPullRequestsReferences` は `userLinkedOnly:true` でも `false` でも空。
- そのため、受け入れ条件2の「手で紐づけた PR」は fixture を作るスクリプトでは用意できない。確認するときは、Web UI で #11 に PR #19 を紐づける（条件2の手順）。

### O1. `gh tree branch <PR の head の branch>` はエラーになる（仕様の確認待ち）

```
$ gh tree branch stack/a
gh-tree: branch "stack/a" is the head of PR #16; use `gh tree pr 16`
[exit=1]
```

PR #17 は `stack/a` を base にしているので、`stack/a` を根にした木（#17 → #18）を出す読み方もできる。PRD 5章は「指定した branch を根にした PR tree」とだけ書いていて、PR の head になっている branch の扱いは決めていない。実装は案内付きのエラーにしている。11章の受け入れ条件には含まれないので、判定には入れていない。

### O2. `--labels` の並び順は API の順

`--labels epic,analytics,bug` で、#3 は `[bug] [analytics]` と出る（フラグに書いた順ではない）。PRD は順序を決めていないので、判定には入れていない。
