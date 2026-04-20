# Scrapfly Go SDK — release/dev Makefile.
# Target names mirror sdk/python & sdk/rust Makefiles for muscle-memory parity.
# Go modules are tag-published (no registry), so `release` = test + tag + push.

VERSION ?=
NEXT_VERSION ?=

.PHONY: init install dev bump generate-docs release fmt lint vet test

init:
	go env -w GOTOOLCHAIN=auto

install:
	go mod download
	go build ./...

dev:
	go build ./...
	go test -count=1 ./... -run '^$$'  # compile tests without running

bump:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make bump VERSION=x.y.z"; exit 2; fi
	@# No in-repo version constant for sdk/go; the module version IS the git tag.
	@# We still create a bump commit so the release loop stays uniform across SDKs.
	git commit --allow-empty -m "bump version to $(VERSION)"
	git push

generate-docs:
	@mkdir -p docs
	go doc -all ./... > docs/go-reference.txt

release:
	@if [ -z "$(VERSION)" ]; then echo "Usage: make release VERSION=x.y.z [NEXT_VERSION=x.y.(z+1)]"; exit 2; fi
	git branch | grep \* | cut -d ' ' -f2 | grep main || exit 1
	git pull origin main
	$(MAKE) vet
	$(MAKE) test
	$(MAKE) generate-docs
	-git add docs
	-git commit -m "Update Go reference for version $(VERSION)"
	-git push origin main
	git tag -a v$(VERSION) -m "Version $(VERSION)"
	git push --tags
	@if [ -n "$(NEXT_VERSION)" ]; then $(MAKE) bump VERSION=$(NEXT_VERSION); fi

fmt:
	gofmt -w .

lint: vet

vet:
	go vet ./...

test:
	go test -count=1 ./...
