# splash
_Incredibly_ fast download/update tool.

## Usage
0. Build or acquire a [prebuilt binary](https://github.com/polynite/splash/releases) for your system.
1. Run `splash -h` to see all available options.

## Common use-cases
* To download a specific manifest from file, use `-manifest-file=<path to manifest>`.
* To download only specific files, use `-files=<files to download>`.
* To change the download directory, use `-install-dir=<path>`.

For example, to download the latest build to `C:\Games\FN` use `splash -install-dir=C:\Games\FN`.  

If you wanted to only download the main binary and launcher use `splash -files=FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe,FortniteGame/Binaries/Win64/FortniteLauncher.exe`.  

**NOTE:** Old manifests have been removed from the various CDNs but chunks still seem to be available. This means the only way to download older builds is by using a complete manifest.

## Building
0. Download and install [Go](https://golang.org/dl/).
1. Clone the repository.
2. `go build .`
