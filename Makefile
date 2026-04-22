.PHONY: capture-refresh

# capture-refresh: re-run the dogfood capture driver and show the diff.
# Aborts if the working tree has uncommitted changes to runtime or SDK code.
capture-refresh:
	@if git diff --name-only HEAD | grep -qE '^(softprobe-runtime|softprobe-go|softprobe-js|softprobe-python|softprobe-java)/'; then \
		echo "ERROR: uncommitted changes to runtime or SDK code detected."; \
		echo "Commit or stash those changes before running capture-refresh."; \
		exit 1; \
	fi
	@bash spec/dogfood/capture.sh
	@git diff -- spec/examples/cases/control-plane-v1.case.json
