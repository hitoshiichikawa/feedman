# Review Notes

<!-- idd-claude:review round=1 model=claude-opus-4-7 timestamp=2026-05-26T00:00:00Z -->

## Reviewed Scope

- Branch: claude/issue-21-impl--html
- HEAD commit: 53d2c674c2a55d9668ec078b1c954156ce933b04
- Compared to: develop..HEAD

## Verified Requirements

- 1.1 — `item-detail.tsx` で `dangerouslySetInnerHTML={{ __html: sanitizedContent }}`（`useMemo(() => sanitizeContentHtml(item.content))`）に差し替え。`item-detail.test.tsx`「記事本文にscript要素が含まれるときサニタイズされて描画されること」で検証
- 1.2 — 描画経路を `sanitizeContentHtml` 通過に統一し生 HTML を直接挿入しない（`item-detail.tsx`）。script 除去テストで生 HTML 非挿入を裏取り
- 1.3 — `sanitize.test.ts`「空文字列のとき空文字列を返すこと」/ `item-detail.test.tsx`「記事本文が空文字列のとき空のコンテンツ領域を表示すること」（`innerHTML` が空）
- 1.4 — `sanitize.test.ts`「同一入力に対して常に同一のサニタイズ結果を返すこと」（first===second、再適用も安定）
- 2.1 — `sanitize.ts` の `ALLOWED_TAGS` に `script` 不在で除去。`sanitize.test.ts`「script要素…除去」/ `item-detail.test.tsx` script 除去テスト
- 2.2 — `iframe`/`style` も許可タグ外で除去。`sanitize.test.ts`「iframe要素…除去」「style要素…除去」
- 2.3 — `ALLOWED_ATTR` に `on*` 不在で除去。`sanitize.test.ts`「onerror属性…除去」「onclick属性…除去」/ `item-detail.test.tsx` onerror テスト
- 2.4 — `ALLOWED_URI_REGEXP` で `javascript:` 無効化。`sanitize.test.ts`「aタグのhrefにjavascriptスキームが含まれるとき…無効化または除去」
- 3.1 — `ALLOWED_TAGS`（p, br, a, ul, ol, li, blockquote, pre, code, strong, em, img）保持。`sanitize.test.ts`「許可タグのみで構成されるとき…保持」
- 3.2 — `sanitize.test.ts`「https URLを持つリンク…保持」「https srcを持つ画像…保持」
- 3.3 — `sanitize.ts` の `ALLOWED_TAGS`/`ALLOWED_ATTR` をバックエンド bluemonday 許可タグと一致させて整合範囲を維持。3.1/3.2 テストで保持を裏取り
- NFR 1.1 — `internal/security/content_sanitizer.go` 含むバックエンドは差分なし（`git diff --stat develop..HEAD -- internal/` が空）
- NFR 1.2 — `item-detail.tsx` の記事本文描画を `sanitizeContentHtml` 通過の単一経路に統一
- NFR 2.1 — 既存 10 ケース（`item-detail.test.tsx`）は不変で pass（許可タグの視覚的等価性を維持）

## Findings

なし

## Summary

requirements.md の全 numeric ID（1.1〜3.3 / NFR 1.1, 1.2, 2.1）に対応する実装とテストを `web/src/lib/sanitize.ts` `web/src/lib/sanitize.test.ts` `web/src/components/item-detail.tsx` `web/src/components/item-detail.test.tsx` で確認した。変更は `web/`（記事展開表示と新規サニタイズユーティリティ）に閉じ、バックエンドサニタイザは未変更で Out of Scope への逸脱なし。確認事項の URL スキーム差は要件 3.3「整合する範囲に維持」と Out of Scope 第4項（ポリシー単一化除外）の範囲内で AC 違反に当たらない。

RESULT: approve
