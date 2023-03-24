
.PHONY: cross-build
cross-build:
	rm -rf _bin
	go get github.com/mitchellh/gox@v0.4.0
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/vela/{{.OS}}-{{.Arch}}/vela" -osarch='$(TARGETS)' ./references/cmd/cli
	$(GOBUILD_ENV) $(GOX) -ldflags $(LDFLAGS) -parallel=2 -output="_bin/kubectl-vela/{{.OS}}-{{.Arch}}/kubectl-vela" -osarch='$(TARGETS)' ./cmd/plugin

.PHONY: compress
compress:
	( \
		echo "\n## Release Info\nVERSION: $(VELA_VERSION)" >> README.md && \
		echo "GIT_COMMIT: $(GIT_COMMIT_LONG)\n" >> README.md && \
		cd _bin/vela && \
		$(DIST_DIRS) cp ../../LICENSE {} \; && \
		$(DIST_DIRS) cp ../../README.md {} \; && \
		$(DIST_DIRS) tar -zcf vela-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r vela-{}.zip {} \; && \
		cd ../kubectl-vela && \
		$(DIST_DIRS) cp ../../LICENSE {} \; && \
		$(DIST_DIRS) cp ../../README.md {} \; && \
		$(DIST_DIRS) tar -zcf kubectl-vela-{}.tar.gz {} \; && \
		$(DIST_DIRS) zip -r kubectl-vela-{}.zip {} \; && \
		cd .. && \
		sha256sum vela/vela-* kubectl-vela/kubectl-vela-* > sha256sums.txt \
	)
