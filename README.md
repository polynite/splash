# splash
_Incredibly_ fast download/update tool.

## Usage
0. Build or acquire a [prebuilt binary](https://github.com/polynite/splash/releases) for your system.
1. Run `splash -h` to see all available options.

## Common use-cases
* To download a [specific manifest](https://github.com/polynite/fn-releases), use `-manifest=<manifest id>`.
* To download only specific files, use `-files=<files to download>`.
* To change the download directory, use `-install-dir=<path>`.

For example, to download build `10.0-CL-7658179` to `C:\Games\FN` use `splash -manifest=wcfjh9c-okLtEOiDMkG8VzIC1p-ENg -install-dir=C:\Games\FN`.  
If you wanted to only download the main binary and launcher for `13.30-CL-13884634` use `splash -manifest=__d-73Y9siJhxSaCRE6egZe3gbpjNw -files=FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe,FortniteGame/Binaries/Win64/FortniteLauncher.exe`.

## Building
0. Download and install [Go](https://golang.org/dl/).
1. Clone the repository.
2. `go build .`
