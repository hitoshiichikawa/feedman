-- パスキー再認証時の credential backup 属性（BackupEligible / BackupState）不整合による
-- 400 拒否（Issue #234）を修正するため、passkey_credentials に BE/BS 2 列を追加する。
-- go-webauthn v0.17.4 の login validation（webauthn/login.go:371 の BE 一致判定）を通過させる
-- ために、登録時に authenticator が報告した BE/BS を永続化し認証時 lookup で復元する（Req 2.1）。
--
-- NFR 1.1: 既存列（id / user_id / credential_id / public_key / sign_count / attestation_type /
-- aaguid / transports / created_at / last_used_at）の削除・型変更・rename は行わない。
-- NFR 1.2: 追加列の既定値は安全側（false）とし、既存行には自動的に false が入る。
-- NFR 1.3: 本修正前に登録された行は BE=false / BS=false のまま読み出せ、アプリケーションを
-- 異常終了させない（結果として BE=1 assertion では不一致となるが、削除・再登録で復旧する運用）。
--
-- BE/BS は WHERE 句で使わないため追加インデックスは張らない。
-- ALTER TABLE ADD COLUMN ... NOT NULL DEFAULT false は PostgreSQL 11+ で全行 rewrite 無しで
-- 即時完了する（system catalog レベルで default を保持）ため、long-running lock の懸念はない。
ALTER TABLE passkey_credentials
    ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN backup_state    BOOLEAN NOT NULL DEFAULT false;
