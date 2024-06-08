wit_files = $(sort $(shell find testdata -name '*.wit' ! -name '*.golden.*'))

.PHONY: json
json: $(wit_files)

.PHONY: $(wit_files)
$(wit_files):
	wasm-tools component wit -j $@ > $@.json
