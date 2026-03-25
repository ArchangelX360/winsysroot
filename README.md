# WinSysRoot

_Automatically assemble Windows Sysroots directly from Microsoft sources_

## Features

- Very small, minimal sysroots.
- Fully cross-platform, no proprietary code involved in the toolchain. Works with any
  properly-configured LLVM.
- Very fast for the amount of CAB and VSIX content it has to unpack.
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

`winsysroot` is now a low-level offline tool. It does not issue HTTP requests itself and it no
longer owns manifest resolution or download planning. The intended flow is that Bazel repository
rules or another external orchestrator decide what to download and what to extract, and `winsysroot`
only handles opaque Windows archive formats.

The primitive commands are:

```sh
winsysroot msi-info --input path/to/sdk.msi
winsysroot cab-extract --layout path/to/layout.json --out-dir path/to/sysroot --cab path/to/a.cab --cab path/to/b.cab
winsysroot zip-list --input path/to/toolset.vsix
winsysroot zip-extract --input path/to/toolset.vsix --layout path/to/layout.json --out-dir path/to/sysroot
winsysroot write-vfs --root-dir path/to/sysroot
```

`msi-info` prints CAB membership and MSI file mappings as JSON. `cab-extract` and `zip-extract`
consume a private JSON layout file with entries of the form:

```sh
{
  "entries": [
    {
      "archive_path": "ARCHIVE_MEMBER_NAME",
      "output_path": "relative/output/path"
    }
  ]
}
```

The full option list can be shown using `winsysroot help`. Note that this does NOT need a
case-insensitive directory on Linux/MacOS. It doesn't break it, but it is also not required.

If you are assembling a standalone sysroot outside Bazel, generate `vfsoverlay.yaml` with
`write-vfs` after extraction. The included wrapper scripts still expect that overlay file:

```sh
WINSYSROOT=somewere/my-sysroot wrappers/clang-cl-x64 /o examples/helloworld-x64.exe examples/helloworld.cc
```

If your clang-cl is not called `clang-cl`, you can set the `CLANG_CL` environment variable to what
it is in your environment.

## Notes

- arm64ec is VERY new and as of LLVM 15 does not fully work.

## Is this legal?

Probably. I'm not a lawyer and this is not legal advice, but I'm not distributing any
non-redistributable Microsoft content or circumventing any licensing schemes, you're downloading all
content directly from Microsoft's servers (just a lot more efficiently). Note that you cannot
legally distribute any sysroots generated from this tool without Microsoft's permission.
