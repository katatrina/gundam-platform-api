CREATE TABLE "users"
(
    "id"              text PRIMARY KEY     DEFAULT (gen_random_uuid()),
    "name"            text        NOT NULL DEFAULT '',
    "hashed_password" text,
    "email"           text UNIQUE NOT NULL,
    "email_verified"  bool        NOT NULL,
    "role"            text        NOT NULL DEFAULT 'buyer',
    "avatar"         text,
    "created_at"      timestamptz NOT NULL DEFAULT (now())
);
