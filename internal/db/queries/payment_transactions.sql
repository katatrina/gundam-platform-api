CREATE TABLE "payment_transactions"
(
    "id"                      bigserial PRIMARY KEY,
    "user_id"                 text                         NOT NULL,
    "amount"                  bigint                       NOT NULL,
    "provider"                payment_transaction_provider NOT NULL,
    "provider_transaction_id" text                         NOT NULL,
    "status"                  text                         NOT NULL DEFAULT 'pending',
    "metadata"                jsonb,
    "created_at"              timestamptz                  NOT NULL DEFAULT (now()),
    "updated_at"              timestamptz                  NOT NULL DEFAULT (now())
);

-- name: CreatePaymentTransaction :one
INSERT INTO payment_transactions (id,
                                  user_id,
                                  amount,
                                  provider,
                                  provider_transaction_id,
                                  status,
                                  metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

