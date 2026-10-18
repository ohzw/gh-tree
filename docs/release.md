# リリースの手順とバージョンの決め方

## バージョンの決め方

[Semantic Versioning](https://semver.org/lang/ja/) に従い、tag は `vX.Y.Z` とする。互換を約束する範囲は、コマンドとフラグ（名前と意味）と `--json` の出力。tree の文字表示は人が読むためのものなので、見た目を変えても互換を壊したことにはしない。

1.0.0 より前（`0.Y.Z`）は、上の桁を1つずつずらして使う。

| 上げる桁 | 変更の種類 | 例 |
|---|---|---|
| `Y`（0.1.0 → 0.2.0） | 互換を壊す変更 | フラグやサブコマンドの削除・改名・意味の変更、`--json` の項目の削除・改名・形の変更（`schemaVersion` を上げる変更） |
| `Z`（0.1.0 → 0.1.1） | 互換を壊さない変更 | 機能の追加、`--json` への項目の追加、表示の改善、不具合の修正 |

1.0.0 以降は通常の SemVer に戻す（互換を壊す変更は `X`、機能の追加は `Y`、不具合の修正は `Z`）。

- `schemaVersion` は JSON の形だけを表す番号で、アプリのバージョンとは別に数える。`schemaVersion` を上げるときは、互換を壊す変更としてバージョンを上げる。
- README や docs だけの変更ではリリースしない。次のリリースに含める。

## 手順

1. `main` に変更を push し、test のワークフローが通っていることを確認する。
2. `main` の先頭に tag を打って push する。

   ```sh
   git tag v0.1.1
   git push origin v0.1.1
   ```

3. `.github/workflows/release.yml`（`cli/gh-extension-precompile`）が各プラットフォームのバイナリを作り、Release に付ける。
4. `gh extension upgrade tree` か `gh extension install ohzw/gh-tree` で、新しいバージョンが入ることを確認する。

tag を打ち間違えたら、Release と tag を消して打ち直す（`gh release delete vX.Y.Z --cleanup-tag`）。利用者がすでに入れた後なら、打ち直さずに次のバージョンを出す。
