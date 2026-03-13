# Output Formatting

## Output flags (`-o`)

The `-o` flag controls which fields are included in the output. Flags can be combined:

| Flag | Field |
|------|-------|
| `t` | Target (hostname, URL, IP, etc.) |
| `d` | Target description |
| `c` | Category (wildcard, url, cidr, etc.) |
| `u` | Program URL |

### Examples

```bash
# Target only (default for db get)
bbscope poll -o t

# Target + program URL (default for poll)
bbscope poll -o tu

# All fields
bbscope poll -o tdcu
```

## Delimiter (`-d`)

Set the separator between fields (default: space):

```bash
# Tab-delimited
bbscope poll -o tdu -d $'\t'

# Comma-delimited
bbscope poll -o tdu -d ","

# Pipe-delimited
bbscope poll -o tdu -d "|"
```

## Output formats (db print)

The `db print` command supports structured output formats:

```bash
# Plain text (default)
bbscope db print

# JSON
bbscope db print --format json

# CSV
bbscope db print --format csv
```

## Change output (--db mode)

When polling with `--db`, changes are printed with emoji prefixes:

```
🆕  h1  https://hackerone.com/program  *.new-target.com
❌  bc  https://bugcrowd.com/program  removed-target.com
🔄  it  https://intigriti.com/program  updated-target.com
❌ Program removed: https://hackerone.com/old-program
```

With AI normalization enabled, normalized variants show as:

```
🆕  h1  https://hackerone.com/program  *.example.com (main site) -> *.example.com
```
