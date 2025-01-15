CREATE TABLE "users"
(
    "id"              text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "name"            text        NOT NULL DEFAULT '',
    "hashed_password" text        NOT NULL DEFAULT '',
    "email"           text UNIQUE NOT NULL,
    "email_verified"  bool        NOT NULL,
    "role"            text        NOT NULL DEFAULT 'buyer',
    "picture"         text        NOT NULL DEFAULT '',
    "created_at"      timestamptz NOT NULL DEFAULT (now())
);
