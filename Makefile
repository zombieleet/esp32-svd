SVDTOOLS ?= svdtools

CHIPS := $(sort $(patsubst esp-pacs/%/svd,%,$(wildcard esp-pacs/*/svd)))

all: $(patsubst %,svd/%.svd,$(CHIPS))

# esp32c6-lp copies PMU from the patched esp32c6 SVD.
svd/esp32c6-lp.svd: svd/esp32c6.svd

svd/%.svd: patch
	@mkdir -p svd
	$(SVDTOOLS) patch esp-pacs/$*/svd/patches/$*.yaml
	mv esp-pacs/$*/svd/$*.base.svd.patched esp-pacs/$*/svd/$*.svd
	cp esp-pacs/$*/svd/$*.svd $@
	if [ -f tinygo/$*.yaml ]; then $(SVDTOOLS) patch tinygo/$*.yaml && mv $@.patched $@; fi
	go run flatten/main.go $@

# Apply changes to .yaml files.
patch:
	go run patch.go

.PHONY: all patch
