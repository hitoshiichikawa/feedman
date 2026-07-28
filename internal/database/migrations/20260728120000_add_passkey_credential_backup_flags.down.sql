-- Issue #234 で追加した passkey_credentials.backup_eligible / backup_state の rollback。
-- 追加した 2 列のみを DROP し、既存列（id / user_id / credential_id / public_key /
-- sign_count / attestation_type / aaguid / transports / created_at / last_used_at）には
-- 一切触れず、既存データを不変で復元する（up と同じ NFR 1.1 の考え方に従う）。
--
-- BE 対応コードは列不在時にコンパイル・実行に失敗するため、rollback を行う場合は
-- 「アプリケーションコードのロールバック → 本 migration の down 適用」の順で運用する。

ALTER TABLE passkey_credentials
    DROP COLUMN IF EXISTS backup_state,
    DROP COLUMN IF EXISTS backup_eligible;
