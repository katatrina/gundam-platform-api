-- name: GetAdminTotalBusinessUsers :one
-- Metric 1: Tổng users hoạt động trên nền tảng (bảng users - không tính admin và moderator)
SELECT COUNT(*) as count
FROM users
WHERE deleted_at IS NULL
  AND role IN ('seller'
    , 'member');

-- name: GetAdminTotalRegularOrdersThisMonth :one
-- Metric 2: Tổng đơn hàng thường tháng này (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE type = 'regular'
  AND created_at >= date_trunc('month'
    , CURRENT_DATE);

-- name: GetAdminTotalExchangeOrdersThisMonth :one
-- Metric 3: Tổng đơn hàng trao đổi tháng này (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE type = 'exchange'
  AND created_at >= date_trunc('month'
    , CURRENT_DATE);

-- name: GetAdminTotalAuctionOrdersThisMonth :one
-- Metric 4: Tổng đơn hàng đấu giá tháng này (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE type = 'auction'
  AND created_at >= date_trunc('month'
    , CURRENT_DATE);

-- name: GetAdminTotalRevenueThisMonth :one
-- Doanh thu thực của nền tảng từ subscription payments
SELECT COALESCE(-SUM(amount), 0) ::bigint as revenue
FROM wallet_entries
WHERE entry_type = 'subscription_payment'
  AND status = 'completed'
  AND created_at >= date_trunc('month', CURRENT_DATE);

-- name: GetAdminCompletedExchangesThisMonth :one
-- Metric 6: Trao đổi hoàn thành thành công tháng này (bảng exchanges)
SELECT COUNT(*) as count
FROM exchanges
WHERE status = 'completed'
  AND completed_at >= date_trunc('month'
    , CURRENT_DATE);

-- name: GetAdminCompletedAuctionsThisWeek :one
-- Metric 7: Đấu giá hoàn thành thành công tuần này (bảng auctions)
SELECT COUNT(*) as count
FROM auctions
WHERE status = 'completed'
  AND updated_at >= date_trunc('week'
    , CURRENT_DATE);

-- name: GetAdminTotalWalletVolumeThisWeek :one
-- Metric 8: Tổng volume giao dịch ví thành công tuần này (bảng wallet_entries)
SELECT COALESCE(SUM(ABS(amount)), 0) ::bigint as total_volume
FROM wallet_entries
WHERE status = 'completed'
  AND created_at >= date_trunc('week', CURRENT_DATE);

-- name: GetAdminTotalPublishedGundams :one
-- Metric 9: Tổng Gundam đang bán trên nền tảng (bảng gundams)
SELECT COUNT(*) as count
FROM gundams
WHERE status = 'published';

-- name: GetAdminNewUsersThisWeek :one
-- Metric 10: Users mới đăng ký tuần này (bảng users)
SELECT COUNT(*) as count
FROM users
WHERE created_at >= date_trunc('week'
    , CURRENT_DATE)
  AND deleted_at IS NULL;