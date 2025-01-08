migrate-create:
	migrate create -ext sql -dir internal/db/migrations -seq -digits 2 $(NAME)

sqlc:
	sqlc generate

compose-down:
	docker compose down
	docker rmi -f gundam_platform-api:latest
