-- name: CountSellerActiveAuctions :one
SELECT COUNT(*)
FROM auctions
WHERE seller_id = $1
  AND status IN ('scheduled', 'active');

-- name: CreateAuction :one
INSERT INTO auctions (id,
                      request_id,
                      gundam_id,
                      seller_id,
                      gundam_snapshot,
                      starting_price,
                      bid_increment,
                      buy_now_price,
                      current_price,
                      deposit_rate,
                      deposit_amount,
                      start_time,
                      end_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING *;

-- name: UpdateAuction :one
UPDATE auctions
SET status             = COALESCE(sqlc.narg('status'), status),
    updated_at         = now()
WHERE id = $1 RETURNING *;

-- name: GetAuctionByID :one
SELECT *
FROM auctions
WHERE id = $1;

-- name: ListAuctions :many
SELECT *
FROM auctions
WHERE status = COALESCE(sqlc.narg('status'), status)
ORDER BY CASE status
             -- Phiên đang diễn ra: ưu tiên theo thời gian kết thúc gần nhất
             WHEN 'active' THEN EXTRACT(EPOCH FROM end_time)
             -- Phiên sắp diễn ra: ưu tiên theo thời gian bắt đầu sớm nhất
             WHEN 'scheduled' THEN EXTRACT(EPOCH FROM start_time)
             -- Các trạng thái khác: sắp xếp theo thời gian tạo mới nhất
             ELSE EXTRACT(EPOCH FROM created_at) * -1
             END ASC,
         created_at DESC;

-- name: CheckUserParticipation :one
SELECT EXISTS(SELECT 1
              FROM auction_participants
              WHERE auction_id = $1
                AND user_id = $2) AS "has_participated";