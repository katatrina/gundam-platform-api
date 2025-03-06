CREATE TYPE "user_role" AS ENUM (
  'member',
  'seller',
  'moderator',
  'admin'
);

CREATE TYPE "gundam_condition" AS ENUM (
  'new',
  'open box',
  'second hand'
);

CREATE TYPE "gundam_scale" AS ENUM (
  '1/144',
  '1/100',
  '1/60',
  '1/48'
);

CREATE TYPE "gundam_status" AS ENUM (
  'available',
  'selling',
  'auction',
  'exchange'
);

CREATE TYPE "order_status" AS ENUM (
  'pending',
  'packaging',
  'delivering',
  'successful',
  'failed',
  'canceled'
);

CREATE TYPE "payment_method" AS ENUM (
  'cod',
  'wallet'
);

CREATE TABLE "users"
(
    "id"                    text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "full_name"             text,
    "hashed_password"       text,
    "email"                 text UNIQUE NOT NULL,
    "email_verified"        bool        NOT NULL DEFAULT false,
    "phone_number"          text UNIQUE,
    "phone_number_verified" bool        NOT NULL DEFAULT false,
    "role"                  user_role   NOT NULL DEFAULT 'member',
    "avatar_url"            text,
    "created_at"            timestamptz NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz NOT NULL DEFAULT (now())
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
    "condition"             gundam_condition NOT NULL,
    "condition_description" text,
    "manufacturer"          text             NOT NULL,
    "weight"                bigint           NOT NULL,
    "scale"                 gundam_scale     NOT NULL,
    "description"           text             NOT NULL,
    "price"                 bigint           NOT NULL,
    "status"                gundam_status    NOT NULL DEFAULT 'available',
    "created_at"            timestamptz      NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz      NOT NULL DEFAULT (now()),
    "deleted_at"            timestamptz
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

CREATE TABLE "user_subscriptions"
(
    "id"                 bigserial PRIMARY KEY,
    "user_id"            text        NOT NULL,
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
    "id"             bigserial PRIMARY KEY,
    "buyer_id"       text           NOT NULL,
    "seller_id"      text           NOT NULL,
    "total_price"    bigint         NOT NULL,
    "status"         order_status   NOT NULL DEFAULT 'pending',
    "payment_method" payment_method NOT NULL,
    "note"           text,
    "cancel_reason"  text,
    "created_at"     timestamptz    NOT NULL DEFAULT (now()),
    "updated_at"     timestamptz    NOT NULL DEFAULT (now())
);

CREATE TABLE "order_items"
(
    "id"         bigserial PRIMARY KEY,
    "order_id"   bigint      NOT NULL,
    "gundam_id"  bigint      NOT NULL,
    "price"      bigint      NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "shipments"
(
    "id"               bigserial PRIMARY KEY,
    "order_id"         bigint,
    "tracking_code"    text        NOT NULL,
    "shipping_address" text        NOT NULL,
    "shipping_method"  text        NOT NULL,
    "status"           text        NOT NULL,
    "shipping_cost"    bigint      NOT NULL,
    "created_at"       timestamptz NOT NULL DEFAULT (now()),
    "updated_at"       timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "wallets"
(
    "id"         bigserial PRIMARY KEY,
    "user_id"    text UNIQUE NOT NULL,
    "balance"    bigint      NOT NULL DEFAULT 0,
    "currency"   text        NOT NULL DEFAULT 'VND',
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "wallet_transactions"
(
    "id"               bigserial PRIMARY KEY,
    "wallet_id"        bigint      NOT NULL,
    "transaction_type" text        NOT NULL,
    "amount"           bigint      NOT NULL,
    "description"      text,
    "status"           text        NOT NULL DEFAULT 'pending',
    "created_at"       timestamptz NOT NULL DEFAULT (now()),
    "updated_at"       timestamptz NOT NULL DEFAULT (now())
);

CREATE INDEX ON "user_addresses" ("user_id", "is_primary");

CREATE INDEX ON "user_addresses" ("user_id", "is_pickup_address");

CREATE UNIQUE INDEX ON "cart_items" ("cart_id", "gundam_id");

CREATE INDEX "idx_user_active_subscription" ON "user_subscriptions" ("user_id", "is_active");

CREATE INDEX ON "wallets" ("user_id");

CREATE INDEX ON "wallet_transactions" ("wallet_id");

ALTER TABLE "user_addresses"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("owner_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("grade_id") REFERENCES "gundam_grades" ("id");

ALTER TABLE "gundam_accessories"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

ALTER TABLE "gundam_images"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

ALTER TABLE "carts"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE;

ALTER TABLE "cart_items"
    ADD FOREIGN KEY ("cart_id") REFERENCES "carts" ("id");

ALTER TABLE "cart_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id") ON DELETE CASCADE;

ALTER TABLE "user_subscriptions"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "user_subscriptions"
    ADD FOREIGN KEY ("plan_id") REFERENCES "subscription_plans" ("id");

ALTER TABLE "orders"
    ADD FOREIGN KEY ("buyer_id") REFERENCES "users" ("id");

ALTER TABLE "orders"
    ADD FOREIGN KEY ("seller_id") REFERENCES "users" ("id");

ALTER TABLE "order_items"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id");

ALTER TABLE "order_items"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

ALTER TABLE "shipments"
    ADD FOREIGN KEY ("order_id") REFERENCES "orders" ("id");

ALTER TABLE "wallets"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "wallet_transactions"
    ADD FOREIGN KEY ("wallet_id") REFERENCES "wallets" ("id");
