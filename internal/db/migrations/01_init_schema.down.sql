-- Drop all foreign key constraints first
ALTER TABLE "wallet_transactions" DROP CONSTRAINT IF EXISTS "wallet_transactions_wallet_id_fkey";
ALTER TABLE "wallets" DROP CONSTRAINT IF EXISTS "wallets_user_id_fkey";
ALTER TABLE "shipments" DROP CONSTRAINT IF EXISTS "shipments_order_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT IF EXISTS "order_items_gundam_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT IF EXISTS "order_items_order_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT IF EXISTS "orders_seller_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT IF EXISTS "orders_buyer_id_fkey";
ALTER TABLE "user_subscriptions" DROP CONSTRAINT IF EXISTS "user_subscriptions_plan_id_fkey";
ALTER TABLE "user_subscriptions" DROP CONSTRAINT IF EXISTS "user_subscriptions_user_id_fkey";
ALTER TABLE "cart_items" DROP CONSTRAINT IF EXISTS "cart_items_gundam_id_fkey";
ALTER TABLE "cart_items" DROP CONSTRAINT IF EXISTS "cart_items_cart_id_fkey";
ALTER TABLE "carts" DROP CONSTRAINT IF EXISTS "carts_user_id_fkey";
ALTER TABLE "gundam_images" DROP CONSTRAINT IF EXISTS "gundam_images_gundam_id_fkey";
ALTER TABLE "gundam_accessories" DROP CONSTRAINT IF EXISTS "gundam_accessories_gundam_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT IF EXISTS "gundams_grade_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT IF EXISTS "gundams_owner_id_fkey";
ALTER TABLE "user_addresses" DROP CONSTRAINT IF EXISTS "user_addresses_user_id_fkey";

-- Drop all indexes
DROP INDEX IF EXISTS "wallet_transactions_wallet_id_idx";
DROP INDEX IF EXISTS "wallets_user_id_idx";
DROP INDEX IF EXISTS "idx_user_active_subscription";
DROP INDEX IF EXISTS "cart_items_cart_id_gundam_id_idx";
DROP INDEX IF EXISTS "user_addresses_user_id_is_pickup_address_idx";
DROP INDEX IF EXISTS "user_addresses_user_id_is_primary_idx";

-- Drop all tables
DROP TABLE IF EXISTS "wallet_transactions";
DROP TABLE IF EXISTS "wallets";
DROP TABLE IF EXISTS "shipments";
DROP TABLE IF EXISTS "order_items";
DROP TABLE IF EXISTS "orders";
DROP TABLE IF EXISTS "user_subscriptions";
DROP TABLE IF EXISTS "subscription_plans";
DROP TABLE IF EXISTS "cart_items";
DROP TABLE IF EXISTS "carts";
DROP TABLE IF EXISTS "gundam_images";
DROP TABLE IF EXISTS "gundam_grades";
DROP TABLE IF EXISTS "gundam_accessories";
DROP TABLE IF EXISTS "gundams";
DROP TABLE IF EXISTS "user_addresses";
DROP TABLE IF EXISTS "users";

-- Drop all custom types
DROP TYPE IF EXISTS "payment_method";
DROP TYPE IF EXISTS "order_status";
DROP TYPE IF EXISTS "gundam_status";
DROP TYPE IF EXISTS "gundam_scale";
DROP TYPE IF EXISTS "gundam_condition";
DROP TYPE IF EXISTS "user_role";