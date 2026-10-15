# API 前提の検証結果（PRD 8〜10章）

- 実施日: 2026-10-15
- 方法: 認証済みの `gh api graphql`（gh 2.102.0、github.com）で公開データだけを読んだ。GitHub 上のデータは作っていない。
- 時間は手元の Mac から `gh api graphql` を1回起動して応答を受け取るまでの実測値（壁時計時間）。`{viewer{login}}` だけの問い合わせでも 0.34〜0.40 秒かかるので、この分は gh の起動と往復の固定費として含まれている。
- 応答は必要な部分だけを残して短くしてある（`…` で省略）。

---

## 1. `closedByPullRequestsReferences` が手で紐づけた PR と closing keyword の PR の両方を返すか

### PRD の前提
`closedByPullRequestsReferences(userLinkedOnly:false)` で、手で紐づけた PR と `Fixes #N` で紐づいた PR の両方が取れる（4章「紐づいた PR」、10章、11章の受け入れ条件）。

### スキーマ（introspection）
| 引数 | 既定値 | 説明 |
|---|---|---|
| `userLinkedOnly` | `false` | Return only manually linked PRs |
| `excludeUserLinked` | `false` | Exclude manually linked PRs |
| `includeClosedPrs` | `false` | Include closed PRs in results |
| `orderByState` | `false` | Return results ordered by state |

フィールドの説明は "List of open pull requests referenced from this issue"。

### クエリ
```graphql
query($o:String!,$r:String!,$n:Int!){
  repository(owner:$o,name:$r){
    issue(number:$n){
      number state stateReason
      dflt:     closedByPullRequestsReferences(first:10){nodes{number state}}
      all:      closedByPullRequestsReferences(first:10,userLinkedOnly:false,includeClosedPrs:true){nodes{number state}}
      userOnly: closedByPullRequestsReferences(first:10,userLinkedOnly:true,includeClosedPrs:true){nodes{number state}}
      kwOnly:   closedByPullRequestsReferences(first:10,excludeUserLinked:true,includeClosedPrs:true){nodes{number state}}
    }
  }
}
```

### 実データ（抜粋）
| Issue | PR 本文の closing keyword | `dflt` | `all` | `userOnly` | `kwOnly` |
|---|---|---|---|---|---|
| cli/cli#14495 | PR #14516 に `Fixes #14495` | 14516 MERGED | 14516 MERGED | なし | 14516 MERGED |
| astral-sh/uv#21807 | PR #21805 に keyword なし（`Follows up on …#21788` のみ） | 21805 MERGED | 21805 MERGED | 21805 MERGED | なし |
| microsoft/vscode#341412（open） | PR #341393 に keyword なし | 341393 MERGED | 341393 MERGED | 341393 MERGED | なし |
| cli/cli#14614 | PR #14616 に `Fixes #14614`（手でも紐づけ済み） | 14616 MERGED | 14616 MERGED | 14616 MERGED | なし |
| cli/cli#14189 | #14211 は手で紐づけ、#14222 は keyword | 14222 MERGED | 14211 CLOSED, 14222 MERGED | 14211 CLOSED | 14222 MERGED |
| astral-sh/ruff#29215 | #29260 は手で紐づけ、#29251・#29322 は keyword | 29322 MERGED | 29251 CLOSED, 29322 MERGED, 29260 CLOSED | 29260 CLOSED | 29251 CLOSED, 29322 MERGED |

```json
// astral-sh/ruff#29215
{"number":29215,"state":"CLOSED","stateReason":"COMPLETED",
 "dflt":{"nodes":[{"number":29322,"state":"MERGED"}]},
 "all":{"nodes":[{"number":29251,"state":"CLOSED"},{"number":29322,"state":"MERGED"},{"number":29260,"state":"CLOSED"}]},
 "userOnly":{"nodes":[{"number":29260,"state":"CLOSED"}]},
 "kwOnly":{"nodes":[{"number":29251,"state":"CLOSED"},{"number":29322,"state":"MERGED"}]}}
```

14 リポジトリの `linked:pr` の Issue（PR 1,700件ほど）を集計すると、どの PR も `userOnly` か `kwOnly` のどちらか一方だけに入っていた。手でも紐づけ、本文にも keyword がある PR（cli/cli#14616 など）は「手で紐づけた」側に入る。

### 判定: **確認できた（条件付き）**
- `userLinkedOnly:false`（既定）で、手で紐づけた PR と keyword の PR の両方が返る。`userLinkedOnly:true` では手で紐づけた PR だけになる。
- ただし `includeClosedPrs` の既定は `false` で、**merge されずに close された PR は返らない**（open・merged は返る）。上の cli/cli#14189 では、手で紐づけた #14211（CLOSED）が既定では消える。
- open な Issue に merged の PR が紐づいたまま残ることもある（microsoft/vscode#341412）。

### PRD への影響
- 10章の API 欄に `includeClosedPrs` をどうするか明記する。7章の「closed（merge なし）は赤」を Issue tree でも出すなら `includeClosedPrs:true` が必要。出さないなら、Issue tree では closed の PR は出ないと書く。
- 4章の「紐づいた PR」の定義に、merge されずに close された PR を含むかどうかを足す。

---

## 2. `subIssues` がリポジトリをまたぐ子を返し、各節点に repository などがあるか

### PRD の前提
Sub-issue は別のリポジトリにもあり得る。各子から `repository.nameWithOwner`、`number`、`state`、`stateReason` が取れ、追加の問い合わせなしで `owner/repo#N` と表示できる（6章、11章）。

### 例の探し方
GraphQL の `search` は `has:sub-issue` や `has:parent-issue` を無視した（ヒット数がリポジトリの Issue 総数と同じだった）。REST の `GET /search/issues?advanced_search=true&q=is:issue has:parent-issue org:…` なら効き、各 item の `parent_issue_url` から、親と子のリポジトリが違う組を見つけた。

### クエリ
```graphql
query($o:String!,$r:String!,$n:Int!){
  repository(owner:$o,name:$r){
    issue(number:$n){
      number repository{nameWithOwner}
      subIssuesSummary{total completed percentCompleted}
      subIssues(first:50){
        totalCount
        nodes{ number state stateReason repository{nameWithOwner} title }
      }
    }
  }
  rateLimit{cost remaining}
}
```

### 実データ（抜粋）
```json
// microsoft/hlsl-specs#602
{"subIssuesSummary":{"total":9,"completed":3,"percentCompleted":33},
 "subIssues":{"totalCount":9,"nodes":[
   {"number":7841,"state":"OPEN","stateReason":null,"repository":{"nameWithOwner":"microsoft/DirectXShaderCompiler"}},
   {"number":7844,"state":"CLOSED","stateReason":"NOT_PLANNED","repository":{"nameWithOwner":"microsoft/DirectXShaderCompiler"}},
   {"number":603,"state":"CLOSED","stateReason":"COMPLETED","repository":{"nameWithOwner":"microsoft/hlsl-specs"}},
   {"number":8433,"state":"CLOSED","stateReason":"COMPLETED","repository":{"nameWithOwner":"microsoft/DirectXShaderCompiler"}},
   …]}}
```
ほかに確認した例:
- dotnet/msbuild#14234 の子に dotnet/sdk#56045（OPEN）
- dotnet/dotnet#8303 の子に dotnet/roslyn#84885（CLOSED / COMPLETED）
- Azure/typespec-azure#4825 の子に Azure/azure-rest-api-specs#47254（OPEN）

### 判定: **確認できた**
- 別のリポジトリの子も `subIssues` に混ざって返り、`repository.nameWithOwner`、`number`、`state`、`stateReason` が1回の問い合わせで取れる。
- 補足: microsoft/hlsl-specs#602 の `subIssuesSummary.completed` は 3 で、内訳は COMPLETED 2件と NOT_PLANNED 1件。**`completed` は closed の数で、理由は問わない**。
- 確かめられなかった点: 子が、見る人に権限のない private リポジトリにある場合の扱い（節点が消えるのか、`totalCount` と数が合わなくなるのか）。確かめるには、private リポジトリの Issue を public な親の子にし、その private リポジトリに権限のないアカウントで読む fixture が要る。

### PRD への影響
- 6章の「直下の子の完了数（例: `1/4`）」は `subIssuesSummary.completed / total` をそのまま使う、つまり not planned と duplicate も「完了」に数える、と明記する（自前で数え直すなら、その規則を書く）。

---

## 3. `stateReason` の値と DUPLICATE

### PRD の前提
Issue の closed の理由は completed / not planned / duplicate（4章、7章）。

### スキーマ（introspection）
`IssueStateReason` の値: `REOPENED`、`NOT_PLANNED`、`COMPLETED`、`DUPLICATE`（どれも deprecated ではない）。

`Issue.stateReason` には引数 `enableDuplicate: Boolean = false` がまだある。説明は「Upcoming Change on 2025-10-01 UTC: `enableDuplicate` will be removed. Reason: The state reason for duplicate issue is now returned by default.」。

### クエリ
```graphql
query{
  repository(owner:"cli",name:"cli"){
    dup:      issue(number:14627){number state stateReason duplicateOf{number}}
    notPlan:  issue(number:14601){number state stateReason}
    done:     issue(number:14495){number state stateReason}
    reopened: issue(number:11942){number state stateReason}
    open:     issue(number:14647){number state stateReason}
  }
  vscode: repository(owner:"microsoft",name:"vscode"){
    dupNoRef: issue(number:333427){number state stateReason duplicateOf{number}}
  }
}
```

### 実データ
```json
{"repository":{
  "dup":      {"number":14627,"state":"CLOSED","stateReason":"DUPLICATE","duplicateOf":{"number":9403}},
  "notPlan":  {"number":14601,"state":"CLOSED","stateReason":"NOT_PLANNED"},
  "done":     {"number":14495,"state":"CLOSED","stateReason":"COMPLETED"},
  "reopened": {"number":11942,"state":"OPEN","stateReason":"REOPENED"},
  "open":     {"number":14647,"state":"OPEN","stateReason":null}},
 "vscode":{
  "dupNoRef": {"number":333427,"state":"CLOSED","stateReason":"DUPLICATE","duplicateOf":null}}}
```
集計:
- 直近に更新された closed の Issue 100件で、`stateReason` と `stateReason(enableDuplicate:true)` は全件一致した（cli/cli: NOT_PLANNED 73、COMPLETED 20、DUPLICATE 7。microsoft/vscode: COMPLETED 76、NOT_PLANNED 18、DUPLICATE 6）。
- 古い順の closed 100件（cli/cli、golang/go）は全件 COMPLETED で、null はなかった。
- open の Issue の `stateReason` は null か `REOPENED`。

### 判定: **確認できた**
- `DUPLICATE` は引数なしの既定で返る。4つの値はどれも実データで出た。
- `REOPENED` は open の Issue に付く値で、closed の理由ではない。
- DUPLICATE でも `duplicateOf` が null のことがある（microsoft/vscode#333427）。

### PRD への影響
- `enableDuplicate` は付けない（削除予定の引数で、なくても DUPLICATE は返る）。
- 表示と JSON で、open の Issue の `stateReason` が `REOPENED` になり得ることを書く。表示では open の理由を出さないのか、`open (reopened)` と出すのかを決める。

---

## 4. 問い合わせのコストと時間

### 4-1. Issue tree（深さ2を1回で取る）

#### クエリ（`C` = 子の `first`、`G` = 孫の `first`）
```graphql
query($o:String!,$r:String!,$n:Int!){
  repository(owner:$o,name:$r){
    issue(number:$n){
      number title state stateReason
      subIssuesSummary{total completed percentCompleted}
      closedByPullRequestsReferences(first:5){nodes{number state isDraft baseRefName headRefName repository{nameWithOwner}}}
      subIssues(first:C){
        totalCount pageInfo{hasNextPage endCursor}
        nodes{
          number title state stateReason repository{nameWithOwner}
          subIssuesSummary{total completed percentCompleted}
          closedByPullRequestsReferences(first:5){nodes{number state isDraft baseRefName headRefName repository{nameWithOwner}}}
          subIssues(first:G){
            totalCount pageInfo{hasNextPage endCursor}
            nodes{
              number title state stateReason repository{nameWithOwner}
              subIssuesSummary{total completed percentCompleted}
              closedByPullRequestsReferences(first:5){nodes{number state isDraft baseRefName headRefName repository{nameWithOwner}}}
            }
          }
        }
      }
    }
  }
  rateLimit{cost remaining}
}
```

#### 対象
子が約50件の親として、REST の advanced search で見つかった中で最も多い dotnet/fsharp#20139（子 59件、孫なし）を使った。孫のある大きな木として、dotnet/msbuild#11801、microsoft/mcp#3104、dotnet/aspnetcore#68227 も測った。

#### 結果（各3回）
| 親 | C / G | 子 | 孫 | PR | cost | 時間（秒） |
|---|---|---|---|---|---|---|
| dotnet/fsharp#20139 | 50 / 50 | 50 / 59（次ページあり） | 0 | 50 | 26 | 0.93, 0.87, 3.31 |
| dotnet/fsharp#20139 | 100 / 100 | 59 / 59 | 0 | 58 | 102 | 1.01, 0.88, 1.26 |
| dotnet/fsharp#20139 | 100 / 20 | 59 / 59 | 0 | 58 | 22 | 1.24, 0.97, 1.00 |
| dotnet/msbuild#11801 | 50 / 50 | 23 / 23 | 128 | 52 | 26 | 2.00, 2.00, 1.83 |
| microsoft/mcp#3104 | 50 / 50 | 28 / 28 | 147（50件を超える子あり） | 64 | 26 | 1.72, 1.58, 2.03 |
| microsoft/mcp#3104 | 100 / 100 | 28 / 28 | 160 | 74 | 102 | 1.95, 1.53, 1.61 |
| dotnet/aspnetcore#68227 | 50 / 50 | 11 / 11 | 57 | 0 | 26 | 0.81, 0.92, 0.81 |

```json
// dotnet/msbuild#11801（C=50, G=50）抜粋
{"number":11801,"state":"OPEN","stateReason":null,
 "subIssuesSummary":{"total":23,"completed":13,"percentCompleted":56},
 "subIssues":{"totalCount":23,"nodes":[
   {"number":12460,"state":"CLOSED","stateReason":"COMPLETED","repository":{"nameWithOwner":"dotnet/msbuild"},
    "subIssuesSummary":{"total":2,"completed":2,"percentCompleted":100},
    "closedByPullRequestsReferences":{"nodes":[]},
    "subIssues":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":"Mg"},"nodes":[
      {"number":11827,"state":"CLOSED","stateReason":"COMPLETED",
       "closedByPullRequestsReferences":{"nodes":[{"number":11941,"state":"MERGED","isDraft":false,
         "baseRefName":"main","headRefName":"spec-for-🧵","repository":{"nameWithOwner":"dotnet/msbuild"}}]}}]}},
   …]},
 "rateLimit":{"cost":26,"remaining":4804}}
```

#### 読み取れること
- cost は、取る節点の数ではなく `first` の掛け算で決まる（C×G ≒ 2,500 で 26、10,000 で 102、2,000 で 22）。実際の子と孫が少なくても同じ cost になる。
- 子 59件・孫なしなら 0.9〜1.3 秒（1回だけ 3.3 秒の外れ値）。孫まで含めて 150〜200 節点ほどの木では 1.6〜2.0 秒で、取得だけで2秒の目標にほぼ届く。描画の時間は含んでいない。
- 1回で取れるのは、子が `C` 件、孫が子ごとに `G` 件まで。超えた分（fsharp#20139 で C=50 のとき、mcp#3104 で G=50 のとき）はページ送りでの追加取得が要る。

### 4-2. PR の一覧

#### クエリ
```graphql
query($o:String!,$r:String!){
  repository(owner:$o,name:$r){
    pullRequests(states:OPEN,first:100,orderBy:{field:CREATED_AT,direction:DESC}){
      totalCount pageInfo{hasNextPage endCursor}
      nodes{number title state isDraft baseRefName headRefName createdAt
            closingIssuesReferences(first:5){nodes{number}}}
    }
  }
  rateLimit{cost remaining}
}
```

#### 結果（各3回）
| リポジトリ | open の PR | 取った件数 | cost | 時間（秒） |
|---|---|---|---|---|
| microsoft/vscode | 2,847 | 100 | 1 | 1.14, 0.90, 0.94 |
| kubernetes/kubernetes | 1,362 | 100 | 1 | 0.89, 1.03, 1.12 |
| home-assistant/core | 1,017 | 100 | 1 | 0.97, 0.83, 0.94 |

home-assistant/core の open の PR を全件ページ送りで取ると、11ページ、cost 11、合計 9.8 秒だった（順番に取得）。

```json
// microsoft/vscode 抜粋
{"totalCount":2847,"pageInfo":{"hasNextPage":true},
 "nodes":[{"number":341692,"title":"Don't reattach restored terminal editors to another window's terminal",
   "state":"OPEN","isDraft":true,"baseRefName":"main","headRefName":"anthonykim1/verify-restored-terminal-pid",
   "createdAt":"2026-10-15T06:24:11Z","closingIssuesReferences":{"nodes":[{"number":341502}]}}, …]}
```

### 判定: **確認できた（2秒の目標は Issue tree の大きい木ではぎりぎり）**
- Issue tree は根1件につき1回で、深さ2まで取れる。cost は 22〜26 点（C×G を 2,500 以下にした場合）で、1時間 5,000 点の上限に対して問題はない。
- open の PR が100件なら1回、cost 1、約1秒で取れる。
- ページ送りは順番にしかできない（次の cursor が要る）ので、open の PR が1,000件を超えるリポジトリでは `gh tree pr` の既定（open）でも10秒前後かかる。PRD は `--state merged|all` だけを2秒の目標の対象外にしているが、open だけでもこの規模になる。

### PRD への影響
- 9章の Issue tree の取り方に、`first` の値を書く。例: 子 `first:100`、孫 `first:20`（cost 22、子 100件まで1回で取れる）として、`hasNextPage` の子だけを追加で取る。
- 9章の2秒の目標は、計測値（取得だけで 0.9〜2.0 秒）に照らすと余裕がない。目標を「取得 + 描画で 3 秒」にするか、計測の条件（ネットワーク、gh の起動を含むか）を書く。
- open の PR が数百件を超えるリポジトリでは、`gh tree pr` の既定（open）でも複数ページになり2秒を超える、と書く。

---

## 5. 削除された branch・fork の PR の `baseRefName` / `headRefName` / `headRepository`

### スキーマ（introspection）
| フィールド | 型 | 説明 |
|---|---|---|
| `baseRefName` | `String!`（NON_NULL） | name of the base Ref …, even if the ref has been deleted |
| `headRefName` | `String!`（NON_NULL） | name of the head Ref …, even if the ref has been deleted |
| `baseRef` / `headRef` | `Ref`（nullable） | |
| `headRepository` | `Repository`（nullable） | |
| `headRepositoryOwner` | `RepositoryOwner`（nullable） | |
| `isCrossRepository` | `Boolean!` | |

### クエリ
```graphql
query{
  cli: repository(owner:"cli",name:"cli"){
    sameRepoDeleted: pullRequest(number:1){number state merged isCrossRepository baseRefName headRefName baseRef{name} headRef{name} headRepository{nameWithOwner} headRepositoryOwner{login}}
  }
  vscode: repository(owner:"microsoft",name:"vscode"){
    deletedFork:     pullRequest(number:61){number state merged isCrossRepository baseRefName headRefName baseRef{name} headRef{name} headRepository{nameWithOwner} headRepositoryOwner{login}}
    sameRepoDeleted: pullRequest(number:124){number state merged isCrossRepository baseRefName headRefName baseRef{name} headRef{name} headRepository{nameWithOwner} headRepositoryOwner{login}}
  }
  ha: repository(owner:"home-assistant",name:"core"){
    forkHeadIsDev:   pullRequest(number:181481){number state isCrossRepository baseRefName headRefName headRepository{nameWithOwner} headRepositoryOwner{login}}
  }
}
```

### 実データ
```json
{"cli":{"sameRepoDeleted":{"number":1,"state":"MERGED","merged":true,"isCrossRepository":false,
   "baseRefName":"prototype","headRefName":"gh-pr","baseRef":null,"headRef":null,
   "headRepository":{"nameWithOwner":"cli/cli"},"headRepositoryOwner":{"login":"cli"}}},
 "vscode":{
  "deletedFork":{"number":61,"state":"MERGED","merged":true,"isCrossRepository":true,
   "baseRefName":"master","headRefName":"fix-typo","baseRef":null,"headRef":null,
   "headRepository":null,"headRepositoryOwner":{"login":"mogemimi"}},
  "sameRepoDeleted":{"number":124,"state":"MERGED","merged":true,"isCrossRepository":false,
   "baseRefName":"master","headRefName":"weinand-patch-1","baseRef":null,"headRef":null,
   "headRepository":{"nameWithOwner":"microsoft/vscode"},"headRepositoryOwner":{"login":"microsoft"}}},
 "ha":{"forkHeadIsDev":{"number":181481,"state":"OPEN","isCrossRepository":true,
   "baseRefName":"dev","headRefName":"dev",
   "headRepository":{"nameWithOwner":"forestvogo/core"},"headRepositoryOwner":{"login":"forestvogo"}}}}
```
集計: microsoft/vscode の古い順の merged / closed の PR 100件のうち、50件は `headRepository` が null（fork が消えている）。どれも `headRefName` と `baseRefName` は入っており、`headRepositoryOwner` も null ではなかった。

### 判定: **確認できた**
- head・base の branch が消えても、`baseRefName` と `headRefName` は残る（型も NON_NULL）。消えると null になるのは `baseRef` と `headRef`。
- fork が消えた PR では `headRepository` が null になる。`headRepositoryOwner.login` は残っていた。

### 見つかった問題: fork の head の名前が base の名前とぶつかる
- PRD 8章の「base と同じ名前の head を持つ PR を親にする」は、head の branch がどのリポジトリにあるかを見ていない。
- home-assistant/core の open の PR 1,017件のうち 1,008件は base が `dev` で、fork の `dev` を head にした PR が 13件ある（#181481 など）。この規則のままだと、**fork の PR 13件が、`dev` 向けの PR 約1,000件の親の候補になる**。
- PRD 10章の PR のフィールドには `isCrossRepository` も `headRepository` もないので、今の取得内容ではこれを区別できない。

### PRD への影響
- 8章の「PR の親」を、同じリポジトリの head（`isCrossRepository:false`）を持つ PR だけを親の候補にする、と直す。fork の PR は親にならない（子にはなる）。
- 10章の PR のフィールドに `isCrossRepository` を足す。fork の branch を `owner:branch` と表示するなら `headRepositoryOwner{login}` も足す（`headRepository` は null になり得るので使わない）。
- 8章の「消えた branch」は、`baseRefName` / `headRefName` だけを使えば今の書き方で成り立つ。`baseRef` / `headRef` には頼らない、と書き添える。

---

## 判定のまとめ

| # | PRD の前提 | 判定 | 要点 |
|---|---|---|---|
| 1 | `closedByPullRequestsReferences(userLinkedOnly:false)` で、手で紐づけた PR と keyword の PR の両方が取れる | 確認できた（条件付き） | 両方取れる。ただし既定（`includeClosedPrs:false`）では merge されずに close された PR が返らない |
| 2 | `subIssues` が別のリポジトリの子を返し、`repository.nameWithOwner`・`number`・`state`・`stateReason` がある | 確認できた | `subIssuesSummary.completed` は理由を問わず closed を数える。権限のない private の子は確かめられなかった |
| 3 | `stateReason` に completed / not planned / duplicate がある | 確認できた | DUPLICATE は既定で返る（`enableDuplicate` は不要で、削除予定）。open では `REOPENED` もあり得る |
| 4 | 根1件につき約1回、2秒以内 | 確認できた（目標はぎりぎり） | 深さ2は cost 22〜26、0.9〜2.0 秒。open の PR 100件は cost 1、約1秒。1,000件を超えると約10秒 |
| 5 | 消えた branch でも base / head が取れる | 確認できた | `*RefName` は NON_NULL で残る。`headRepository` は null になり得る。fork の head 名が base 名とぶつかる問題が見つかった |

## 推奨する PRD の修正

1. **8章「PR の親」:** 親の候補を、同じリポジトリの head を持つ PR（`isCrossRepository:false`）に限る。fork の PR は親にしない。（5）
2. **10章 API（PR）:** `isCrossRepository` を足す。fork の branch を `owner:branch` と出すなら `headRepositoryOwner{login}` も足す。`headRepository` は null になり得るので頼らない。（5）
3. **10章 API（Issue）:** `closedByPullRequestsReferences` の `includeClosedPrs` を明記する。Issue tree で merge されずに close された PR（赤）を出すなら `true` にする。（1）
4. **4章「紐づいた PR」:** merge されずに close された PR を含むかどうかを定義に足す。（1）
5. **6章 Issue tree の完了数:** `subIssuesSummary.completed / total` を使い、not planned と duplicate も完了に数える、と明記する（または自前で数える規則を書く）。（2）
6. **4章「状態」・7章:** open の Issue の `stateReason` が `REOPENED` になり得ることと、その表示の仕方を書く。`enableDuplicate` は使わない。（3）
7. **9章 性能:** Issue tree の `first` の値（例: 子 100、孫 20、cost 22）と、`hasNextPage` の子だけを追加で取ることを書く。2秒の目標は、取得だけで 0.9〜2.0 秒という計測値に合わせて見直すか、計測の条件を書く。（4）
8. **9章 性能:** open の PR が数百件を超えるリポジトリでは、`gh tree pr` の既定（open）でも2秒を超えることを書く（1,017件で約10秒）。（4）
9. **10章 API（PR）:** `--issues` で別のリポジトリの Issue を `owner/repo#N` と出すなら、`closingIssuesReferences` に `repository{nameWithOwner}` を足す。
