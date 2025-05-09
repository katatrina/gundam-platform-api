-- name: CountSellerActiveAuctions :one
SELECT COUNT(*)
FROM auctions
WHERE seller_id = $1
  AND status IN ('scheduled', 'active');