.PHONY: up down db-reset sqlc api seed test web loadtest

up:            ## start postgres + redis
	docker compose up -d --wait postgres redis

down:
	docker compose down

db-reset:      ## wipe the local database volume
	docker compose down -v && docker compose up -d --wait postgres redis

sqlc:
	cd backend && sqlc generate

api:           ## run the API locally (migrations apply on start)
	cd backend && go run ./cmd/api

seed:          ## publish demo events (idempotent)
	cd backend && go run ./cmd/seed

test:
	cd backend && go test ./... -count=1

web:
	cd frontend && npm run dev

loadtest:      ## k6 race + oversell + 60 s load (API must be running in development)
	cd loadtest && k6 run race_one_seat.js && k6 run ga_oversell.js && k6 run booking_load.js
