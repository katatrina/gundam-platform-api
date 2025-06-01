-- name: GetModPendingAuctionRequestsCount :one
-- Metric 1: Yêu cầu đấu giá chờ moderator duyệt (bảng auction_requests)
SELECT COUNT(*) as count
FROM auction_requests
WHERE status = 'pending';

-- name: GetModPendingWithdrawalRequestsCount :one
-- Metric 2: Yêu cầu rút tiền chờ moderator xử lý (bảng withdrawal_requests)
SELECT COUNT(*) as count
FROM withdrawal_requests
WHERE status = 'pending';

-- name: GetModTotalExchangesThisWeek :one
-- Metric 3: Tất cả trao đổi được tạo tuần này - VOLUME INDICATOR (bảng exchanges)
SELECT COUNT(*) as count
FROM exchanges
WHERE created_at >= date_trunc('week', CURRENT_DATE);

-- name: GetModTotalOrdersThisWeek :one
-- Metric 4: Tất cả đơn hàng được tạo tuần này - VOLUME INDICATOR (bảng orders)
SELECT COUNT(*) as count
FROM orders
WHERE created_at >= date_trunc('week', CURRENT_DATE);