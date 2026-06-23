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
