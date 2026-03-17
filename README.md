# WinSysRoot

_Automatically assemble Windows Sysroots directly from Microsoft sources_

## Features

- Very small, minimal sysroots. The x64 tarball zstd-compressed is just 106MiB.
- Fully cross-platform, no proprietary code involved in the toolchain. Works with any
  properly-configured LLVM.
- Very fast. The compressed x64 tarball takes ~20s to download and assemble.
- Selectable Windows SDK version
- Integrated VFS overlay to make it work on case-sensitive filesystems.
- Near 100% compatibility with normal Microsoft MSVC. No Cygwin/MinGW/MSYS2.

## Installation

This requires an up-to-date Go toolchain, currently there are no precompiled binaries provided.

```sh
go install git.dolansoft.org/lorenz/winsysroot@latest
```

It also requires LLVM 15 or higher with lld-link, which you need to install for your platform.

## Usage

`winsysroot` is now an offline tool. It does not issue HTTP requests itself; you are expected to
download the Visual Studio installer manifest, the relevant Windows SDK MSI files, and the planned
CAB/VSIX payloads externally.

The primary workflow is:

```sh
winsysroot plan \
  --installer-manifest path/to/installer-manifest.json \
  --winsdk-msi-dir path/to/msis \
  --win-sdk-version 10.0.26100 \
  --architectures x64,arm64 \
  --out-manifest path/to/download-plan.json
```

followed by:

```sh
winsysroot assemble \
  --in-manifest path/to/download-plan.json \
  --winsdk-msi-dir path/to/msis \
  --downloads-dir path/to/downloads \
  --out-dir somewere/my-sysroot \
  --out-metadata path/to/versions.json
```

You can list SDK versions from a local installer manifest using:

```sh
winsysroot list-win-sdk-versions --installer-manifest path/to/installer-manifest.json
```

The full option list can be shown using `winsysroot help`.

Note that this does NOT need a case-insensitive directory on Linux/MacOS. It doesn't break it, but
it is also not required.

This sysroot can then be used either standalone or with the included wrapper scripts:

```sh
WINSYSROOT=somewere/my-sysroot wrappers/clang-cl-x64 /o examples/helloworld-x64.exe examples/helloworld.cc
```

If your clang-cl is not called `clang-cl`, you can set the `CLANG_CL` environment variable to what
it is in your environment.

## Notes

- arm64ec is VERY new and as of LLVM 15 does not fully work.
- The tarball has a hardcoded VFS location at /winsysroot which you need to change to the real
  unpacked path as the driver discovery does not go through the VFS overlay yet and thus passing
  /winsysroot as path doesn't work.

## Is this legal?

Probably. I'm not a lawyer and this is not legal advice, but I'm not distributing any
non-redistributable Microsoft content or circumventing any licensing schemes, you're downloading all
content directly from Microsoft's servers (just a lot more efficiently). Note that you cannot
legally distribute any sysroots generated from this tool without Microsoft's permission.
