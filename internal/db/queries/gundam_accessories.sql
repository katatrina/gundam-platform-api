-- name: DeleteAllGundamAccessories :exec
DELETE FROM gundam_accessories
WHERE gundam_id = $1;