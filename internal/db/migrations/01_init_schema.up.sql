CREATE TYPE "user_role" AS ENUM (
  'member',
  'seller',
  'moderator',
  'admin'
);

CREATE TYPE "gundam_condition" AS ENUM (
  'new',
  'open box',
  'used'
);

CREATE TYPE "gundam_scale" AS ENUM (
  '1/144',
  '1/100',
  '1/60',
  '1/48'
);

CREATE TYPE "gundam_status" AS ENUM (
  'in store',
  'published',
  'processing',
  'pending auction approval',
  'auctioning'
);

CREATE TYPE "order_status" AS ENUM (
  'pending',
  'packaging',
  'delivering',
  'delivered',
  'completed',
  'failed',
  'canceled'
);

CREATE TYPE "payment_method" AS ENUM (
  'cod',
  'wallet'
);

CREATE TYPE "delivery_overral_status" AS ENUM (
  'picking',
  'delivering',
  'delivered',
  'failed',
  'return'
);

CREATE TYPE "wallet_entry_type" AS ENUM (
  'deposit',
  'withdrawal',
  'payment',
  'payment_received',
  'non_withdrawable',
  'refund',
  'refund_deduction',
  'auction_lock',
  'auction_release',
  'auction_payment',
  'platform_fee'
);

CREATE TYPE "wallet_reference_type" AS ENUM (
  'order',
  'auction',
  'withdrawal_request',
  'deposit_request',
  'promotion',
  'affiliate',
  'zalopay'
);

CREATE TYPE "wallet_entry_status" AS ENUM (
  'pending',
  'completed',
  'failed'
);

CREATE TYPE "order_transaction_status" AS ENUM (
  'pending',
  'completed',
  'refunded',
  'failed'
);

CREATE TYPE "payment_transaction_provider" AS ENUM (
  'zalopay'
);

CREATE TYPE "payment_transaction_status" AS ENUM (
  'pending',
  'completed',
  'failed'
);

CREATE TYPE "payment_transaction_type" AS ENUM (
  'wallet_deposit'
);

CREATE TABLE "users"
(
    "id"                    text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "google_account_id"     text,
    "full_name"             text        NOT NULL,
    "hashed_password"       text,
    "email"                 text UNIQUE NOT NULL,
    "email_verified"        bool        NOT NULL DEFAULT false,
    "phone_number"          text UNIQUE,
    "phone_number_verified" bool        NOT NULL DEFAULT false,
    "role"                  user_role   NOT NULL DEFAULT 'member',
    "avatar_url"            text,
    "created_at"            timestamptz NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz NOT NULL DEFAULT (now()),
    "deleted_at"            timestamptz
);

CREATE TABLE "user_addresses"
(
    "id"                bigserial PRIMARY KEY,
    "user_id"           text        NOT NULL,
    "full_name"         text        NOT NULL,
    "phone_number"      text        NOT NULL,
    "province_name"     text        NOT NULL,
    "district_name"     text        NOT NULL,
    "ghn_district_id"   bigint      NOT NULL,
    "ward_name"         text        NOT NULL,
    "ghn_ward_code"     text        NOT NULL,
    "detail"            text        NOT NULL,
    "is_primary"        boolean     NOT NULL DEFAULT false,
    "is_pickup_address" boolean     NOT NULL DEFAULT false,
    "created_at"        timestamptz NOT NULL DEFAULT (now()),
    "updated_at"        timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "gundams"
(
    "id"                    bigserial PRIMARY KEY,
    "owner_id"              text             NOT NULL,
    "name"                  text             NOT NULL,
    "slug"                  text UNIQUE      NOT NULL,
    "grade_id"              bigint           NOT NULL,
    "quantity"              bigint           NOT NULL DEFAULT 1,
    "condition"             gundam_condition NOT NULL,
    "condition_description" text,
    "manufacturer"          text             NOT NULL,
    "weight"                bigint           NOT NULL,
    "scale"                 gundam_scale     NOT NULL,
    "description"           text             NOT NULL,
    "price"                 bigint           NOT NULL,
    "status"                gundam_status    NOT NULL DEFAULT 'in store',
    "created_at"            timestamptz      NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz      NOT NULL DEFAULT (now())
);

CREATE TABLE "gundam_accessories"
(
    "id"         bigserial PRIMARY KEY,
    "name"       text        NOT NULL,
    "gundam_id"  bigint      NOT NULL,
    "quantity"   bigint      NOT NULL DEFAULT 1,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "gundam_grades"
(
    "id"           bigserial PRIMARY KEY,
    "name"         text        NOT NULL,
    "display_name" text        NOT NULL,
    "slug"         text UNIQUE NOT NULL,
    "created_at"   timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "gundam_images"
(
    "id"         bigserial PRIMARY KEY,
    "gundam_id"  bigint      NOT NULL,
    "url"        text        NOT NULL,
    "is_primary" bool        NOT NULL DEFAULT false,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "carts"
(
    "id"         bigserial PRIMARY KEY,
    "user_id"    text UNIQUE NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "cart_items"
(
    "id"         text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "cart_id"    bigint      NOT NULL,
    "gundam_id"  bigint      NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "subscription_plans"
(
    "id"                bigserial PRIMARY KEY,
    "name"              text        NOT NULL,
    "duration_days"     bigint,
    "max_listings"      bigint,
    "max_open_auctions" bigint,
    "is_unlimited"      bool        NOT NULL DEFAULT false,
    "price"             bigint      NOT NULL,
    "created_at"        timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "seller_subscriptions"
(
    "id"                 bigserial PRIMARY KEY,
    "seller_id"          text        NOT NULL,
    "plan_id"            bigint      NOT NULL,
    "start_date"         timestamptz NOT NULL DEFAULT (now()),
    "end_date"           timestamptz,
    "listings_used"      bigint      NOT NULL DEFAULT 0,
    "open_auctions_used" bigint      NOT NULL DEFAULT 0,
    "is_active"          bool        NOT NULL DEFAULT true,
    "created_at"         timestamptz NOT NULL DEFAULT (now()),
    "updated_at"         timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "orders"
(
    "id"               uuid PRIMARY KEY,
    "code"             text UNIQUE    NOT NULL,
    "buyer_id"         text           NOT NULL,
    "seller_id"        text           NOT NULL,
    "items_subtotal"   bigint         NOT NULL,
    "delivery_fee"     bigint         NOT NULL,
    "total_amount"     bigint         NOT NULL,
    "status"           order_status   NOT NULL DEFAULT 'pending',
    "payment_method"   payment_method NOT NULL,
    "note"             text,
    "is_packaged"      bool           NOT NULL DEFAULT false,
    "packaging_images" text[],
    "created_at"       timestamptz    NOT NULL DEFAULT (now()),
    "updated_at"       timestamptz    NOT NULL DEFAULT (now())
);

CREATE TABLE "order_items"
(
    "id"         bigserial PRIMARY KEY,
    "order_id"   text        NOT NULL,
    "gundam_id"  bigint      NOT NULL,
    "quantity"   bigint      NOT NULL DEFAULT 1,
    "price"      bigint      NOT NULL,
    "weight"     bigint      NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "delivery_information"
(
    "id"              bigserial PRIMARY KEY,
    "user_id"         text        NOT NULL,
    "full_name"       text        NOT NULL,
    "phone_number"    text        NOT NULL,
    "province_name"   text        NOT NULL,
    "district_name"   text        NOT NULL,
    "ghn_district_id" bigint      NOT NULL,
    "ward_name"       text        NOT NULL,
    "ghn_ward_code"   text        NOT NULL,
    "detail"          text        NOT NULL,
    "created_at"      timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "order_deliveries"
(
    "id"                     bigserial PRIMARY KEY,
    "order_id"               text        NOT NULL,
    "delivery_tracking_code" text,
    "expected_delivery_time" timestamptz NOT NULL,
    "status"                 text,
    "overall_status"         delivery_overral_status,
    "from_delivery_id"       bigint      NOT NULL,
    "to_delivery_id"         bigint      NOT NULL,
    "created_at"             timestamptz NOT NULL DEFAULT (now()),
    "updated_at"             timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "wallets"
(
    "id"                      bigserial PRIMARY KEY,
    "user_id"                 text UNIQUE NOT NULL,
    "balance"                 bigint      NOT NULL DEFAULT 0,
    "non_withdrawable_amount" bigint      NOT NULL DEFAULT 0,
    "currency"                text        NOT NULL DEFAULT 'VND',
    "created_at"              timestamptz NOT NULL DEFAULT (now()),
    "updated_at"              timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "wallet_entries"
(
    "id"             bigserial PRIMARY KEY,
    "wallet_id"      bigint                NOT NULL,
    "reference_id"   text,
    "reference_type" wallet_reference_type NOT NULL,
    "entry_type"     wallet_entry_type     NOT NULL,
    "amount"         bigint                NOT NULL,
    "status"         wallet_entry_status   NOT NULL DEFAULT 'pending',
    "created_at"     timestamptz           NOT NULL DEFAULT (now()),
    "updated_at"     timestamptz           NOT NULL DEFAULT (now()),
    "completed_at"   timestamptz
);

CREATE TABLE "order_transactions"
(
    "id"              bigserial PRIMARY KEY,
    "order_id"        text                     NOT NULL,
    "amount"          bigint                   NOT NULL,
    "status"          order_transaction_status NOT NULL DEFAULT 'pending',
    "buyer_entry_id"  bigint                   NOT NULL,
    "seller_entry_id" bigint,
    "created_at"      timestamptz              NOT NULL DEFAULT (now()),
    "updated_at"      timestamptz              NOT NULL DEFAULT (now()),
    "completed_at"    timestamptz
);

CREATE TABLE "payment_transactions"
(
    "id"                      bigserial PRIMARY KEY,
    "user_id"                 text                         NOT NULL,
    "amount"                  bigint                       NOT NULL,
    "transaction_type"        payment_transaction_type     NOT NULL,
    "provider"                payment_transaction_provider NOT NULL,
    "provider_transaction_id" text                         NOT NULL,
    "status"                  payment_transaction_status   NOT NULL DEFAULT 'pending',
    "metadata"                jsonb,
    "created_at"              timestamptz                  NOT NULL DEFAULT (now()),
    "updated_at"              timestamptz                  NOT NULL DEFAULT (now())
);

CREATE INDEX ON "user_addresses" ("user_id", "is_primary");

CREATE INDEX ON "user_addresses" ("user_id", "is_pickup_address");

CREATE UNIQUE INDEX "unique_cart_item" ON "cart_items" ("cart_id", "gundam_id");

CREATE INDEX "idx_seller_active_subscription" ON "seller_subscriptions" ("seller_id", "is_active");

CREATE INDEX ON "wallets" ("user_id");

CREATE INDEX ON "wallet_entries" ("wallet_id");

CREATE INDEX ON "wallet_entries" ("reference_id", "reference_type");

CREATE INDEX ON "order_transactions" ("order_id");

CREATE UNIQUE INDEX ON "payment_transactions" ("provider", "provider_transaction_id");

CREATE INDEX ON "payment_transactions" ("user_id", "status");

ALTER TABLE "user_addresses"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("owner_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("grade_id") REFERENCES "gundam_grades" ("id");

ALTER TABLE "gundam_accessories"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE CASCADE;

ALTER TABLE "gundam_images"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE CASCADE;

ALTER TABLE "carts"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "cart_items"
    ADD FOREIGN KEY ("cart_id") REFERENCES "carts" ("id") ON DELETE CASCADE;

ALTER TABLE "cart_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE CASCADE;

ALTER TABLE "seller_subscriptions"
    ADD FOREIGN KEY ("plan_id") REFERENCES "subscription_plans" ("id");

ALTER TABLE "seller_subscriptions"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "orders"
    ADD FOREIGN KEY ("buyer_id") REFERENCES "users" ("id");

ALTER TABLE "orders"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id");

ALTER TABLE "order_items"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON DELETE CASCADE;

ALTER TABLE "order_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE SET NULL;

ALTER TABLE "delivery_information"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "order_deliveries"
    ADD FOREIGN KEY ("from_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "order_deliveries"
    ADD FOREIGN KEY ("to_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "order_deliveries"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON DELETE CASCADE;

ALTER TABLE "wallets"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "wallet_entries"
    ADD FOREIGN KEY ("wallet_id") REFERENCES "wallets" ("id") ON DELETE CASCADE;

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("buyer_entry_id") REFERENCES "wallet_entries" ("id");

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("seller_entry_id") REFERENCES "wallet_entries" ("id");

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON DELETE CASCADE;

ALTER TABLE "payment_transactions"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");
