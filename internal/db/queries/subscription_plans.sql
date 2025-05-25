-- name: ListSubscriptionPlans :many
SELECT * FROM subscription_plans
ORDER BY price ASC;

-- name: GetSubscriptionPlanByID :one
SELECT *
FROM subscription_plans
WHERE id = $1;