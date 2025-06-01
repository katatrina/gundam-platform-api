-- name: GetSellerByID :one
SELECT *
FROM users
WHERE id = $1
  AND role = 'seller';

-- name: ListSalesOrders :many
SELECT *
FROM orders
WHERE seller_id = $1
  AND type != 'exchange'
  AND status = COALESCE(sqlc.narg('status')::order_status, status)
ORDER BY updated_at DESC, created_at DESC;

-- name: GetSalesOrder :one
SELECT *
FROM orders
WHERE id = sqlc.arg('order_id')
  AND seller_id = sqlc.arg('seller_id')
  AND type != 'exchange';

-- name: UpdateSellerProfileByID :one
UPDATE seller_profiles
SET shop_name  = COALESCE(sqlc.narg('shop_name'), shop_name),
    updated_at = now()
WHERE seller_id = $1 RETURNING *;

-- name: GetSellerDetailByID :one
SELECT sqlc.embed(u),
       sqlc.embed(sp)
FROM users u
         JOIN seller_profiles sp ON u.id = sp.seller_id
WHERE u.id = $1;

-- name: GetSellerProfileByID :one
SELECT *
FROM seller_profiles
WHERE seller_id = $1;

-- name: CreateSellerProfile :one
INSERT INTO seller_profiles (seller_id, shop_name)
VALUES ($1, $2) RETURNING *;

-- name: GetSellerPublishedGundamsCount :one
-- Metric 1: Gundam đang bán của seller này (bảng gundams)
SELECT COUNT(*) as count
FROM gundams
WHERE owner_id = $1 AND status = 'published';

-- name: GetSellerTotalIncome :one
-- Metric 2: Tổng thu nhập của seller (bảng wallet_entries)
SELECT COALESCE(SUM(amount), 0) ::bigint as total_income
FROM wallet_entries
WHERE wallet_id = $1
  AND entry_type IN ('payment_received', 'auction_seller_payment')
  AND status = 'completed';

-- name: GetSellerCompletedOrdersCount :one
-- Metric 3: Đơn hàng hoàn thành của seller (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE seller_id = $1 AND status = 'completed';

-- name: GetSellerProcessingOrdersCount :one
-- Metric 4: Đơn hàng đang xử lý của seller (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE seller_id = $1 AND status IN ('pending', 'packaging', 'delivering');

-- name: GetSellerIncomeThisMonth :one
-- Metric 5: Thu nhập tháng này của seller (bảng wallet_entries)
SELECT COALESCE(SUM(amount), 0) ::bigint as income_this_month
FROM wallet_entries
WHERE wallet_id = $1
  AND entry_type IN ('payment_received', 'auction_seller_payment')
  AND status = 'completed'
  AND created_at >= date_trunc('month', CURRENT_DATE);

-- name: GetSellerActiveAuctionsCount :one
-- Metric 6: Đấu giá đang hoạt động của seller (bảng auctions)
SELECT COUNT(*) as count
FROM auctions
WHERE seller_id = $1 AND status = 'active';

-- name: GetSellerPendingAuctionRequestsCount :one
-- Metric 7: Yêu cầu đấu giá chờ duyệt của seller (bảng auction_requests)
SELECT COUNT(*) as count
FROM auction_requests
WHERE seller_id = $1 AND status = 'pending';