-- name: CreateTrialSubscription :exec
WITH trial_plan AS (SELECT id
                    FROM subscription_plans
                    WHERE name = 'GÓI DÙNG THỬ' LIMIT 1
    )
INSERT
INTO user_subscriptions (user_id,
                         plan_id,
                         start_date,
                         end_date,
                         is_active)
VALUES (
    $1, (SELECT id FROM trial_plan), now(), NULL, true
    );
