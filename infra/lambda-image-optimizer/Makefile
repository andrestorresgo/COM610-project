# ── image-optimizer Lambda — build & deploy helpers ──────────────────────────
#
# chai2010/webp wraps libwebp via CGO, so we CANNOT use CGO_ENABLED=0.
# Cross-compiling CGO to Linux/x86_64 requires the correct sysroot and
# cross-compiler — the cleanest solution is a Docker build that matches
# the Lambda runtime environment exactly.
#
# Requirements:
#   - Docker (for `make build` / `make all`)
#   - AWS CLI configured with deployment permissions (for `make deploy`)
#   - zip utility
#
# Usage:
#   make build    → compile inside Docker (Linux/amd64, CGO against libwebp)
#   make zip      → package bootstrap into deployment.zip
#   make deploy   → upload zip to an existing Lambda function
#   make all      → build + zip

BINARY   := bootstrap
FUNCTION := image-optimizer
IMAGE    := image-optimizer-builder

.PHONY: all build zip deploy clean docker-image

all: build zip

# ── Docker build image ────────────────────────────────────────────────────────
# Uses the official Amazon Linux 2023 base so the glibc version and
# libwebp headers exactly match what will run in the provided.al2023 Lambda runtime.
docker-image:
	@echo "→ Building Docker builder image..."
	docker build --platform linux/amd64 -t $(IMAGE) -f Dockerfile.build .
	@echo "✓ Builder image ready"

# ── Compile inside Docker ─────────────────────────────────────────────────────
build: docker-image
	@echo "→ Compiling $(BINARY) for linux/amd64 with CGO..."
	docker run --rm --platform linux/amd64 \
		-v "$(PWD)":/workspace \
		-w /workspace \
		$(IMAGE) \
		sh -c 'CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
		       go build -tags lambda.norpc -ldflags="-s -w" -o $(BINARY) .'
	@echo "✓ Build complete"

zip:
	@echo "→ Packaging deployment.zip..."
	zip -j deployment.zip $(BINARY)
	@echo "✓ deployment.zip ready ($$(du -sh deployment.zip | cut -f1))"

deploy: zip
	@echo "→ Deploying to Lambda function: $(FUNCTION)..."
	aws lambda update-function-code \
		--function-name $(FUNCTION) \
		--zip-file fileb://deployment.zip \
		--architectures x86_64
	@echo "✓ Deployed"

clean:
	rm -f $(BINARY) deployment.zip
	docker rmi -f $(IMAGE) 2>/dev/null || true