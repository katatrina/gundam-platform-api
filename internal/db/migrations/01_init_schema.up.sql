CREATE TABLE "users"
(
    id                bigserial PRIMARY KEY,
    "hashed_password" text        NOT NULL,
    "email"           text UNIQUE NOT NULL,
    "role"            text        NOT NULL DEFAULT 'user',
    "created_at"      timestamptz NOT NULL DEFAULT (now())
);
