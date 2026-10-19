.PHONY: env up down logs test cover lint mocks

env:
	@test -f .env || { sed -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$$(openssl rand -hex 16)|" \
		-e "s|^JWT_SECRET=.*|JWT_SECRET=$$(openssl rand -hex 32)|" .env.example > .env && \
		echo ".env created"; }

up: env
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f app

test:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./service/... ./worker/...
	go tool cover -func=coverage.out

lint:
	go vet ./...

mocks:
	go run github.com/vektra/mockery/v3@v3.8.1
