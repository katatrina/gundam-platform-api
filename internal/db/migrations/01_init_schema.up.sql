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
  'pending_auction_approval',
  'auctioning',
  'for exchange',
  'exchanging'
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

CREATE TYPE "order_type" AS ENUM (
  'regular',
  'exchange',
  'auction'
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
  'refund',
  'hold_funds',
  'release_funds',
  'exchange_compensation_hold',
  'exchange_compensation_transfer',
  'exchange_compensation_release',
  'auction_deposit',
  'auction_deposit_refund',
  'auction_compensation',
  'auction_winner_payment',
  'auction_seller_payment'
);

CREATE TYPE "wallet_reference_type" AS ENUM (
  'order',
  'auction',
  'withdrawal_request',
  'deposit_request',
  'exchange'
);

CREATE TYPE "wallet_entry_status" AS ENUM (
  'pending',
  'completed',
  'canceled',
  'failed'
);

CREATE TYPE "wallet_affected_field" AS ENUM (
  'balance',
  'non_withdrawable_amount',
  'both'
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

CREATE TYPE "exchange_post_status" AS ENUM (
  'open',
  'closed'
);

CREATE TYPE "exchange_status" AS ENUM (
  'pending',
  'packaging',
  'delivering',
  'delivered',
  'completed',
  'canceled',
  'failed'
);

CREATE TYPE "auction_request_status" AS ENUM (
  'pending',
  'approved',
  'rejected'
);

CREATE TYPE "auction_status" AS ENUM (
  'scheduled',
  'active',
  'ended',
  'completed',
  'canceled',
  'failed'
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

CREATE TABLE "seller_profiles"
(
    "seller_id"  text PRIMARY KEY,
    "shop_name"  text        NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
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
    "series"                text             NOT NULL,
    "parts_total"           bigint           NOT NULL,
    "material"              text             NOT NULL,
    "version"               text             NOT NULL,
    "quantity"              bigint           NOT NULL DEFAULT 1,
    "condition"             gundam_condition NOT NULL,
    "condition_description" text,
    "manufacturer"          text             NOT NULL,
    "weight"                bigint           NOT NULL,
    "scale"                 gundam_scale     NOT NULL,
    "description"           text             NOT NULL,
    "price"                 bigint,
    "release_year"          bigint,
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
    "id"                   uuid PRIMARY KEY,
    "code"                 text UNIQUE    NOT NULL,
    "buyer_id"             text           NOT NULL,
    "seller_id"            text           NOT NULL,
    "items_subtotal"       bigint         NOT NULL,
    "delivery_fee"         bigint         NOT NULL,
    "total_amount"         bigint         NOT NULL,
    "status"               order_status   NOT NULL DEFAULT 'pending',
    "payment_method"       payment_method NOT NULL,
    "type"                 order_type     NOT NULL DEFAULT 'regular',
    "note"                 text,
    "is_packaged"          bool           NOT NULL DEFAULT false,
    "packaging_image_urls" text[],
    "canceled_by"          text,
    "canceled_reason"      text,
    "created_at"           timestamptz    NOT NULL DEFAULT (now()),
    "updated_at"           timestamptz    NOT NULL DEFAULT (now()),
    "completed_at"         timestamptz
);

CREATE TABLE "order_items"
(
    "id"         bigserial PRIMARY KEY,
    "order_id"   uuid        NOT NULL,
    "gundam_id"  bigint,
    "name"       text        NOT NULL,
    "slug"       text        NOT NULL,
    "grade"      text        NOT NULL,
    "scale"      text        NOT NULL,
    "quantity"   bigint      NOT NULL DEFAULT 1,
    "price"      bigint      NOT NULL,
    "weight"     bigint      NOT NULL,
    "image_url"  text        NOT NULL,
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
    "order_id"               uuid        NOT NULL,
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
    "user_id"                 text PRIMARY KEY,
    "balance"                 bigint      NOT NULL DEFAULT 0,
    "non_withdrawable_amount" bigint      NOT NULL DEFAULT 0,
    "currency"                text        NOT NULL DEFAULT 'VND',
    "created_at"              timestamptz NOT NULL DEFAULT (now()),
    "updated_at"              timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "wallet_entries"
(
    "id"             bigserial PRIMARY KEY,
    "wallet_id"      text                  NOT NULL,
    "reference_id"   text,
    "reference_type" wallet_reference_type NOT NULL,
    "entry_type"     wallet_entry_type     NOT NULL,
    "affected_field" wallet_affected_field NOT NULL DEFAULT 'balance',
    "amount"         bigint                NOT NULL,
    "status"         wallet_entry_status   NOT NULL DEFAULT 'pending',
    "created_at"     timestamptz           NOT NULL DEFAULT (now()),
    "updated_at"     timestamptz           NOT NULL DEFAULT (now()),
    "completed_at"   timestamptz
);

CREATE TABLE "order_transactions"
(
    "id"              bigserial PRIMARY KEY,
    "order_id"        uuid                     NOT NULL,
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

CREATE TABLE "exchange_posts"
(
    "id"              uuid PRIMARY KEY,
    "user_id"         text                 NOT NULL,
    "content"         text                 NOT NULL,
    "post_image_urls" text[] NOT NULL,
    "status"          exchange_post_status NOT NULL DEFAULT 'open',
    "created_at"      timestamptz          NOT NULL DEFAULT (now()),
    "updated_at"      timestamptz          NOT NULL DEFAULT (now())
);

CREATE TABLE "exchange_post_items"
(
    "id"         uuid PRIMARY KEY,
    "post_id"    uuid        NOT NULL,
    "gundam_id"  bigint      NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "exchange_offers"
(
    "id"                    uuid PRIMARY KEY,
    "post_id"               uuid        NOT NULL,
    "offerer_id"            text        NOT NULL,
    "payer_id"              text,
    "compensation_amount"   bigint,
    "note"                  text,
    "negotiations_count"    bigint      NOT NULL DEFAULT 0,
    "max_negotiations"      bigint      NOT NULL DEFAULT 3,
    "negotiation_requested" bool        NOT NULL DEFAULT false,
    "last_negotiation_at"   timestamptz,
    "created_at"            timestamptz NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "exchange_offer_notes"
(
    "id"         uuid PRIMARY KEY,
    "offer_id"   uuid        NOT NULL,
    "user_id"    text        NOT NULL,
    "content"    text        NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "exchange_offer_items"
(
    "id"             uuid PRIMARY KEY,
    "offer_id"       uuid        NOT NULL,
    "gundam_id"      bigint      NOT NULL,
    "is_from_poster" bool        NOT NULL DEFAULT false,
    "created_at"     timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "exchanges"
(
    "id"                                   uuid PRIMARY KEY,
    "poster_id"                            text            NOT NULL,
    "offerer_id"                           text            NOT NULL,
    "poster_order_id"                      uuid,
    "offerer_order_id"                     uuid,
    "poster_from_delivery_id"              bigint,
    "poster_to_delivery_id"                bigint,
    "offerer_from_delivery_id"             bigint,
    "offerer_to_delivery_id"               bigint,
    "poster_delivery_fee"                  bigint,
    "offerer_delivery_fee"                 bigint,
    "poster_delivery_fee_paid"             bool            NOT NULL DEFAULT false,
    "offerer_delivery_fee_paid"            bool            NOT NULL DEFAULT false,
    "poster_order_expected_delivery_time"  timestamptz,
    "offerer_order_expected_delivery_time" timestamptz,
    "poster_order_note"                    text,
    "offerer_order_note"                   text,
    "payer_id"                             text,
    "compensation_amount"                  bigint,
    "status"                               exchange_status NOT NULL DEFAULT 'pending',
    "canceled_by"                          text,
    "canceled_reason"                      text,
    "created_at"                           timestamptz     NOT NULL DEFAULT (now()),
    "updated_at"                           timestamptz     NOT NULL DEFAULT (now()),
    "completed_at"                         timestamptz
);

CREATE TABLE "exchange_items"
(
    "id"             uuid PRIMARY KEY,
    "exchange_id"    uuid        NOT NULL,
    "gundam_id"      bigint,
    "name"           text        NOT NULL,
    "slug"           text        NOT NULL,
    "grade"          text        NOT NULL,
    "scale"          text        NOT NULL,
    "quantity"       bigint      NOT NULL DEFAULT 1,
    "weight"         bigint      NOT NULL,
    "image_url"      text        NOT NULL,
    "owner_id"       text,
    "is_from_poster" bool        NOT NULL DEFAULT false,
    "created_at"     timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "auction_requests"
(
    "id"              uuid PRIMARY KEY,
    "gundam_id"       bigint,
    "seller_id"       text                   NOT NULL,
    "gundam_snapshot" jsonb                  NOT NULL,
    "starting_price"  bigint                 NOT NULL,
    "bid_increment"   bigint                 NOT NULL,
    "buy_now_price"   bigint,
    "deposit_rate"    numeric(5, 4)          NOT NULL DEFAULT 0.15,
    "deposit_amount"  bigint                 NOT NULL,
    "start_time"      timestamptz            NOT NULL,
    "end_time"        timestamptz            NOT NULL,
    "status"          auction_request_status NOT NULL DEFAULT 'pending',
    "rejected_by"     text,
    "rejected_reason" text,
    "approved_by"     text,
    "created_at"      timestamptz            NOT NULL DEFAULT (now()),
    "updated_at"      timestamptz            NOT NULL DEFAULT (now())
);

CREATE TABLE "auctions"
(
    "id"                      uuid PRIMARY KEY,
    "request_id"              uuid UNIQUE,
    "gundam_id"               bigint,
    "seller_id"               text           NOT NULL,
    "gundam_snapshot"         jsonb          NOT NULL,
    "starting_price"          bigint         NOT NULL,
    "bid_increment"           bigint         NOT NULL,
    "winning_bid_id"          uuid,
    "buy_now_price"           bigint,
    "start_time"              timestamptz    NOT NULL,
    "end_time"                timestamptz    NOT NULL,
    "actual_end_time"         timestamptz,
    "status"                  auction_status NOT NULL DEFAULT 'scheduled',
    "current_price"           bigint         NOT NULL,
    "deposit_rate"            numeric(5, 4)  NOT NULL,
    "deposit_amount"          bigint         NOT NULL,
    "total_participants"      int            NOT NULL DEFAULT 0,
    "total_bids"              int            NOT NULL DEFAULT 0,
    "winner_payment_deadline" timestamptz,
    "order_id"                uuid,
    "canceled_by"             text,
    "canceled_reason"         text,
    "created_at"              timestamptz    NOT NULL DEFAULT (now()),
    "updated_at"              timestamptz    NOT NULL DEFAULT (now())
);

CREATE TABLE "auction_bids"
(
    "id"             uuid PRIMARY KEY,
    "auction_id"     uuid,
    "bidder_id"      text,
    "participant_id" uuid        NOT NULL,
    "amount"         bigint      NOT NULL,
    "created_at"     timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "auction_participants"
(
    "id"               uuid PRIMARY KEY,
    "auction_id"       uuid        NOT NULL,
    "user_id"          text        NOT NULL,
    "deposit_amount"   bigint      NOT NULL,
    "deposit_entry_id" bigint      NOT NULL,
    "is_refunded"      bool        NOT NULL DEFAULT false,
    "created_at"       timestamptz NOT NULL DEFAULT (now()),
    "updated_at"       timestamptz NOT NULL DEFAULT (now())
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

CREATE INDEX ON "exchange_posts" ("user_id");

CREATE INDEX ON "exchange_posts" ("status");

CREATE INDEX ON "exchange_posts" ("created_at");

CREATE UNIQUE INDEX ON "exchange_post_items" ("post_id", "gundam_id");

CREATE INDEX ON "exchange_offers" ("post_id");

CREATE INDEX ON "exchange_offers" ("offerer_id");

CREATE INDEX ON "exchange_offers" ("created_at");

CREATE UNIQUE INDEX "unique_exchange_offer" ON "exchange_offers" ("post_id", "offerer_id");

CREATE INDEX ON "exchange_offer_notes" ("offer_id");

CREATE INDEX ON "exchange_offer_notes" ("user_id");

CREATE INDEX ON "exchange_offer_notes" ("created_at");

CREATE UNIQUE INDEX ON "exchange_offer_items" ("offer_id", "gundam_id");

CREATE UNIQUE INDEX ON "exchanges" ("poster_order_id");

CREATE UNIQUE INDEX ON "exchanges" ("offerer_order_id");

CREATE INDEX ON "exchanges" ("status");

CREATE UNIQUE INDEX ON "exchange_items" ("exchange_id", "gundam_id");

CREATE INDEX ON "auction_requests" ("seller_id", "status");

CREATE INDEX ON "auction_requests" ("status", "created_at");

CREATE INDEX ON "auction_requests" ("gundam_id", "status");

CREATE INDEX ON "auctions" ("status", "start_time");

CREATE INDEX ON "auctions" ("status", "end_time");

CREATE INDEX ON "auctions" ("seller_id", "status");

CREATE INDEX ON "auctions" ("gundam_id");

CREATE INDEX ON "auctions" ("current_price");

CREATE INDEX ON "auction_bids" ("auction_id", "bidder_id");

CREATE INDEX ON "auction_bids" ("auction_id", "amount");

CREATE INDEX ON "auction_bids" ("participant_id");

CREATE INDEX ON "auction_bids" ("bidder_id", "created_at");

CREATE INDEX ON "auction_bids" ("auction_id", "created_at");

CREATE UNIQUE INDEX ON "auction_participants" ("auction_id", "user_id");

CREATE INDEX ON "auction_participants" ("user_id", "created_at");

ALTER TABLE "seller_profiles"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id") ON DELETE CASCADE;

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

ALTER TABLE "orders"
    ADD FOREIGN KEY ("canceled_by") REFERENCES "users" ("id") ON DELETE SET NULL;

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
    ADD FOREIGN KEY ("wallet_id") REFERENCES "wallets" ("user_id") ON DELETE CASCADE;

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("buyer_entry_id") REFERENCES "wallet_entries" ("id");

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("seller_entry_id") REFERENCES "wallet_entries" ("id");

ALTER TABLE "order_transactions"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON DELETE CASCADE;

ALTER TABLE "payment_transactions"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "exchange_posts"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_post_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

ALTER TABLE "exchange_post_items"
    ADD FOREIGN KEY ("post_id") REFERENCES "exchange_posts" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offers"
    ADD FOREIGN KEY ("post_id") REFERENCES "exchange_posts" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offers"
    ADD FOREIGN KEY ("offerer_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offers"
    ADD FOREIGN KEY ("payer_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offer_notes"
    ADD FOREIGN KEY ("offer_id") REFERENCES "exchange_offers" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offer_notes"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_offer_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

ALTER TABLE "exchange_offer_items"
    ADD FOREIGN KEY ("offer_id") REFERENCES "exchange_offers" ("id") ON DELETE CASCADE;

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("poster_from_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("poster_to_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("offerer_from_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("offerer_to_delivery_id") REFERENCES "delivery_information" ("id");

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("payer_id") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("canceled_by") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("poster_order_id") REFERENCES "orders" ("id") ON DELETE SET NULL;

ALTER TABLE "exchanges"
    ADD FOREIGN KEY ("offerer_order_id") REFERENCES "orders" ("id") ON DELETE SET NULL;

ALTER TABLE "exchange_items"
    ADD FOREIGN KEY ("exchange_id") REFERENCES "exchanges" ("id") ON DELETE CASCADE;

ALTER TABLE "exchange_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE SET NULL;

ALTER TABLE "exchange_items"
    ADD FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_requests"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_requests"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "auction_requests"
    ADD FOREIGN KEY ("rejected_by") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_requests"
    ADD FOREIGN KEY ("approved_by") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("winning_bid_id") REFERENCES "auction_bids" ("id");

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("request_id") REFERENCES "auction_requests" ("id") ON DELETE SET NULL;

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE SET NULL;

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id") ON DELETE SET NULL;

ALTER TABLE "auctions"
    ADD FOREIGN KEY ("canceled_by") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_bids"
    ADD FOREIGN KEY ("auction_id") REFERENCES "auctions" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_bids"
    ADD FOREIGN KEY ("bidder_id") REFERENCES "users" ("id") ON DELETE SET NULL;

ALTER TABLE "auction_bids"
    ADD FOREIGN KEY ("participant_id") REFERENCES "auction_participants" ("id") ON DELETE CASCADE;

ALTER TABLE "auction_participants"
    ADD FOREIGN KEY ("deposit_entry_id") REFERENCES "wallet_entries" ("id");

ALTER TABLE "auction_participants"
    ADD FOREIGN KEY ("auction_id") REFERENCES "auctions" ("id") ON DELETE CASCADE;

ALTER TABLE "auction_participants"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;
