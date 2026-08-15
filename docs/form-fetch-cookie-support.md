# Form / Fetch / Cookie対応表

この表はGrowse v0.9.0の実装を基準とする。「部分対応」は主要な利用経路を扱えるが、Web標準の全機能を実装していない項目を表す。

## Form Controls

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| `input` | 部分対応 | text、password、email、url、number、checkbox、radio、submit、buttonを扱う |
| `textarea` | 対応 | live value、改行、keyboard編集を扱う |
| `select` / `option` | 部分対応 | 単一選択、keyboard選択、selected stateを扱う。multipleは対象外 |
| `button` | 対応 | button、submitとdisabled stateを扱う |
| Focus / keyboard | 部分対応 | Tab順序、click focus、基本的な文字入力と編集を扱う。IME compositionは対象外 |
| CSS pseudo-class | 部分対応 | `:focus`、`:checked`、`:disabled`を扱う |

## ValidationとSubmission

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| Constraint Validation | 部分対応 | required、minlength、maxlength、min、max、step、email、url、patternを扱う |
| invalid control | 対応 | DOM順で最初のinvalid controlを特定してfocusできる |
| submit event | 対応 | submitterを保持し、`PreventDefault`で送信をcancelできる |
| successful controls | 対応 | disabled、unchecked、nameなしなどを除外し、重複nameとDOM順を保持する |
| GET / POST | 対応 | GET queryと`application/x-www-form-urlencoded` POSTを扱う |
| submitter override | 対応 | formaction、formmethod、formenctype、formtargetを扱う |
| 対象外 | 非対応 | multipart/form-data、file upload、dialog target、native validation UI |

Form entryは1,000件、各name/valueは64 KiB、urlencoded結果は1 MiBを上限とする。

## WebGo FetchとHTTP Lifecycle

WebGoは`growse/fetch`の`Fetch(Request, success, failure)`を利用する。相対URL、method、複数値Header、text/byte body、Credentials Modeを指定できる。Responseはstatus、status text、final URL、redirected、公開Headerを持ち、Bodyは`Bytes`、`Text`、`JSON`のいずれかで一度だけ消費できる。

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| 非同期callback | 対応 | Page event queueで成功・失敗のどちらか一回を通知する |
| redirect | 対応 | 301、302、303、307、308とmethod変換を扱い、loop検出・最大10回を適用する |
| timeout / cancel | 対応 | 既定15秒。NavigationとPage終了で進行中Requestをcancelする |
| Body helper | 対応 | Bytes、Text、JSON。二重消費はErrorになる |
| Credentials Mode | 対応 | `omit`、`same-origin`、`include` |
| safety limit | 対応 | Request 1 MiB、Header 100件/64 KiB、Response既定4 MiB |
| HTTP Cache | 対応 | Navigation、Resource Loading、WebGo Source、Fetchで共通のprivate cacheを使用する |
| 対象外 | 非対応 | Promise、ReadableStream、AbortController、Service Worker |

## Cookie

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| Cookie Jar共有 | 対応 | Navigation、Form Submission、FetchがPage単位のメモリ内Jarを共有する |
| Domain / Path | 対応 | host-only、domain-match、path-matchを適用する |
| Secure / HttpOnly | 対応 | SecureはHTTPSだけへ送信し、HttpOnlyはWebGoへ公開しない |
| SameSite | 対応 | Strict、Lax、NoneをRequest種別・method・siteで判定し、NoneにはSecureを要求する |
| expiration | 対応 | Max-AgeとExpiresを扱い、期限切れを除去する |
| safety limit | 対応 | Cookie 1件4 KiB、domainごと180件、Jar全体3,000件 |
| 対象外 | 非対応 | 永続化、Partitioned Cookie、Public Suffix List全体 |

## Same-OriginとCORS

HTTP(S)のscheme、host、正規化したportでOriginを比較する。WebGo Fetchのcross-origin requestにはOrigin Headerを付け、simple requestまたは成功したpreflightだけを送信する。

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| same-origin Fetch | 対応 | 同一OriginではCORS response headerを要求しない |
| simple CORS | 対応 | safelisted methodとHeaderを判定する |
| preflight | 対応 | OPTIONS responseのOrigin、Method、Header、Credentialsを検証し、Max-Ageをcacheする |
| response filtering | 対応 | safelistedまたは明示的に公開されたHeaderだけをWebGoへ渡す |
| credentialed CORS | 対応 | wildcard Originを拒否し、明示OriginとAllow-Credentialsを要求する |
| forbidden Header | 対応 | Cookie、Host、Content-LengthなどWebGoが設定できないHeaderを拒否する |
| 対象外 | 非対応 | opaque Origin、Private Network Access、CORB、CSP、Mixed Content |

WPT由来テストの固定revision、出典、適応差分は[Web Platform Tests由来テスト](wpt.md)を参照する。
HTTP Cacheのdirective、永続化、quota、および対象外は[Storage / Cache対応表](storage-cache-support.md)を参照する。
