## Overview

CLI tool for automatic file backups. You provide input and output paths. The tool watches the input paths, detects file changes, and copies files to the output paths, numerated.

## Installation

First, install Go: https://golang.org. Then run this:

```sh
go install github.com/mitranim/backup@latest
```

This will compile the executable into `$GOPATH/bin/backup`. Make sure `$GOPATH/bin` is in your `$PATH` so the shell can discover the `backup` command. For example, my `~/.profile` contains this:

```sh
export GOPATH="$HOME/go"
export PATH="$GOPATH/bin:$PATH"
```

Alternatively, you can run the executable using the full path. At the time of writing, `~/go` is the default `$GOPATH` for Go installations. Some systems may have a different one.

```sh
~/go/bin/backup
```

## Usage

Create a configuration file as described below. Run `backup -h` to view help. Run `backup` or `backup -v` to run the tool.

## Configuration

The tool _requires_ a JSON config file where you specify inputs and outputs. By default, it must be called `backup.json` and located in the current directory. You may provide another config path via `-c`.

To see all available settings, read the type `Config` in [backup.go](blob/main/backup.go). Some settings may be provided both at the top level and in individual entries. The entry overrides take priority.

Example config. Note that file paths may be either absolute or relative to the directory whence you run the tool. When writing Windows paths, use double backslashes `\\` as separators.

```json
{
  "limit": 32,
  "entries": [
    {
      "input": "/Users/some_user/Documents/some_file_or_directory",
      "output": "/Users/some_user/Downloads/some_backups"
    },
    {
      "input": "Documents/another_file_or_directory",
      "output": "Downloads/some_backups"
      "limit": 64
    }
  ]
}
```

## License

https://unlicense.org
