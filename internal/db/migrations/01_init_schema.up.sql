CREATE TYPE "user_role" AS ENUM (
  'buyer',
  'seller'
);

CREATE TABLE "users"
(
    "id"                    text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "full_name"             text        NOT NULL DEFAULT '',
    "hashed_password"       text,
    "email"                 text UNIQUE NOT NULL,
    "email_verified"        bool        NOT NULL DEFAULT false,
    "phone_number"          text UNIQUE,
    "phone_number_verified" bool        NOT NULL DEFAULT false,
    "role"                  user_role  NOT NULL DEFAULT 'buyer',
    "avatar_url"            text,
    "created_at"            timestamptz NOT NULL DEFAULT (now()),
    "updated_at"            timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "user_addresses"
(
    "id"         bigserial PRIMARY KEY,
    "user_id"    text        NOT NULL,
    "province"   text        NOT NULL,
    "district"   text        NOT NULL,
    "ward"       text        NOT NULL,
    "detail"     text        NOT NULL,
    "is_primary" boolean     NOT NULL DEFAULT false,
    "created_at" timestamptz NOT NULL DEFAULT (now()),
    "updated_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "gundams"
(
    "id"           bigserial PRIMARY KEY,
    "owner_id"     text        NOT NULL,
    "name"         text        NOT NULL,
    "category_id"  bigint      NOT NULL,
    "condition"    text        NOT NULL,
    "manufacturer" text        NOT NULL,
    "scale"        text        NOT NULL,
    "description"  text        NOT NULL DEFAULT '',
    "price"        bigint      NOT NULL,
    "status"       text        NOT NULL DEFAULT 'available',
    "created_at"   timestamptz NOT NULL DEFAULT (now()),
    "updated_at"   timestamptz NOT NULL DEFAULT (now()),
    "deleted_at"   timestamptz
);

CREATE TABLE "gundam_categories"
(
    "id"         bigserial PRIMARY KEY,
    "name"       text        NOT NULL,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "gundam_images"
(
    "id"         bigserial PRIMARY KEY,
    "gundam_id"  bigint      NOT NULL,
    "image_url"  text        NOT NULL,
    "is_primary" bool        NOT NULL DEFAULT false,
    "created_at" timestamptz NOT NULL DEFAULT (now())
);

CREATE TABLE "orders"
(
    "id"          bigserial PRIMARY KEY,
    "buyer_id"    text        NOT NULL,
    "seller_id"   text        NOT NULL,
    "total_price" bigint      NOT NULL,
    "status"      text        NOT NULL DEFAULT 'pending',
    "created_at"  timestamptz NOT NULL DEFAULT (now()),
    "updated_at"  timestamptz NOT NULL DEFAULT (now())
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

CREATE INDEX ON "wallets" ("user_id");

CREATE INDEX ON "wallet_transactions" ("wallet_id");

ALTER TABLE "user_addresses"
    ADD FOREIGN KEY ("user_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("owner_id") REFERENCES "users" ("id");

ALTER TABLE "gundams"
    ADD FOREIGN KEY ("category_id") REFERENCES "gundam_categories" ("id");

ALTER TABLE "gundam_images"
    ADD FOREIGN KEY ("gundam_id") REFERENCES "gundams" ("id");

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
