# Search Provider (v0.21.0)

toolbarの「検索設定」からproviderを管理する。初期設定はDuckDuckGoで、外部候補は無効。DuckDuckGoの検索先には、JavaScriptの実行成否にかかわらず結果本文を描画できる公開の静的HTML endpointを使用する。

- custom providerには一意なID、表示名、keyword、検索URLを指定して「保存」する。任意で候補URLも指定できる。
- 「既定にする」で通常検索の送信先を変更する。`ddg 検索語`など、keywordと空白を入力するとその検索だけ送信先を切り替える。既定設定は変わらない。
- custom providerを編集・削除できる。DuckDuckGoは削除・template変更できないが、別の有効providerを既定にすると無効化できる。
- HTTPS URLに一個の`{searchTerms}`を含める。例: `https://example.com/search?q={searchTerms}`。queryはUTF-8として一度だけencodeする。credential、fragment、未知placeholder、不正port、authority内のplaceholderを拒否する。
- ページのOpenSearch宣言は設定画面に候補として表示する。表示だけでは通信しない。「この送信先を確認して取得・登録」を押した後にだけdescriptionを取得・検証して登録する。登録後は表示名と自動生成keywordを編集できる。POST、UTF-8以外のInputEncoding、追加Paramは対象外。

外部候補を有効にすると、入力した検索語を選択providerの候補endpointへ送る。設定画面に送信先と送信内容を表示する。keyword切替時は選んだproviderへ送る。空入力、command、URL、credential、localhost / IPなどのURLらしい入力は送信しない。periodやURL区切り文字を含む語句も候補送信から除外する。候補を無効化すると進行中のrequestとgenerationを破棄し、保存に失敗しても送信停止を維持する。

検索Navigation、description取得、候補取得ではHTTPSを維持し、redirectを最大3回に制限する。descriptionは64 KiB、候補応答は256 KiB、取得timeoutは2秒。候補は150 ms以上debounceし、最大50件をplain textとして処理する。queryやprovider応答をapplication logへ出力しない。

設定はOS標準のGrowse profile内の`search-providers.json`へ保存する。providerは最大32件、keywordは32 Unicode scalar value、設定は64 KiB。temporary fileのsyncとatomic renameでcommitし、writerをprofile単位のlockで直列化する。保存失敗時は既存設定を維持する（外部候補の無効化だけは即時反映）。破損時は直前の有効snapshotまたはDuckDuckGoへ復旧し、外部候補を無効にする。

検証は`GOTOOLCHAIN=go1.26.6 go test ./internal/searchprovider ./internal/ui ./internal/omnibox ./internal/network`で実行できる。テストのprovider通信は注入したHTTP transportを使い、実検索providerへ接続しない。
