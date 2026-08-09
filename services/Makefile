.PHONY: proto-check proto-generate compose-config

# Stage 1.1 records the toolchain entry points. Implementations are added in
# the corresponding later stages after the Go/Python services are initialized.
proto-check:
	buf lint proto

proto-generate:
	buf generate proto

compose-config:
	docker compose --env-file .env.docker.example -f docker-compose.yaml config
