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

-- name: GetPublishedGundamsCount :one
-- KPI 1: Số Gundam đang đăng bán
SELECT COUNT(*) as count
FROM gundams
WHERE owner_id = $1 AND status = 'published';


-- name: GetSellerTotalIncome :one
-- KPI 2: Tổng thu nhập từ bán hàng và đấu giá
SELECT COALESCE(SUM(amount), 0) ::bigint as total_income
FROM wallet_entries
WHERE wallet_id = $1
  AND entry_type IN ('payment_received', 'auction_seller_payment')
  AND status = 'completed';

-- name: GetCompletedOrdersCount :one
-- KPI 3: Số đơn hàng đã hoàn thành
SELECT COUNT(*) as count
FROM orders
WHERE seller_id = $1 AND status = 'completed';

-- name: GetProcessingOrdersCount :one
-- KPI 4: Số đơn hàng đang xử lý
SELECT COUNT(*) as count
FROM orders
WHERE seller_id = $1 AND status IN ('pending', 'packaging', 'delivering');

-- name: GetIncomeThisMonth :one
-- Bổ sung 1: Thu nhập tháng này
SELECT COALESCE(SUM(amount), 0)::bigint as income_this_month
FROM wallet_entries
WHERE wallet_id = $1
  AND entry_type IN ('payment_received', 'auction_seller_payment')
  AND status = 'completed'
  AND created_at >= date_trunc('month', CURRENT_DATE);

-- name: GetActiveAuctionsCount :one
-- Bổ sung 2: Số phiên đấu giá đang diễn ra
SELECT COUNT(*) as count
FROM auctions
WHERE seller_id = $1 AND status = 'active';

-- name: GetPendingAuctionRequestsCount :one
-- Bổ sung 3: Số yêu cầu đấu giá chờ duyệt
SELECT COUNT(*) as count
FROM auction_requests
WHERE seller_id = $1 AND status = 'pending';