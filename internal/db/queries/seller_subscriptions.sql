-- name: CreateTrialSubscriptionForSeller :exec
WITH trial_plan AS (SELECT id
                    FROM subscription_plans
                    WHERE name = 'GÓI DÙNG THỬ' LIMIT 1
    )
INSERT
INTO seller_subscriptions (seller_id,
                           plan_id,
                           start_date,
                           end_date,
                           is_active)
VALUES (
    $1, (SELECT id FROM trial_plan), now(), NULL, true
    );

-- name: CreateSellerSubscription :one
INSERT INTO seller_subscriptions (seller_id,
                                  plan_id,
                                  start_date,
                                  end_date,
                                  listings_used,
                                  open_auctions_used,
                                  is_active)
VALUES ($1, $2, NOW(), $3, $4, $5, $6) RETURNING *;

-- name: GetCurrentActiveSubscriptionDetailsForSeller :one
SELECT ss.id,
       ss.plan_id,
       p.name  AS subscription_name,
       p.price AS subscription_price,
       ss.seller_id,
       p.max_listings,
       ss.listings_used,
       p.max_open_auctions,
       ss.open_auctions_used,
       ss.is_active,
       p.is_unlimited,
       p.duration_days,
       ss.start_date,
       ss.end_date
FROM seller_subscriptions ss
         JOIN subscription_plans p ON ss.plan_id = p.id
WHERE ss.seller_id = $1
  AND ss.is_active = true;

-- name: UpdateCurrentActiveSubscriptionForSeller :one
UPDATE seller_subscriptions
SET listings_used      = COALESCE(sqlc.narg('listings_used'), listings_used),
    open_auctions_used = COALESCE(sqlc.narg('open_auctions_used'), open_auctions_used),
    is_active          = COALESCE(sqlc.narg('is_active'), is_active),
    updated_at         = now()
WHERE id = sqlc.arg('subscription_id')
  AND seller_id = sqlc.arg('seller_id')
  AND is_active = true RETURNING *;

-- name: ListSubscriptionHistory :many
SELECT ss.id,
       ss.plan_id,
       p.name  AS subscription_name,
       p.price AS subscription_price,
       ss.start_date,
       ss.end_date,
       ss.is_active,
       ss.created_at
FROM seller_subscriptions ss
         JOIN subscription_plans p ON ss.plan_id = p.id
WHERE ss.seller_id = $1
ORDER BY ss.created_at DESC;