# splash
_Incredibly_ fast download/update tool.

NOTE: downloading newer (binary) manifests are currently unsupported.

## Usage
0. Build or acquire a [prebuilt binary](https://github.com/polynite/splash/releases) for your system.
1. Get manifests from [polynite/fn-releases](https://github.com/polynite/fn-releases).
2. Run `splash -h` to see all available options.

## Common use-cases
* To download a specific manifest by id, use `-manifest=<manifest id>`.
* To download a specific manifest from file, drag and drop the manifest file on top of the splash binary, or use `-manifest-file=<path to manifest>`.
* To download only specific files, use `-files=<files to download>`.
* To change the download directory, use `-install-dir=<path>`.

For example, to download the latest build to `C:\Games\FN` use `splash -install-dir=C:\Games\FN`.  

If you wanted to only download the main binary and launcher, use `splash -files=FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe,FortniteGame/Binaries/Win64/FortniteLauncher.exe`.

## Building
0. Download and install [Go](https://golang.org/dl/).
1. Clone the repository.
2. `go build .`
