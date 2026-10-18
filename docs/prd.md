# PRD: `gh-tree`

## 1. 背景
- Issue を親子（Sub-issue）関係で分けて進めると、件数が増えて全体の状態が追えなくなります。PR も Epic branch や stack で入れ子になり、どこに何を merge するのかが見えにくくなります。
- GitHub の Web UI には、Issue の親子、紐づいた PR、PR の merge 先の関係を1画面で見る手段がありません。

## 2. 目的
Issue・PR・branch の関係と、それぞれの GitHub 上の状態を、ターミナルに色付きの tree で1画面に表示します。このツールは状態をそのまま見せるだけで、状態の良し悪しは判定しません。判断は、見た人やエージェントに任せます。

## 3. 対象ユーザー
- GitHub Issues・PR で開発を管理している開発者や PM。特定のチームの運用を前提にしない、公開する汎用ツールとして作ります。
- エージェントにも使わせます。そのため JSON でも出力します。

## 4. 用語
| 用語 | 意味 |
|---|---|
| Issue tree | Sub-issue の親子関係でたどる tree |
| PR tree | PR の merge 先の branch（base）と元の branch（head）でつなぐ tree。base が別の PR の head と一致すれば、その PR の子とみなす |
| 紐づいた PR | Issue の Development 欄に出る PR。GitHub の判定をそのまま使い、手で紐づけた PR と、closing keyword（`Fixes #123` など）で紐づいた PR の両方を含む。merge されずに close された PR も含む。GitHub は default branch 以外を base にした PR の closing keyword を紐づけとして扱わないため、Epic branch 向けの PR は、Web UI で手で紐づけたものだけが出る。gh-tree は PR の本文を自分では解析しない |
| 状態 | GitHub が持つ Issue と PR の状態。Issue は open（reopen されたものは理由 reopened）/ closed（理由: completed・not planned・duplicate）、PR は open / draft / merged / closed |
| 節点 | tree の1要素。Issue、PR、branch（付け先に PR がない branch）のいずれか |

## 5. コマンドの形
```
gh tree issue <number>... [flags]   Issue tree（Sub-issue と、各 Issue に紐づいた PR）
gh tree pr [<number>...] [flags]     PR tree（番号を省けば、対象の状態の PR すべて）
gh tree branch <name> [flags]        指定した branch を根にした PR tree（例: develop、epic/#1244_…）
gh tree <number>                     番号から Issue か PR かを自動で判定する
gh tree                              ヘルプを表示する

共通:
      --json                JSON で出力する
      --no-color            色を付けない（NO_COLOR 環境変数にも従う）
  -d, --depth N             たどる深さ（既定: 無制限）
      --no-icons            Nerd Fonts の Octicons（nf-oct-*）をやめ、記号で描く（既定: Octicons で描く）。状態の語はどちらでも残す
      --labels a,b          節点の横に表示するラベル（既定: 表示しない）。表示する節点の絞り込みには使わない

issue:
      --hide-closed         closed の子を1行にまとめる（例: `└─ … closed 3件`）
      --no-ancestors        指定した Issue の親（祖先）を表示しない

pr / branch:
      --state open|merged|all   表示する PR（既定: open）。merged / all では期間を区切らずに取得する
      --issues                  各 PR に紐づいた Issue を、PR の子の行として表示する
```
- 対象のリポジトリは、カレントディレクトリの git リポジトリです。`gh` の慣習に従い、環境変数 `GH_REPO` があればそちらを優先します。`-R` は設けません。
- `gh tree issue 1 2 3` のように番号を複数渡したときは、根を並べて表示します。

## 6. スコープ

### MVP（v0.1）
| 機能 | 内容 |
|---|---|
| `gh tree issue` | Sub-issue を再帰的にたどる。各 Issue に、番号、タイトル、状態（open なら `open`、reopen された open なら `reopened`、closed なら理由の `completed`・`not planned`・`duplicate`、理由がなければ `closed`）、直下の子の完了数（例: `1/4`。状態の横に付ける）を表示する。完了数は `subIssuesSummary` の `completed / total` をそのまま使い、not planned と duplicate で閉じた子も完了に数える。紐づいた PR は、Issue の子の行として子の Issue より先に並べ、状態と head → base を表示する |
| リポジトリをまたぐ Sub-issue | 別のリポジトリの Issue と PR は `owner/repo#N` と表示する。表示だけで、追加の問い合わせは行わない |
| Issue の祖先 | 指定した Issue に親があれば、一番上の祖先を根にして、指定した Issue までの経路を既定で表示する。経路上の祖先には、ほかの子を出さずに件数だけ示す（例: `└─ … 他 7件`）。祖先の行には完了数を出し、紐づいた PR は出さない。指定した Issue には行頭に `▶` を付けて太字にする。`--depth` は指定した Issue から数える。`--no-ancestors` で消せる。PR を指定したときは、PR が閉じる Issue の親はたどらない |
| `gh tree pr` / `gh tree branch` | PR を base と head でつなぐ。付け先に PR がない branch（`develop` や、Epic PR がまだない Epic branch）は branch を節点にする。各 PR に、番号、タイトル、状態、head を表示する。親の節点が base なので base は出さない。親が base でない PR（循環を切って根に置いた PR など）には head → base を表示する。fork の PR の head は `owner:branch` と表示する。GitHub の stack に登録された PR には、stack の中の位置と本数を表示する（7章） |
| `gh tree pr <number>` | 指定した PR から根の branch までの経路と、その PR より下の木を表示する。兄弟の PR は表示しない |
| `gh tree <number>` | 番号から Issue か PR かを判定し、該当するサブコマンドとして動く |
| 色分け | Issue と PR の状態ごとに、GitHub の Web UI と同じ系統の色を付ける（7章） |
| 出力形式 | tree（既定）と `--json` |

### v0.2 以降の候補（MVP には入れない）
- `gh tree issue --with-prs`: Issue に紐づいた PR の下に、その PR の base のつながりも展開する（Issue と PR をまとめた表示）。
- 親のない Issue をラベルや検索条件で拾う（`--search "label:analytics is:open"`）。
- `--watch` で一定間隔ごとに再表示する。
- Markdown や Mermaid で出力し、Issue や PR のコメントに貼れるようにする。
- PR のレビュー状態・CI の状態を表示する。
- 設定ファイル（表示するラベル、色の上書きなど）。

### 非スコープ
- Issue・PR への書き込み（読み取りだけ）。
- 状態のずれの判定や警告。このツールは状態を表示するだけにする。
- Projects（v2）の field（Status など）の表示。
- `-R` で別のリポジトリを指定する機能。
- TUI での対話操作。
- GitHub Enterprise Server 固有の対応（動けばよいとし、保証はしない）。

## 7. 表示の仕様（例）
```
$ gh tree issue 1244
#1244 [Epic] Telemetryを判断起点で作り直す   ● open 1/4
├─ #1245 Bricks の Progression Funnel      ✓ completed
│  └─ PR #1248  merged  feature/#1245_… → epic/#1244_…
├─ #1254 削除と待機イベントの一本化         ● open
│  └─ PR #1257  open    feature/#1254_… → epic/#1244_…
├─ #1255 MatchBricksReward の送り元を戻す   ● open
│  └─ PR #1256  draft   feature/#1255_… → epic/#1244_…
└─ #1251 KO エフェクトの装備の計測          ● open

$ gh tree pr
develop
└─ PR #1260 Release 2.4                    open   release/2.4
epic/#1244_telemetry-rebuild               （PR なし）
├─ PR #1256 MatchBricksReward の送り元を戻す  draft  feature/#1255_…
└─ PR #1257 削除と待機イベントの一本化         open   feature/#1254_…

$ gh tree branch epic/#1244_telemetry-rebuild --issues
epic/#1244_telemetry-rebuild               （PR なし）
├─ PR #1256 MatchBricksReward の送り元を戻す  draft  feature/#1255_…
│  └─ #1255 MatchBricksReward の送り元を戻す  ● open
└─ PR #1257 削除と待機イベントの一本化         open   feature/#1254_…
   └─ #1254 削除と待機イベントの一本化         ● open
```
- **Issue の状態の色:** open と reopened は緑、completed と closed は紫、not planned と duplicate は灰。
- **狭いターミナル:** 情報は省かずに行を増やす。タイトルを列の中で折り返し、次に PR の head → base を PR の下の行に移し、それでも列の幅が足りない節点はタイトルを幅いっぱいに出して状態と詳細を下の行に移す（多くの節点がそうなるときは全節点をそろえる）。節点の続きの行は子の行より深く字下げする。アイコンは語から離さず、CJK の1文字だけを最後の行に残さない。
- **リンク:** ターミナルに出すときは、Issue と PR の番号とタイトルを OSC 8 のハイパーリンクにして GitHub の URL を開けるようにする。リンクは行ごとに閉じ、罫線・ラベル・状態には付けない。パイプやリダイレクトには付けない。
- **stack:** GitHub の stacked pull requests に登録された PR（GraphQL の `PullRequest.stack` と `stackEntry`）には、head の後ろに位置と本数の印（`⧉ 2/3`）を付け、印の列をそろえる。PR の下の行に head を移すときは印も一緒に移す。stack の番号は Issue や PR と同じ並びの番号で `#N` と読み違えやすいため、表示には出さず `--json` の `stack.number` にだけ入れる。登録されていない PR には印を付けない（tree の形は登録の有無にかかわらず base と head で決まる）。
- **PR の状態の色:** open は緑、draft は灰、merged は紫、closed（merge なし）は赤。
- **PR がない branch:** Epic PR がない branch は、その branch がどこから生えているかを API から知る手段がないため、独立した根にする。`gh tree branch develop` には出ず、`gh tree branch epic/#1244_…` で見る。配下に PR もない branch は表示しない。

## 8. tree の組み立て規則
- **PR の親:** PR の base と同じ名前の head を持つ PR を親にする。親の候補は、同じリポジトリの branch を head にした PR（`isCrossRepository:false`）に限る。fork の PR は子にはなるが、親にはならない。候補が複数あるときは、open の PR を優先する。open がなければ、候補のうち最も新しく作られた PR を親にする。候補がなければ、base の branch を節点にして、その子にする。
- **循環:** base と head のつながりが循環していたら、循環を見つけた所で切り、その PR を根に置く。
- **消えた branch:** merge 後に base の branch が消えた PR も、上の規則どおりに置く。`baseRefName` と `headRefName` は branch が消えても残るので、これだけを使い、`baseRef` と `headRef`（消えると null）には頼らない。表示全体は失敗させない。
- **深さ:** `--depth` は根を 0 として数える。紐づいた PR と Issue の行も1段に数える。

## 9. 非機能要件
- **性能:** 次の問い合わせ回数で取ります。子が50件の Epic、または open の PR が100件以下のリポジトリで、3秒以内に表示することを目標にします（API の計測では、取得だけで 0.9〜2.0 秒でした。`docs/api-verification.md`）。
  - Issue tree は、根の Issue 1件につき GraphQL をおおむね1回。子を `first:100`、孫を `first:20` で深さ2まで一度に取り（cost 約22）、`hasNextPage` の子と、それより深い分だけを追加で取得する。
  - PR tree は、PR の一覧を `first:100` のページ送りで取って、手元でつなぐ。ページ送りは順番にしか取れないため、open の PR が100件を超えるリポジトリ（1,000件で約10秒）と、期間を区切らずに全件を取る `--state merged|all` は、3秒の目標の対象外とする。
- **認証:** `gh` の認証をそのまま使う。必要な scope は `repo` だけ。足りないときは、その旨と対処を表示する。
- **設定:** 設定ファイルは持たない。設定なしで、どのリポジトリでも同じように動く。
- **配布:** Go で作り、コンパイル済みの extension として配る（`gh extension install owner/gh-tree`）。名前は `gh-tree` のままにする。「tree」からはファイル構成の表示を連想しやすいため、README の冒頭で何の tree を表示するかを明記する。

## 10. 技術方針
- **言語:** Go と `cli/go-gh`（認証、リポジトリの判定、GraphQL クライアント、表示幅と色の扱い）。表示には `lipgloss` などを使う。
- **API:**
  - Issue: `state`、`stateReason`（`enableDuplicate` は付けない。削除予定の引数で、なくても DUPLICATE が返る）、`subIssues`、`subIssuesSummary`、`closedByPullRequestsReferences(userLinkedOnly:false, includeClosedPrs:true)`、`labels`、`parent`（根の問い合わせで5段まで入れ子に取り、それより上は node id で追加取得する）
  - PR: `pullRequests(states:…) { number title state isDraft baseRefName headRefName isCrossRepository headRepositoryOwner{login} createdAt stack{number size baseRefName} stackEntry{position} closingIssuesReferences{ number repository{nameWithOwner} } labels }`。`headRepository` は fork が消えると null になるので使わない
- **データの持ち方:** 節点を共通の形にし、表示する tree と同じ入れ子で持ちます。紐づいた PR や Issue も、`kind` を付けて `children` に入れます。
  - 項目: `kind（issue | pr | branch）, repo, number, title, url, state, stateReason, isDraft, base, head, headOwner（fork の PR だけ）, stack（登録された stack の PR だけ）, name（branch の名前）, labels, progress, hiddenClosed（--hide-closed で畳んだ数）, omittedChildren（祖先の、経路にない子の数）, focus（祖先を出したときの指定した Issue）, children[]`。定義は `internal/model/model.go`
  - JSON はこの形をそのまま出し、先頭に `schemaVersion` を置きます。エージェントが読む前提なので、項目の名前は変えない約束にします。形を変えるときは `schemaVersion` を上げます。
- **作りの分け方:** 取得（Issue と PR の取得部分）、組み立て（親子や base と head で tree を作る）、表示（tree / JSON）の3つに分ける。表示の部分は Issue と PR で共通にする。

## 11. 受け入れ条件（MVP）
- [ ] `gh tree issue 1244` で、Sub-issue の tree・状態（closed の理由を含む）・紐づいた PR・直下の子の完了数が色付きで出る。紐づいた PR は、Issue の子の行として出る。
- [ ] 手で紐づけた PR、closing keyword で紐づいた PR、merge されずに close された PR が、紐づいた PR として出る。
- [ ] 別のリポジトリの Sub-issue が `owner/repo#N` として出る。
- [ ] `gh tree pr` で、open の PR が base と head でつながった tree で出る。Epic PR がない Epic branch は、独立した根の節点として出る。fork の PR は親にならない。
- [ ] `gh tree branch epic/#1244_…` で、その branch を根にした PR tree が出る。
- [ ] `gh tree pr 1257` で、根の branch までの経路と、その PR より下の木だけが出る。
- [ ] `gh tree 1257` と `gh tree 1244` で、Issue か PR かを判定して表示する。
- [ ] `--json` の出力が、tree の表示と同じ情報と同じ入れ子を持ち、`schemaVersion` を含む。
- [ ] `--no-color` と `NO_COLOR` で色が消える。
- [ ] 2つ以上のリポジトリで、設定なしで動く。
