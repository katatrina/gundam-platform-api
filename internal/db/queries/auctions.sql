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
SET status                  = COALESCE(sqlc.narg('status'), status),
    current_price           = COALESCE(sqlc.narg('current_price'), current_price),
    winning_bid_id          = COALESCE(sqlc.narg('winning_bid_id'), winning_bid_id),
    winner_payment_deadline = COALESCE(sqlc.narg('winner_payment_deadline'), winner_payment_deadline),
    actual_end_time         = COALESCE(sqlc.narg('actual_end_time'), actual_end_time),
    updated_at              = now()
WHERE id = $1 RETURNING *;

-- name: GetAuctionByID :one
SELECT *
FROM auctions
WHERE id = $1;

-- name: GetAuctionByIDForUpdate :one
SELECT *
FROM auctions
WHERE id = $1
    FOR UPDATE;

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

-- name: IncrementAuctionParticipants :one
UPDATE auctions
SET total_participants = total_participants + 1
WHERE id = $1 RETURNING *;

-- name: IncrementAuctionTotalBids :one
UPDATE auctions
SET total_bids = total_bids + 1
WHERE id = $1 RETURNING *;

-- name: ListUserParticipatedAuctions :many
SELECT sqlc.embed(a),
       sqlc.embed(ap)
FROM auctions a
         JOIN auction_participants ap ON a.id = ap.auction_id
WHERE ap.user_id = $1
ORDER BY CASE a.status
             -- Phiên đang diễn ra: ưu tiên theo thời gian kết thúc gần nhất
             WHEN 'active' THEN EXTRACT(EPOCH FROM a.end_time)
             -- Phiên sắp diễn ra: ưu tiên theo thời gian bắt đầu sớm nhất
             WHEN 'scheduled' THEN EXTRACT(EPOCH FROM a.start_time)
             -- Các trạng thái khác: sắp xếp theo thời gian tạo mới nhất
             ELSE EXTRACT(EPOCH FROM a.created_at) * -1
             END
    ASC,
         a.created_at DESC;

-- name: ListSellerAuctions :many
SELECT *
FROM auctions
WHERE seller_id = $1
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY CASE status
             -- Phiên đang diễn ra: ưu tiên theo thời gian kết thúc gần nhất
             WHEN 'active' THEN EXTRACT(EPOCH FROM end_time)
             -- Phiên sắp diễn ra: ưu tiên theo thời gian bắt đầu sớm nhất
             WHEN 'scheduled' THEN EXTRACT(EPOCH FROM start_time)
             -- Các trạng thái khác: sắp xếp theo thời gian tạo mới nhất
             ELSE EXTRACT(EPOCH FROM created_at) * -1
             END ASC,
         created_at DESC;
