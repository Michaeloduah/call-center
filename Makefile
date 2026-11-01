# Developer commands. Backend env vars are loaded from backend/.env (copy backend/.env.example).
SHELL := /bin/bash
ENV := set -a && source backend/.env && set +a

.PHONY: up down migrate migrate-down migrate-status api platform-admin test web

up: ## Start Postgres and Mailpit
	docker compose -f deploy/docker-compose.yml up -d

down: ## Stop dev services
	docker compose -f deploy/docker-compose.yml down

migrate: ## Apply all migrations
	$(ENV) && cd backend && go run ./cmd/cli migrate up

migrate-down: ## Roll back the latest migration
	$(ENV) && cd backend && go run ./cmd/cli migrate down

migrate-status:
	$(ENV) && cd backend && go run ./cmd/cli migrate status

api: ## Run the Go API on :8080
	$(ENV) && cd backend && go run ./cmd/api

platform-admin: ## make platform-admin EMAIL=you@example.com NAME="Your Name"
	$(ENV) && cd backend && go run ./cmd/cli create-platform-admin -email "$(EMAIL)" -name "$(NAME)"

test: ## Run backend tests (integration tests need `make up`)
	cd backend && TEST_DATABASE_URL=postgres://callcenter:callcenter@localhost:55432/callcenter_test?sslmode=disable \
		TEST_APP_DATABASE_URL=postgres://callcenter_app:callcenter_app@localhost:55432/callcenter_test?sslmode=disable \
		go test ./...

web: ## Run the Next.js app on :3100
	cd web && npm run dev
