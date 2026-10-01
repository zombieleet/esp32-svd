# Patched SVD files for Espressif chips

This repository contains SVD files for Espressif chips, generated from [esp-pacs](https://github.com/esp-rs/esp-pacs), part of the [esp-rs](https://github.com/esp-rs) community. The base files come from [Espressif](https://www.espressif.com/) and esp-pacs applies patches to fix bugs and fill in gaps.

TinyGo uses these files to generate its `device/esp` package. The files in `svd/` are committed so building TinyGo does not require Rust or network access.

These files are not intended to be modified. Instead, they are intended to be used as any SVD file, for example to generate access to registers for various languages.

## Installation Requirements

- **svdtools**: Install with `cargo install --locked svdtools --version 0.5.0`
- **Git** with submodule support

The `svdtools` command must be available in your PATH, or passed to make as `make SVDTOOLS=/path/to/svdtools`.

## Contributing

Please do not contribute changes directly to the SVD files in this repository. Instead, contribute patches upstream in the [esp-pacs](https://github.com/esp-rs/esp-pacs) repository. Patches that are only needed by TinyGo go in `tinygo/<chip>.yaml`.

## Updating

 1. Make sure the esp-pacs submodule is pulled, using `git submodule update --init`.
 2. Download the latest patches by going to the esp-pacs subdirectory and running `git pull`.
 3. Run `make`. This first runs `go run patch.go`, which adds a `dimIndex` to the Wi-Fi clusters in esp-pacs so gen-device-svd can name them. Check `git -C esp-pacs diff` to ensure the modifications look plausible.

For each chip, `make` then does the following:

 1. Applies the esp-pacs patches with svdtools.
 2. Applies `tinygo/<chip>.yaml` with svdtools, if it exists. These patches fix registers that are wrong in esp-pacs, such as missing fields.
 3. Runs `go run flatten/main.go`, which rewrites the SVD file so gen-device-svd can generate every register:
     - Registers that use `derivedFrom` are expanded into full copies of their source register.
     - Clusters without `dim` nested inside an arrayed cluster are moved up into the parent, with the cluster name as a prefix. For example, `CH[n].IN_INT.ENA` becomes `CH[n].IN_INT_ENA`.

`flatten` stops with an error on anything it does not support, such as a `derivedFrom` chain. If that happens after an update, extend `flatten/main.go` or add a patch in `tinygo/`.

Please ensure that the ESP targets supported by TinyGo can still be built:

 1. Regenerate device files from the updated SVD files: `make gen-device-esp`
 2. Build src/examples/blinky1 for all ESP targets
 3. Try to resolve possible issues in `src/machine/machine_esp32*` and `src/runtime/runtime_esp32*`

## License

The SVD files are licensed under the Apache License, Version 2.0 by Espressif Systems. The esp-pacs patches are licensed under either of [Apache License, Version 2.0](LICENSE-APACHE) or [MIT license](LICENSE-MIT) at your option.
