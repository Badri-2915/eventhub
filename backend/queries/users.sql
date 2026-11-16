-- name: UpsertUserByGoogleSub :one
INSERT INTO users (google_sub, email, name, avatar_url)
VALUES ($1, $2, $3, $4)
ON CONFLICT (google_sub) DO UPDATE
SET email = EXCLUDED.email, name = EXCLUDED.name, avatar_url = EXCLUDED.avatar_url
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: MakeHost :one
UPDATE users SET is_host = true WHERE id = $1 RETURNING *;
