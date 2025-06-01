-- name: GetModPendingAuctionRequestsCount :one
-- Metric 1: Yêu cầu đấu giá chờ moderator duyệt (bảng auction_requests)
SELECT COUNT(*) as count
FROM auction_requests
WHERE status = 'pending';