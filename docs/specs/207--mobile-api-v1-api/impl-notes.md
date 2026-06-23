# Implementation Notes

本 spec（#207 v1 モバイル API 契約）の実装に関する補足ノート。

## Implementation Notes

### Task 1
- 採用方針: design.md「Mobile API Contract Document の構成」節（L641-678）の章立て（1〜7）を
  そのまま採用し、各エンドポイントを表形式（URL / 認証 / クエリ / 成功応答 / エラー）+
  応答スキーマ JSON ブロック + フィールド説明表の 3 段構成で記述した。
- 重要な判断:
  - **既存サーバ実装の応答形を直接観察してから記述**: `internal/handler/*_handler.go` の
    `json:"..."` タグを `grep` で確認し、契約文書に書く応答スキーマが実装と乖離しないよう
    にした。特に search の `favicon_url`、cross-feed の `feed_favicon_url` / `since_time`、
    subscriptions の `favicon_url` といった命名の非対称を §7.4 で明示。
  - **`/api/items/{id}` の応答スキーマは「拡張後の形」で記述**: 本 spec の後続 task 6 / 7 で
    handler / service に `feed_title` / `feed_favicon_url` を追加する想定なので、契約文書
    側は task 完了後の最終形（拡張後）で記述し、task 7 と整合させた（design.md「設計判断」
    と一致）。
  - **`avatar_url` は v1 では常に null / 省略**: design.md「設計判断: avatar_url を当面 nil
    固定で返す」節に従い、契約文書では「v1 時点では常に null / 省略される。将来 OAuth
    `picture` claim 保存が入れば実値で返る」と明記。クライアント実装者が両表現を等価と
    扱う前提を文書化した。
  - **エラー応答 401 の text/plain 例外を明記**: `BearerOrSession` middleware が 401 で
    `unauthorized` plain text を返す（JSON 形式ではない）という既存仕様を §2.2 末尾で
    明示。クライアントが 401 を JSON パース失敗で検出しない実装にするため。
  - **commit を 3 つに分割**: 文書本体 / impl-notes 追記 / tasks.md marker を Issue #164
    「1 commit = 1 task ID」契約に従い分離。
- 残存課題: なし（後続 task 2〜7 は本契約文書を参照しながら実装される想定。本 task では
  Go コード変更は一切なし、`go vet` 等の verify も実装変更を含む後続 task で実行される）。

### Task 2
- 採用方針: tasks.md task 2 本文に列挙された 5 種類の追記項目（Native auth / starred items /
  search / cross-feed / 手動フェッチ / `GET /api/users/me` / cross-feed-last-seen）を
  既存の責務別 h3 セクション（認証 / フィード管理 / 記事管理 / 購読管理 / ユーザー管理）に
  そのまま追記し、ユーザー管理表の直後に Mobile API Contract Document への参照リンクを
  blockquote 形式で追加した。
- 重要な判断:
  - **`POST /api/auth/token` / `/refresh` / `/revoke` は「認証（認証不要）」表に追記**:
    design.md「Architecture Pattern & Boundary Map」と router.go の認証グループ配線では
    native auth 系は `BearerOrSession` の **外側**（認証不要グループ）に配置されるため、
    README 上も既存 `/auth/google/login` / `/auth/logout` 等と同じ「認証（認証不要）」表に
    束ねる方が境界の説明と整合する。mobile-api-contract.md §3 とも整合（「認証 不要（auth_code
    or refresh_token を body で提示）」と記載）。
  - **`GET /api/users/me` の説明文に「モバイル / Web 共通。Bearer または Cookie で認証」と
    明記**: 既存 `DELETE /api/users/me` と並べた際、両エンドポイントの認証境界が同じ
    （BearerOrSession 必須）であり、`GET /api/users/me` が `/auth/me` とは別の新規モバイル/Web
    共通エンドポイントである旨を 1 行で示せるようにした。
  - **`GET /auth/me` の説明文を「Web Cookie 専用」に補足**: 本 spec の Req 2.6 / NFR 1.1 で
    `/auth/me` の Cookie 動線を保持する方針なので、README 上で `GET /api/users/me` との
    棲み分け（モバイルクライアントは `/auth/me` を呼ばず `/api/users/me` を使う）が読者に
    伝わるよう補足した。
  - **`GET /api/items/{id}` の説明文に `feed_title` / `feed_favicon_url` の追加を補足**:
    task 7 で実装する記事詳細応答の拡張内容を README 上でも示し、mobile-api-contract.md §5.3
    と整合させた。
  - **Mobile API Contract Document への参照は blockquote 形式**: 既存 README のスタイル
    （`NEXT_PUBLIC_API_URL は廃止しました。` のような注釈が blockquote で記述されている）と
    揃え、API エンドポイント全体に対する補足説明としての位置付けを明示。リンク文字列は
    repository ルート相対パスでマークダウンリンクとして記述（GitHub UI でも閲覧可能）。
- 残存課題: なし（README のみの変更で Go コード / テストは未変更。`go test` / `go vet` は
  挙動を変えないため本 task では実行不要。後続 task 3〜7 で実装変更時に verify される）。

### Task 3
- 採用方針: design.md L348-377「user.Service.GetByID（新規）」節と既存 `Withdraw` の
  lookup ロジック（service.go L171 / L252 / L425-428）を踏襲し、`txBeginner != nil` ⇒
  `txUserDeleter.FindByID`、それ以外 ⇒ `userRepo.FindByID` の二択で薄い wrapper として
  実装した。nil ユーザーは `model.NewUserNotFoundError()`、repository error は
  `fmt.Errorf("ユーザーの取得に失敗しました: %w", err)` で wrap して返す（既存
  `withdrawLegacy` / `withdrawTx` の文言と完全一致）。
- 重要な判断:
  - **`Withdraw` の選択ロジックを `if s.txBeginner != nil` 単独で判定**: 既存 `Withdraw` は
    `withdrawTx` / `withdrawLegacy` の 2 メソッドへ分岐するが、`GetByID` は分岐先のロジック
    が「FindByID を 1 回呼ぶだけ」なので、別関数に分けずに 1 メソッド内で if-else 分岐する形に
    した。コード重複を避けつつ「同パターン」を保つ最小実装。design.md「実装は `s.userRepo.FindByID`
    を呼ぶだけ」「txBeginner パス時は `s.txUserDeleter.FindByID` を使う」の指針と整合。
  - **error wrap 文言は既存 `Withdraw` の "ユーザーの取得に失敗しました" を再利用**: 既存
    `withdrawLegacy` L173 / `withdrawTx` L173 と同一文言を採用し、運用ログ上で `Withdraw`
    と `GetByID` の lookup error が同じ書式で出るようにした。新しい文言を発明しない。
  - **doc comment で「認可を行わない理由」を明示**: design.md L362「ビジネス認可は実施しない
    （caller userID = lookup userID であり middleware で済んでいる）」をそのまま採用し、
    将来読者が「サービス層認可漏れではないか？」と疑わないようにした。CLAUDE.md「Backend §1.
    レイヤリングと依存方向」の「認可はサービス層に集約」の例外条件（middleware が解決した
    userID をそのまま使う read endpoint）を doc comment で説明する形。
  - **テストは subtest で「レガシー / tx 両パス」を 1 つの top-level test 関数に集約**:
    tasks.md L49「レガシーパスと txBeginner パスの両方で挙動が同一であることを確認」を、
    `t.Run` で 2 subtest に分離する形で実装。`TestService_GetByID_Success` /
    `_NotFound_ReturnsUserNotFoundError` / `_RepoError_PropagatesError` の 3 関数 ×
    レガシー・tx の 2 subtest = 計 6 ケース。既存 `TestService_Withdraw_Tx_*` が並列に並ぶ
    既存パターンと一貫させた。
  - **tx パスの subtest で `beginCalled` を assert**: `GetByID` は read-only lookup なので
    `txBeginner.BeginTx` を呼ばない（既存 `Withdraw` は `withdrawTx` でも user 存在確認は
    トランザクション外で実施するパターン / service.go L171）。本 task ではさらに lookup のみ
    なので一切 tx を開始しないことを `if beginner.beginCalled` で明示的に検証した。回帰防止。
  - **`mockUserRepo.findByIDFn` 内で受け取った id を assert**: 「caller userID = lookup userID」
    の前提が崩れないよう、subtest 内で `id != want.ID` のときに `t.Errorf` を発火させる
    軽い contract check を加えた（design.md「認可」節の前提が裏で破られないよう保護）。
- 残存課題: なし。後続 task 4 で `UserServiceInterface` 経由で `GetCurrent` adapter から
  本メソッドを呼び出す配線が入る（adapter は `a.svc.GetByID(ctx, userID)` を呼んで
  `currentUserResponse` に変換 / tasks.md L62-64）。本 task では service 層に閉じた追加のみで
  外部公開 / 配線は触っていない。

### Task 4
- 採用方針: design.md「Components and Interfaces」節 UserHandler.GetCurrent / UserServiceAdapter.GetCurrent
  / router.go の認証必須グループ配下 `r.Route("/api/users", ...)` 追加の 4 箇所の変更指針を
  そのまま採用。`currentUserResponse` を `user_handler.go` 内 private 型として宣言し、
  既存 `UserHandler.Withdraw` の error pattern（`middleware.UserIDFromContext` 失敗 → 401
  UNAUTHORIZED / service エラー → `handleServiceError`）と完全同型で実装した。
- 重要な判断:
  - **`currentUserResponse` の宣言場所を `user_handler.go` 内に集約**: design.md L274-280 の
    interface 定義例と一致させ、handler package 内 private 型として 1 箇所に置いた。
    `service_adapter.go` の `UserServiceAdapter.GetCurrent` も同 package 内で型を参照
    できるため、`*currentUserResponse` を返り値型として直接利用できる（package 境界を跨がない）。
  - **`AvatarURL *string` で `omitempty` 採用 + adapter で常時 nil 固定**: Req 2.4「JSON null
    または応答からの省略」を「省略」側で実装した（design.md「設計判断」節と一致）。
    `*string` + `omitempty` の Go 標準挙動として nil なら JSON から省略され、非 nil
    （将来 OAuth `picture` claim 保存実装後）なら値が出力される。テストでは
    `TestUserHandler_GetCurrent_OmitsAvatarWhenNil` で nil ケースの省略を、
    `TestUserHandler_GetCurrent_OmitsSecrets` で非 nil ケース（`avatar_url` キーが
    出現する場合でも許可キー集合内）を担保した。
  - **既存 `TestUserHandler_Withdraw_*` パターンとテスト名・テスト構造を厳密に揃える**:
    Success / NoUserID_ReturnsUnauthorized / UserNotFound / InternalError の 4 パターンを
    Withdraw と同型で並べ、レビュワーが「対応する Withdraw 系テストを引き写した」と
    一目で理解できるようにした。追加分は OmitsSecrets と OmitsAvatarWhenNil の 2 件で、
    本 spec 固有の Req 2.3 / 2.4 / Security Considerations 検証として独立配置。
  - **OmitsSecrets テストの構造**: 「許可キー集合 {id, email, name, avatar_url} 以外は出ない」
    形で集合演算的に検査し、将来 currentUserResponse に追加フィールドが入った場合に
    レビュワーが当該テストを意識せざるを得ない gate を作った（CLAUDE.md「機能追加
    チェックリスト」の secret 露出禁止の運用上の保護線）。明示的な `forbidden` リスト
    （session_id / refresh_token / password / password_hash / access_token）の検査も
    別途追加し、grep でも検出可能な regression net を二重化。
  - **`router_full_test.go` で `TestRouter_GetUsersMe_RequiresAuth` と
    `TestNewRouter_UserRoutes_GetCurrentEndpoint` の 2 件を追加**: tasks.md L80-81 が指定する
    認証必須グループ確認（401）に加え、session 経路で 200 が返ることも同様の既存
    `TestNewRouter_UserRoutes_WithdrawEndpoint` パターンに揃えて追加した。createTestRouter の
    `mockAuthService.getCurrentUserFn` が既に user-test-1 を返すモック構成なので、新規モック
    を追加せずに動作する（`UserService: &mockUserService{}` の default `getCurrentFn`
    が `&currentUserResponse{ID: userID}` を返すよう mock 側を実装したため、Cookie 経路でも
    200 を返す）。
  - **`SetupUserRoutes` で `r.Get("/me", h.GetCurrent)` を `r.Delete("/me", h.Withdraw)`
    より前に登録**: chi は同一 path で method 別 handler を持つため順序は意味的に問題に
    ならないが、design.md File Structure Plan の記述順序（GetCurrent → Withdraw）に
    合わせ、可読性のため `r.Get` を先頭に置いた。`router.go` の認証必須グループ内も
    同様の順序にした。
- 残存課題: なし。本 task は handler / adapter / router の追加のみで `app.go` の wiring は
  既存 `UserServiceAdapter` の生成箇所のままで GetCurrent メソッドが追加で見える
  （adapter 拡張のみで配線変更不要）。`go test ./...` と `go vet ./...` は全て pass。
  後続 task 5（`/auth/me` Cookie 経路 non-regression テスト）/ task 6 / 7（記事詳細応答の
  feed_title / feed_favicon_url 追加）に本 task は影響を与えない。

### Task 5
- 採用方針: 既存 `mockAuthService` パターン + `json.NewDecoder(...).Decode(&body)` で応答を
  `map[string]interface{}` に decode し、サブテスト 2 件（`Cookie_Present_ReturnsExistingShape`
  / `NoCookie_ReturnsUnauthorized`）で `/auth/me` の Cookie 経路応答が本 spec 導入で
  変化していないことを保護する non-regression test を追加。`auth_handler.go` /
  `SetupAuthRoutes` は一切変更しない（task 本文の制約）。
- 重要な判断:
  - **キー集合の「厳密一致」を採用**: 既存 `TestAuthHandler_Me_Authenticated_ReturnsUserJSON`
    は status と Content-Type のみを検査するが、本テストでは `len(body) != len(allowed)`
    によって応答キー集合が **厳密に** `{id, email, name}` のみであることを assert する。
    将来 `model.User` に `AvatarURL` フィールドが追加されたり、`/auth/me` handler の
    map literal に新フィールドが紛れ込んだ場合に、本テストが落ちることで mobile-api-contract.md
    §2.2「`GET /api/users/me`（モバイル/Web 共通）と `GET /auth/me`（Web Cookie 専用）の
    棲み分け」が壊れる regression を機械的に検出できる gate になる。
  - **既存 `TestAuthHandler_Me_Authenticated_ReturnsUserJSON` との重複を許容**: 両テストの
    観点は分離している（既存 = status/Content-Type のみ・本テスト = 応答本文のキー集合まで踏み込む
    non-regression）。task 5 本文「**既存形** `{id, email, name}` を含み、本 spec 導入前と
    同一であることを assert」「`avatar_url` のような新フィールドが `/auth/me` から漏れて
    返らないことも合わせて assert」が要求する 2 つの観点は、既存テストでは満たせないため
    新規追加が必要だった。
  - **禁止キーの明示 enum 列挙で grep 可能な regression net を二重化**: `len(body)` ベースの
    集合演算的検査に加え、`forbidden := []string{"avatar_url", "session_id", "refresh_token",
    "password", "password_hash", "access_token"}` の明示 enum を追加 assert。前者は厳密一致で
    確実に検出するが、後者は `grep -r 'session_id\|refresh_token\|avatar_url' internal/handler`
    で「secret 漏出 / 新フィールド漏出を検知する gate がどこにあるか」を運用者が grep で
    見つけられるようにする目的（task 4 の `TestUserHandler_GetCurrent_OmitsSecrets` と
    同じ二重化パターン）。
  - **`keysOf` ヘルパーは既存 `integration_test.go` のものを再利用**: 同 package 内に既存の
    `keysOf(m map[string]any) []string`（integration_test.go:2040）が存在するため、新規定義は
    重複宣言コンパイルエラーになる。私が一旦追加した重複定義を削除し、既存ヘルパーを利用する
    形に修正した（`any` は `interface{}` のエイリアスなので `map[string]interface{}` を渡せる）。
  - **`session_id` Cookie 値の assert**: mock `getCurrentUserFn` 内で受け取った `sessionID` が
    request Cookie 値 `"valid-session"` と一致することを軽い contract check として確認。Cookie
    の値が service 層に正しく渡される配線も併せて保護する。
- 残存課題: なし。task 6 / 7 は記事詳細応答の `feed_title` / `feed_favicon_url` 追加だが、
  `/auth/me`（本 task の保護対象）と `/api/items/{id}`（task 7 の拡張対象）は完全に異なる
  endpoint であり、本 task の non-regression gate は互いに干渉しない。`go test ./internal/handler/...`
  と `go vet ./...` は全て pass。
