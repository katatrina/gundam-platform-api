-- First drop all foreign key constraints
ALTER TABLE "wallet_transactions" DROP CONSTRAINT IF EXISTS "wallet_transactions_wallet_id_fkey";
ALTER TABLE "wallets" DROP CONSTRAINT IF EXISTS "wallets_user_id_fkey";
ALTER TABLE "shipments" DROP CONSTRAINT IF EXISTS "shipments_order_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT IF EXISTS "order_items_gundam_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT IF EXISTS "order_items_order_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT IF EXISTS "orders_seller_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT IF EXISTS "orders_buyer_id_fkey";
ALTER TABLE "gundam_images" DROP CONSTRAINT IF EXISTS "gundam_images_gundam_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT IF EXISTS "gundams_category_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT IF EXISTS "gundams_owner_id_fkey";
ALTER TABLE "user_addresses" DROP CONSTRAINT IF EXISTS "user_addresses_user_id_fkey";

-- Drop indexes
DROP INDEX IF EXISTS "wallet_transactions_wallet_id_idx";
DROP INDEX IF EXISTS "wallets_user_id_idx";
DROP INDEX IF EXISTS "user_addresses_user_id_is_primary_idx";

-- Drop all tables in reverse order
DROP TABLE IF EXISTS "wallet_transactions";
DROP TABLE IF EXISTS "wallets";
DROP TABLE IF EXISTS "shipments";
DROP TABLE IF EXISTS "order_items";
DROP TABLE IF EXISTS "orders";
DROP TABLE IF EXISTS "gundam_images";
DROP TABLE IF EXISTS "gundams";
DROP TABLE IF EXISTS "gundam_categories";
DROP TABLE IF EXISTS "user_addresses";
DROP TABLE IF EXISTS "users";

-- Drop custom types
DROP TYPE IF EXISTS "user_role";