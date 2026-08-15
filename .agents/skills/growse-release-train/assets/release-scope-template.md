# Growse {{VERSION}} スコープ定義書

## 1. 文書概要

本書は、Growse {{VERSION}}で実装する{{SCOPE_SUMMARY}}の範囲、完了条件、対象外を定義する。

{{VERSION}}は、{{PREVIOUS_VERSION}}までの実装が完成していることを前提とする。

---

## 2. リリーステーマ

> **{{RELEASE_THEME}}**

{{USER_VALUE}}

---

## 3. 準拠基準

- {{STANDARD_OR_SPECIFICATION}}

仕様全体への適合は宣言せず、本書で明示した対応範囲に限定する。

---

## 4. 目標

1. {{GOAL_1}}
2. {{GOAL_2}}
3. {{GOAL_3}}

---

## 5. {{MAJOR_ITEM_1}}

### 5.1 {{SUBJECT}}

{{REQUIREMENTS_AND_SEMANTICS}}

### 5.2 安全上限

- {{LIMIT_OR_FAILURE_POLICY}}

---

## 6. {{MAJOR_ITEM_2}}

{{REQUIREMENTS_AND_SEMANTICS}}

---

## 7. テスト方針

- Unit Test: {{UNIT_TEST_STRATEGY}}
- Integration Test: {{INTEGRATION_TEST_STRATEGY}}
- Visual Regression: {{VISUAL_TEST_STRATEGY}}
- Conformance Test: {{CONFORMANCE_TEST_STRATEGY}}

---

## 8. Showcase

{{SHOWCASE_AND_USER_FLOW}}

---

## 9. 実装フェーズ

### Phase 1 — {{MAJOR_ITEM_1}}

{{PHASE_1_SCOPE}}

### Phase 2 — {{MAJOR_ITEM_2}}

{{PHASE_2_SCOPE}}

### Phase 3 — 品質とリリース

{{QUALITY_SCOPE}}

---

## 10. {{VERSION}} 非対象

- {{OUT_OF_SCOPE_1}}
- {{OUT_OF_SCOPE_2}}

---

## 11. 完了条件

### {{MAJOR_ITEM_1}}

- [ ] {{ACCEPTANCE_CRITERION_1}}
- [ ] {{ACCEPTANCE_CRITERION_2}}

### {{MAJOR_ITEM_2}}

- [ ] {{ACCEPTANCE_CRITERION_3}}
- [ ] {{ACCEPTANCE_CRITERION_4}}

### 品質とリリース

- [ ] 対応FeatureのUnit TestとIntegration Testが成功する
- [ ] 安全上限、失敗、cancel、Lifecycleを決定的に検証できる
- [ ] `go test ./...`が成功する
- [ ] `make ci`が成功する
- [ ] CIの全必須チェックが成功する
- [ ] Linux、macOS、Windows向け成果物を生成できる
- [ ] Dockerイメージを生成できる
- [ ] README、SECURITY.md、対応表、Showcaseが{{VERSION}}の実装と一致する

---

## 12. Growse {{VERSION}}の定義

Growse {{VERSION}}とは、

> **{{RELEASE_DEFINITION}}**

を満たすReleaseである。
