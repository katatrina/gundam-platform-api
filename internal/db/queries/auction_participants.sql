-- name: CreateAuctionParticipant :one
INSERT INTO auction_participants (id,
                                  auction_id,
                                  user_id,
                                  deposit_amount,
                                  deposit_entry_id)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetAuctionParticipantByUserID :one
SELECT *
FROM auction_participants
WHERE user_id = $1
  AND auction_id = $2;;

-- name: ListAuctionParticipants :many
SELECT *
FROM auction_participants
WHERE auction_id = $1
ORDER BY created_at DESC;

-- name: ListAuctionParticipantsExcept :many
SELECT *
FROM auction_participants
WHERE auction_id = $1
  AND user_id != $2
ORDER BY created_at DESC;

-- name: UpdateAuctionParticipant :one
UPDATE auction_participants
SET is_refunded = COALESCE(sqlc.narg('is_refunded'), is_refunded),
    updated_at  = now()
WHERE id = $1 RETURNING *;