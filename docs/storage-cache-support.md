# Storage / Cache対応表

この表はGrowse v0.10.0の実装を基準とする。「部分対応」は主要なPage利用経路を扱えるが、Web標準の全機能を実装していない項目を表す。

## Web Storage

WebGoは`growse/storage`の`Local()`と`Session()`からStorageを取得し、`Get`、`Set`、`Remove`、`Clear`、`Length`、`Key`を使用する。`OnChange`では、別のsame-origin TabがcommitしたLocal Storage更新を受信できる。HTTP(S)のscheme、host、正規化したportからOriginを作り、異なるOriginの値を共有しない。

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| Local Storage | 対応 | OSのUser Config Directory配下へOrigin単位で永続化し、same-origin Tab間で共有してBrowser再起動後に復元する |
| Storage Event | 対応 | commit後に更新元以外のsame-origin Tabへkey、old/new value、clear種別、credentialを除去したURLをcommit順に配送する |
| Session Storage | 対応 | TabのPage Sessionごとに分離し、reloadとsame-document traversalでは維持してTab終了時に破棄する |
| key順序 | 対応 | 挿入順を保持し、既存keyのvalue更新では順序を変更しない |
| quota | 対応 | key 4 KiB、value 1 MiB、Origin 5 MiB、Profile 50 MiB、Origin数128 |
| atomic update | 対応 | 一時fileのsyncとrenameでcommitし、失敗時はmemory状態も更新前へ戻す |
| data validation | 対応 | UTF-8、schema version、Origin、重複key、sizeを検証し、破損をErrorとして扱う |
| file protection | 対応 | directory 0700、file 0600。保存値自体は暗号化しない |
| 対象外 | 非対応 | 複数Browser Window／Process間のStorage Event、Session Storage Event、third-party partition、利用者向けStorage管理UI |

既定のLocal Storage rootは`os.UserConfigDir()/growse/local-storage`である。保存file名はOriginのSHA-256で決めるが、内容は平文JSONなので、秘密情報の保管庫としては使用しない。

## HTTP Cache

同じBrowser SessionのNavigation、Stylesheet、Image、WebGo Source、FetchはTab間で同じprivate HTTP Cache Policyを使用する。通常Navigationはfresh entryを再利用し、通常reloadはDocumentを再検証しつつfresh subresourceを再利用する。強制reloadはDocumentとsubresourceの両方へ再検証を要求する。

| 機能 | 対応 | 補足 |
| --- | --- | --- |
| Cache key | 対応 | GET / HEAD、credentialを除去したURL、partition、`Vary` request headerでvariantを分離する |
| Freshness | 対応 | `max-age`、`Expires`、`Date`、`Age`とLast-Modified heuristicを扱う |
| directive | 部分対応 | `no-cache`、`no-store`、`private`、`public`、`must-revalidate`、`immutable`を扱う |
| revalidation | 対応 | `ETag` / `Last-Modified`で条件付きrequestを作り、304 Headerをmergeして保存済みBodyを再利用する |
| invalidation | 対応 | 成功した状態変更Methodの後に同じURLと`Location` / `Content-Location`のsame-origin entryを無効化する |
| Disk Cache | 対応 | OSのUser Cache Directory配下へ保存し、LRU eviction、schema version、Body SHA-256、破損復旧を扱う |
| quota | 対応 | memory 1,024 entry・keyごと32 variant、disk 1 entry 4 MiB・Origin 32 MiB・全体128 MiB |
| Security Policy | 対応 | credentialを含むrequest/responseを保存せず、hit時もMIME、Origin、CORS、Credentials Policyを適用する |
| 対象外 | 非対応 | shared cache、Range、`stale-if-error`、`stale-while-revalidate`、Service Worker Cache API |

既定のDisk Cache rootは`os.UserCacheDir()/growse/http-cache`である。Cacheは性能最適化であり、破損entryや読めないentryは再利用せずNetworkから取得し直す。

## Test出典

Web Storageのkey順、HTTP CacheのFreshnessとInvalidationは固定revisionのWeb Platform Testsから縮約している。Freshness、Age、304 merge、InvalidationはRFC 9111由来のtable-driven Testでも検証する。出典と意図的な差分は[Web Platform Tests由来テスト](wpt.md)を参照する。
