DB_URL=postgresql://root:secret@localhost:5432/gundam_platform?sslmode=disable

migrate-create:
	migrate create -ext sql -dir internal/db/migrations -seq -digits 2 $(NAME)

migrate-up:
	migrate -path internal/db/migrations -database "postgresql://root:secret@localhost:5432/gundam_platform?sslmode=disable" -verbose up

migrate-down:
	migrate -path internal/db/migrations -database "postgresql://root:secret@localhost:5432/gundam_platform?sslmode=disable" -verbose down

sqlc:
	sqlc generate

compose:
	docker compose down
	docker rmi -f gundam_platform-api:latest
	docker compose up --build
