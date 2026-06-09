.PHONY: demo up run down test

demo:
	docker compose --profile demo up --build --abort-on-container-exit client

up:
	docker compose up -d zap

run:
	docker compose --profile demo run --rm client

down:
	docker compose down --remove-orphans

test:
	docker compose --profile demo run --rm --no-deps client python -m unittest discover -s tests -v