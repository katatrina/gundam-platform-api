-- Down Migration: Remove withdrawal system
-- Reverses the withdrawal system migration

-- 1. Drop foreign key constraints first
ALTER TABLE "withdrawal_requests" DROP CONSTRAINT IF EXISTS "withdrawal_requests_user_id_fkey";
ALTER TABLE "withdrawal_requests" DROP CONSTRAINT IF EXISTS "withdrawal_requests_bank_account_id_fkey";
ALTER TABLE "withdrawal_requests" DROP CONSTRAINT IF EXISTS "withdrawal_requests_processed_by_fkey";
ALTER TABLE "withdrawal_requests" DROP CONSTRAINT IF EXISTS "withdrawal_requests_wallet_entry_id_fkey";
ALTER TABLE "user_bank_accounts" DROP CONSTRAINT IF EXISTS "user_bank_accounts_user_id_fkey";

-- 2. Drop indexes
DROP INDEX IF EXISTS "user_bank_accounts_user_id_account_number_idx";
DROP INDEX IF EXISTS "withdrawal_requests_user_id_status_idx";
DROP INDEX IF EXISTS "withdrawal_requests_status_created_at_idx";

-- 3. Drop tables
DROP TABLE IF EXISTS "withdrawal_requests";
DROP TABLE IF EXISTS "user_bank_accounts";

-- 4. Drop enum type
DROP TYPE IF EXISTS "withdrawal_request_status";