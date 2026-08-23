---
name: growse-release-train
description: Growseの新リリースをテーマ決定、release/vX.Y.Zブランチとdocs/vX.Y.Z.mdの作成、大項目ごとの直列ブランチ、小項目単位の実装・テスト・チェック更新・日本語Conventional Commit、大項目PR、CI修正まで一貫して進める。次リリースの企画、リリース定義、完了条件の実装、または途中のリリース開発を再開するときに使用する。単発Issue、リリースと無関係な修正、タグ作成だけには使用しない。
---

# Growse Release Train

Growseのリリース定義書を唯一の進捗表として扱い、リリースブランチから大項目ブランチを一本の履歴として積み上げる。

## 開始状態を確認する

1. Repository root、remote、default branch、現在branch、未commit変更、既存のrelease branch、open PRを確認する。
2. 未commit変更を利用者の所有物として扱い、対象外の変更をstash、破棄、上書きしない。
3. 対象versionが分かる場合は`scripts/release-status.sh vX.Y.Z`を実行し、定義書とcheckboxの状態を確認する。
4. 既存の`docs/vX.Y.Z.md`、branch、commit、PRから再開位置を判断する。完了済み作業を作り直さない。
5. GitHub操作前に認証を確認する。Networkまたは認証の一時失敗では安全な確認を行って再試行する。

## 新しいリリースを定義する

1. 現行実装、直近のscope文書、README、対応表、未解決Issueを調査する。
2. 次に自然な到達点を表すリリーステーマを2〜3案提示する。各案に利用者価値、主な大項目、対象外を簡潔に付ける。
3. テーマとversionは製品方針を変える選択なので、利用者の決定を待つ。決定前にbranch、scope文書、実装を作らない。
4. default branchを最新化し、cleanな状態から`release/vX.Y.Z`を作成して移動する。
5. `assets/release-scope-template.md`と過去のscope文書を基準に`docs/vX.Y.Z.md`を作る。
6. scope文書へ少なくとも次を含める。
   - 文書概要と前提version
   - 一文のリリーステーマ
   - 準拠基準
   - 目標
   - 大項目別の仕様、安全上限、対象外
   - テスト方針とShowcase
   - 実装フェーズ
   - 大項目別の完了条件checkbox
   - 品質とリリースの完了条件
7. 曖昧なcheckboxを避け、1項目を1つの実装と対応Testで証明できる粒度にする。
8. 文書を検査し、`docs(release): vX.Y.Zのリリーススコープを定義`として日本語commitする。
9. release branchをpushし、後続PRのbaseとして利用可能にする。

## 大項目ブランチを直列化する

完了条件の`###`見出しを大項目、その直下のcheckboxを少項目として扱う。

1. 大項目の性質からbranch prefixを選ぶ。
   - 新機能: `feature/`
   - 不具合修正: `fix/`
   - 保守・依存・Release作業: `chore/`
   - 内部設計変更: `refactor/`
   - 性能改善: `perf/`
   - CI: `ci/`
   - Build: `build/`
   - 文書のみ: `docs/`
2. branch名を`<prefix>/vX.Y.Z-<major-item-slug>`とする。
3. 最初の大項目branchを`release/vX.Y.Z`から作る。
4. 2本目以降は前の大項目branchを起点に作る。`release -> 大項目1 -> 大項目2`の祖先関係を保つ。
5. 次の大項目へ進む前に、前のPRを`release/vX.Y.Z`へMerge commitで取り込む。Squashまたはrebase mergeで祖先関係が失われた場合は、重複差分を持ち込まず最新のrelease branchへ後続branchをrebaseし、例外を報告する。
6. 全大項目PRのbaseを必ず`release/vX.Y.Z`にする。`main`や前のfeature branchをbaseにしない。

## 少項目を一つずつ実装する

各checkboxについて、次を完了するまで次のcheckboxへ移らない。

1. checkboxを満たす観測可能な振る舞いと非対象を特定する。
2. 関連実装と既存Testを読み、必要最小限の変更を実装する。
3. 正常系、境界、失敗またはLifecycleを必要に応じて含む対応Testを追加する。
4. 最小の対象Testを実行し、続いて関連packageのTestを実行する。
5. Testが成功した後だけ、対象checkboxを`[x]`へ更新する。
6. 実装、対応Test、checkbox更新だけを明示的にstageする。
7. 次の形式で日本語の詳細commitを作る。

```text
feat(scheduler): 同一deadlineのTimerを登録順に配送する
fix(storage): rename失敗時に更新前の値へロールバックする
chore(release): vX.Y.Zの成果物検証を追加する
```

branchの`feature/`はcommitでは`feat`を使用する。Testだけなら`test`、文書だけなら`docs`、CIだけなら`ci`を使用する。

Test失敗時はcheckboxを更新せず、原因を調査して同じ少項目の範囲で修正する。無関係な失敗を見つけた場合は証拠を記録し、必要なら別の完了条件または別Issueとして分離する。

## 大項目をPRにする

1. 大項目の全checkboxが完了したことを確認する。
2. 対象package Test、`go test ./...`、`make ci`を実行する。環境差が重要なら対象OSのGitHub Actionsも実行する。
3. diff、commit列、release branchとの差分を確認し、前の大項目や利用者の変更が混ざっていないことを確認する。
4. branchをpushし、`release/vX.Y.Z`向けPRを作る。PR titleにも`feat:`、`fix:`、`chore:`などを付ける。
5. PR本文へ大項目、完了checkbox、主要設計、安全上限、Test結果を記載する。
6. 必須CIを監視する。失敗した場合はlogから原因を特定し、in-scopeで非破壊的な修正なら同じbranchへ日本語commitして再実行する。CI失敗だけを理由に利用者の再承認待ちで停止しない。
7. CI成功後、リリース全体の実行を依頼されている場合はMerge commitでrelease branchへ取り込み、次の大項目へ進む。単一大項目だけを依頼されている場合はPRを渡して終了する。

## リリース完了を判定する

1. 全大項目と品質・リリースのcheckboxが`[x]`であることを確認する。
2. `go test ./...`、`make ci`、必須GitHub Actionsを再実行する。
3. README、SECURITY.md、対応表、Showcase、Installer、成果物versionがscope文書と一致することを確認する。
4. release branchのopen PR、未push commit、未commit変更がないことを確認する。
5. 完了内容、PR一覧、Test結果、既知の対象外を報告する。
6. リリース全体の実行を依頼されている場合は、`release/vX.Y.Z`から`main`への最終PRを作成し、必須CI成功後にMerge commitで取り込む。続けてrelease branchのHEADへannotated tag `vX.Y.Z`を作成・pushする。GitHub Releaseがtagから自動作成されるRepositoryでは作成完了を確認し、手動作成は行わない。
7. 利用者が最終PR、tag、またはGitHub Releaseを明示的に除外した場合だけ、その操作を省略して理由を報告する。

## 中断条件

次の場合だけ利用者へ判断を求める。

- リリーステーマ、version、scopeが未決定
- 仕様の選択で公開APIや互換性が大きく変わる
- 完了条件外への重大なscope拡張が必要
- destructive操作、credential変更、Ruleset変更、公開Releaseが必要
- 利用者の未commit変更と競合し、安全に分離できない

通常の実装判断、Test追加、in-scopeなCI修正では進行を止めない。

## Resources

- 新しいscope文書を作るときは`assets/release-scope-template.md`を使用する。
- 開始時、再開時、大項目完了時は`scripts/release-status.sh vX.Y.Z`を実行する。
