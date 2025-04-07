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

-- name: GetCurrentActiveSubscriptionDetailsForSeller :one
SELECT ss.id,
       ss.plan_id,
       p.name AS plan_name,
       ss.seller_id,
       p.max_listings,
       ss.listings_used,
       p.max_open_auctions,
       ss.open_auctions_used,
       ss.is_active,
       p.is_unlimited,
       ss.end_date
FROM seller_subscriptions ss
         JOIN subscription_plans p ON ss.plan_id = p.id
WHERE ss.seller_id = $1
  AND ss.is_active = true
ORDER BY ss.start_date DESC LIMIT 1;

-- name: UpdateCurrentActiveSubscriptionForSeller :exec
UPDATE seller_subscriptions
SET listings_used = COALESCE(sqlc.narg('listings_used'), listings_used),
    updated_at    = now()
WHERE id = sqlc.arg('subscription_id')
  AND seller_id = sqlc.arg('seller_id')
  AND is_active = true;