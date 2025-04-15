-- name: GetGradeByID :one
SELECT *
FROM gundam_grades
WHERE id = $1;