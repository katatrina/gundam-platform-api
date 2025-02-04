-- Xóa các ràng buộc khóa ngoại
ALTER TABLE "wallet_transactions" DROP CONSTRAINT "wallet_transactions_wallet_id_fkey";
ALTER TABLE "wallets" DROP CONSTRAINT "wallets_user_id_fkey";
ALTER TABLE "shipments" DROP CONSTRAINT "shipments_order_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT "order_items_gundam_id_fkey";
ALTER TABLE "order_items" DROP CONSTRAINT "order_items_order_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT "orders_seller_id_fkey";
ALTER TABLE "orders" DROP CONSTRAINT "orders_buyer_id_fkey";
ALTER TABLE "gundam_images" DROP CONSTRAINT "gundam_images_gundam_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT "gundams_grade_id_fkey";
ALTER TABLE "gundams" DROP CONSTRAINT "gundams_owner_id_fkey";
ALTER TABLE "user_addresses" DROP CONSTRAINT "user_addresses_user_id_fkey";

-- Xóa các chỉ mục
DROP INDEX IF EXISTS "user_addresses_user_id_is_primary_idx";
DROP INDEX IF EXISTS "wallets_user_id_idx";
DROP INDEX IF EXISTS "wallet_transactions_wallet_id_idx";

-- Xóa các bảng
DROP TABLE IF EXISTS "wallet_transactions";
DROP TABLE IF EXISTS "wallets";
DROP TABLE IF EXISTS "shipments";
DROP TABLE IF EXISTS "order_items";
DROP TABLE IF EXISTS "orders";
DROP TABLE IF EXISTS "gundam_images";
DROP TABLE IF EXISTS "gundam_grades";
DROP TABLE IF EXISTS "gundams";
DROP TABLE IF EXISTS "user_addresses";
DROP TABLE IF EXISTS "users";

-- Xóa các ENUM types
DROP TYPE IF EXISTS "gundam_condition";
DROP TYPE IF EXISTS "user_role";