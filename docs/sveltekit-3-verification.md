# SvelteKit 3.0 Release Candidate Verification

検証日: 2026-11-07  
対象: `release/v0.21.0` / SvelteKit fixture

## 固定version

| Component | Version |
| --- | --- |
| `@sveltejs/kit` | 3.0.0 |
| `svelte` | 5.57.1 |
| `@sveltejs/adapter-static` | 4.0.1 |
| `vite` | 8.3.6 |
| Node.js | 24.18.0 |
| pnpm | 11.21.0 |

`package.json`、`pnpm-lock.yaml`、`fixture-manifest.json`のversionが一致することを確認した。lockfileから解決済みのdependency treeを使い、一時worktreeでSvelteKit static buildとUnicode正規化を再実行した結果、`sources/sveltekit/build`と`fixtures/sveltekit/upstream-export`はbyte-for-byte一致した。`upstream.sha256`の17 artifactもすべて一致した。

完全なoffline reinstallは、このworkstationのpnpm storeに`esm-env-1.2.2` tarballがないため実行できなかった。既存のlockfile準拠`node_modules`からのbuild再現には外部networkを使用していない。CIはchecked-in artifactとdigestを検査し、npm registryへ接続しない。

## Growseで確認した契約

- isolated desktop JavaScript RuntimeがSvelteKit SSRを維持したままhydrationを完了し、Runtime / script errorを残さない
- hydration前後でSSR root identityを維持する
- SvelteKit 3.0.0のupstream entrypoint / application contractを一度ずつ取得する
- reactive state、class / style mutation、label付きinput、enhanced form submitが動作する
- dialogを開いてfocusし、menuを表示できる
- client navigationで`/svelte/about`へ移動し、BackでSSR rootとpopstateを復元する
- desktop 1024 px / narrow 640 pxでhydration後のinteractionとscroll対象を保持する
- Go EngineではSSR HTMLだけを保持し、JavaScript moduleを取得・実行しない
- fixture manifestとupstream digestが改変されていない

## 実行したgate

中心となる個別testは`TestSvelteKitIsolatedDesktopRuntimeHydratesPromptly`と`TestSvelteKitSSRFixtureHydratesAndEnhancesForm`である。

```sh
GOTOOLCHAIN=go1.26.10 go test -race ./examples/modern-web-compat \
  -run 'Test(SvelteKitIsolatedDesktopRuntimeHydratesPromptly|SvelteKitSSRFixtureHydratesAndEnhancesForm)$' \
  -count=1 -v

GOTOOLCHAIN=go1.26.10 go test -race ./examples/modern-web-compat \
  -run 'TestV019FrameworkCorpusDesktopNarrowHydrationInteractionAndScroll/sveltekit|TestFrameworkFixturesKeepJavaScriptDisabledInGoEngine/SvelteKit|TestUpstreamFrameworkBuildDigests/sveltekit' \
  -count=1 -v
```

両gateは成功した。build時にLightning CSSがTailwind固有の`@theme` / `@tailwind` / `@source`をunknown at-ruleとして通知するが、buildは成功し、正規化後artifactとdigestは固定baselineに一致する。
