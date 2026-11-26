SERVICES := ledger-api bill-scheduler
# Pinned: lint results should not change because a new linter release came out.
LINT_IMAGE := golangci/golangci-lint:v2.15.0

.PHONY: compose-up compose-down test test-integration lint e2e demo

compose-up:
	docker compose up -d --build --wait

compose-down:
	docker compose down -v

test:
	@for s in $(SERVICES); do (cd services/$$s && go test -race ./...) || exit 1; done

# Needs Docker: testcontainers starts a throwaway Postgres.
test-integration:
	@for s in $(SERVICES); do (cd services/$$s && go test -race -tags integration ./...) || exit 1; done

# Each service is its own Go module, so golangci-lint must run inside each directory.
lint:
	@for s in $(SERVICES); do \
		(cd services/$$s && go vet ./...) || exit 1; \
		docker run --rm -v "$(CURDIR)/services/$$s:/app" -w /app $(LINT_IMAGE) golangci-lint run || exit 1; \
	done

# compose-up --wait only knows containers are running (distroless images have no
# healthcheck), so poll /readyz from the host before starting the tests.
# -count=1: never serve a cached result; the point is to hit the live stack.
e2e: compose-up
	curl -sf --retry 30 --retry-delay 1 --retry-connrefused --retry-all-errors http://localhost:8080/readyz > /dev/null
	cd tests/e2e && BASE_URL=http://localhost:8080 REDELIVER_CMD="$(CURDIR)/scripts/redeliver.sh compose" go test -tags e2e -count=1 -v ./...

demo:
	scripts/demo.sh
