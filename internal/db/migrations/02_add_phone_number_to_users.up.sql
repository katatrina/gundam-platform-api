ALTER TABLE "users"
    ADD COLUMN "phone_number" text UNIQUE;

ALTER TABLE "users"
    ADD COLUMN "phone_number_verified" bool NOT NULL DEFAULT false;
