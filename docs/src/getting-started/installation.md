# Installation

## Go install (recommended)

Requires Go 1.24+:

```bash
go install github.com/sw33tLie/bbscope/v2@latest
```

## Prebuilt binaries

Download from [GitHub Releases](https://github.com/sw33tLie/bbscope/releases). Binaries are available for Linux, macOS, and Windows (amd64/arm64).

## Docker

```bash
docker pull ghcr.io/sw33tlie/bbscope:latest
docker run --rm ghcr.io/sw33tlie/bbscope:latest poll h1 --user x --token y
```

## Build from source

```bash
git clone https://github.com/sw33tLie/bbscope.git
cd bbscope
go build -o bbscope .
```
