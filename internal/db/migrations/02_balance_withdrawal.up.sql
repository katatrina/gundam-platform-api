-- 1. Create withdrawal_request_status enum
CREATE TYPE "withdrawal_request_status" AS ENUM (
  'pending',
  'approved',
  'completed',
  'rejected',
  'canceled'
);

-- 2. Create user_bank_accounts table
CREATE TABLE "user_bank_accounts"
(
    "id"              uuid PRIMARY KEY,
    "user_id"         text        NOT NULL,
    "account_name"    text        NOT NULL,
    "account_number"  text        NOT NULL,
    "bank_code"       text        NOT NULL,
    "bank_name"       text        NOT NULL,
    "bank_short_name" text        NOT NULL,
    "created_at"      timestamptz NOT NULL DEFAULT (now()),
    "updated_at"      timestamptz NOT NULL DEFAULT (now())
);

-- 3. Create withdrawal_requests table
CREATE TABLE "withdrawal_requests"
(
    "id"                    uuid PRIMARY KEY,
    "user_id"               text                      NOT NULL,
    "bank_account_id"       uuid                      NOT NULL,
    "amount"                bigint                    NOT NULL,
    "status"                withdrawal_request_status NOT NULL DEFAULT 'pending',
    "processed_by"          text,
    "processed_at"          timestamptz,
    "rejected_reason"       text,
    "transaction_reference" text,
    "wallet_entry_id"       bigint,
    "created_at"            timestamptz               NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz               NOT NULL DEFAULT (now()),
    "completed_at"          timestamptz
);

-- 5. Add foreign key constraints
ALTER TABLE "user_bank_accounts"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");
ALTER TABLE "withdrawal_requests"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");
ALTER TABLE "withdrawal_requests"
    ADD FOREIGN KEY ("bank_account_id") REFERENCES "user_bank_accounts" ("id");
ALTER TABLE "withdrawal_requests"
    ADD FOREIGN KEY ("processed_by") REFERENCES "users" ("id");
ALTER TABLE "withdrawal_requests"
    ADD FOREIGN KEY ("wallet_entry_id") REFERENCES "wallet_entries" ("id");