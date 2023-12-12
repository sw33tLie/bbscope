# bbscope
The ultimate scope gathering tool for [HackerOne](https://hackerone.com/), [Bugcrowd](https://bugcrowd.com/), [Intigriti](https://intigriti.com), [Immunefi](https://immunefi.com/) and [YesWeHack](https://yeswehack.com/) by sw33tLie.

Need to grep all the large scope domains that you've got on your bug bounty platforms? This is the right tool for the job.  
What about getting a list of android apps that you are allowed to test? We've got you covered as well.

Reverse engineering god? No worries, you can get a list of binaries to analyze too :)

## Installation
Make sure you've a recent version of the Go compiler installed on your system.
Then just run:
```
GO111MODULE=on go install github.com/sw33tLie/bbscope@latest
```

## Usage
```
bbscope (h1|bc|it|ywh|immunefi) -t <YOUR_TOKEN> <other-flags>
```
How to get the session token:
- HackerOne: login, then grab your API token [here](https://hackerone.com/settings/api_token/edit)
- Bugcrowd: login, then grab the `_bugcrowd_session` cookie. NOTE: This has changed, it's not the `_crowdcontrol_session` cookie anymore.
- Intigriti: Get your researcher API token [here](https://app.intigriti.com/researcher/personal-access-tokens)
- YesWeHack: login, then intercept a request to api.yeswehack.com and look for the `Authorization: Bearer  XXX` header. XXX is your token
- Immunefi: no token required

When using bbscope for HackerOne, the username flag (`-u`) is mandatory.

Remember that you can use the --help flag to get a description for all flags.

## Examples
Below you'll find some example commands.
Keep in mind that all of them work with Bugcrowd, Intigriti and YesWeHack subcommands (`bc`, `it` and `ywh`) as well, not just with `h1`.

### Print all in-scope targets from all your HackerOne programs that offer rewards
```
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -b -o t
```
The output will look like this:
```
app.example.com
*.user.example.com
*.demo.com
www.something.com
```

### Print all in-scope targets from all your private Bugcrowd programs that offer rewards
```
bbscope bc -t <YOUR_TOKEN> -b -p -o t
```

### Print all in-scope targets+program page URL from all Intigriti programs, including OOS elements
```
it -t <YOUR_TOKEN> -o tu --oos
```

### Print all in-scope Android APKs from all your HackerOne programs
```
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o t -c android
```

### Print all in-scope targets from all your HackerOne programs with extra data

```
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o tdu -d ", "
```

This will print a list of in-scope targets from all your HackerOne programs (including public ones and VDPs) but, on the same line, it will also print the target description (when available) and the program's URL.
It might look like this:
```
something.com, Something's main website, https://hackerone.com/something
*.demo.com, All assets owned by Demo are in scope, https://hackerone.com/demo
```
### Get program URLs for your HackerOne private programs

```
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o u -p | sort -u
```
You'll get a list like this:
```
https://hackerone.com/demo
https://hackerone.com/something
```

### Get all immunefi scope

```
bbscope immunefi
```

## Beware of scope oddities
In an ideal world, all programs use the in-scope table in the same way to clearly show what's in scope, and make parsing easy.
Unfortunately, that's not always the case.

Sometimes assets are assigned the wrong category.
For example, if you're going after URLs using the `-c url`, double checking using `-c all` is often a good idea.

## Thanks
- [0xatul](https://github.com/0xatul)
- [JoeMilian](https://github.com/JoeMilian)
- [ByteOven](https://github.com/ByteOven)
- [dee-see](https://gitlab.com/dee-see)
- [jub0bs](https://jub0bs.com)
- [0xbeefed](https://github.com/0xbeefed)
