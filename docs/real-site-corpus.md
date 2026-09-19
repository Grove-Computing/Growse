# v0.20.0 Real-Site Corpus運用

`examples/real-site-compat`は、公開サイトで観測したCSS / layout問題を、CIで再現できるsanitized offline fixtureへ縮約したcorpusである。公開サイトのHTMLを無条件に複製するものではない。

## 固定情報

`corpus.json`は各caseのsource URL、取得日、fixture SHA-256、固定Chrome版、desktop / narrow viewport、DPR、locale、font set、安全上限を保持する。fixtureは外部network、cookie、Authorization、access token、個人化response、tracking payloadを含めない。

参照PNGは`Google Chrome 153.0.8010.52`、DPR 1、font hinting無効で生成した。CIは公開networkへ接続せず、repository内のfixtureと参照PNGだけを使用する。

## Visual evidence

次のcommandは14 case-viewをGrowseでlayout / paintし、Growse PNG、Chrome reference PNG、diff PNG、semantic region metric JSONを生成する。

```sh
GROWSE_REAL_SITE_ARTIFACT_DIR=/tmp/growse-v020-real-site-visual \
  bash tests/v020-real-site-visual.sh
```

metricは表示box / node、stylesheet / CSS rule、glyph、region bounds / text量、P0 / P1候補を記録する。pixel diffだけでmain content消失、required region欠落、text overlap / clipを合格にしない。

## Fixture / baseline更新手順

1. live siteを固定viewportで確認し、変更がsite側かGrowse側かを分離する。
2. 問題の再現に必要なDOM / CSS / resourceだけをfixtureへ反映し、credentialとtracking payloadを除去する。
3. `sha256sum examples/real-site-compat/fixtures/*.html`で変更対象のdigestだけを更新する。
4. 固定Chrome commandで対象のdesktop / narrow referenceを再生成する。
5. PR本文へsource URL、取得日、変更理由、影響region、before / after screenshotを記録する。
6. `bash tests/v020-real-site-visual.sh`と関連package testを実行する。

参照PNGやhashを実装差分へ合わせて無条件に上書きしてはならない。Chrome版を変更する場合は全caseを再確認し、`referenceBrowser`と差分理由を同じPRで更新する。

## 安全上限

- 16 megapixel / screenshot
- 16 MiB / artifact、256 MiB / corpus artifact
- 50,000 glyph、4,000 displayed node、256 stylesheet、8,192 CSS rule / page
- P0 / P1 issueは各case-view最大256件

超過はpanic、hang、無言の切捨てではなく、case、viewport、実測値、上限を含むtest errorにする。
