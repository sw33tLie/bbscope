# bbscope
The ultimate scope gathering tool for HackerOne, Bugcrowd, and Intigriti by sw33tLie.

Need to grep all the large scope domains that you've got on your bug bounty platforms? This is the right tool for the job.  
What about getting a list of android apps that you are allowed to test? We've got you covered as well.

Reverse engineering god? No worries, you can get a list of binaries to analyze too :)

## Installation
Make sure you've a recent version of the Go compiler installed on your system.
Then just run:
```
GO111MODULE=on go get -u github.com/sw33tLie/bbscope
```

## Usage
```
bbscope (h1|bc|it) -t <YOUR_TOKEN> <other-flags>
```
How to get the session token:
- HackerOne: login, then grab your API token [here](https://hackerone.com/settings/api_token/edit)
- Bugcrowd: login, then grab the `_crowdcontrol_session` cookie
- Intigriti: login, then intercept a request to api.intigriti.com and look for the `Authentication: Bearer XXX` header. XXX is your token

Remember that you can use the --help flag to get a description for all flags.

## Examples
Below you'll find some example commands.
Keep in mind that all of them work with Bugcrowd and Intigriti subcommands (`bc` and `it`) as well, not just with `h1`.

### Print all in-scope targets from all your HackerOne programs that offer rewards
```
bbscope h1 -t <YOUR_TOKEN> -b -o t
```
The output will look like this:
```
app.example.com
*.user.example.com
*.demo.com
www.something.com
```

### Print all in-scope targets from all your private HackerOne programs that offer rewards
```
bbscope h1 -t <YOUR_TOKEN> -b -p -o t
```

### Print all in-scope Android APKs from all your HackerOne programs
```
bbscope h1 -t <YOUR_TOKEN> -o t -c android
```

### Print all in-scope targets from all your HackerOne programs with extra data

```
bbscope h1 -t <YOUR_TOKEN> -o tdu -d ", "
```

This will print a list of in-scope targets from all your HackerOne programs (including public ones and VDPs) but, on the same line, it will also print the target description (when available) and the program's URL.
It might look like this:
```
something.com, Something's main website, https://hackerone.com/something
*.demo.com, All assets owned by Demo are in scope, https://hackerone.com/demo
```
### Get program URLs for your HackerOne private programs

```
bbscope h1 -t <YOUR_TOKEN> -o u -p | sort -u
```
You'll get a list like this:
```
https://hackerone.com/demo
https://hackerone.com/something
```

## Beware of scope oddities
In an ideal world, all programs use the in-scope table in the same way to clearly show what's in scope, and make parsing easy.
Unfortunately, that's not always the case.

Sometimes assets are assigned the wrong category.
For example, if you're going after URLs using the `-c url`, double checking using `-c all` is often a good idea.

Other times, on HackerOne, you will find targets written in the scope description, instead of in the scope title.
A few programs that do this are:
- [Verizon Media](https://hackerone.com/verizonmedia/?type=team)
- [Mail.ru](https://hackerone.com/mailru)

If you want to grep those URLs as well, you **MUST** include `d` in the printing options flag (`-o`).

Sometimes it gets even stranger: [Spotify](https://hackerone.com/spotify) uses titles of the in-scope table to list wildcards, but then lists the actually in-scope subdomains in the targets description.

Human minds are weird and this tool does not attempt to parse nonsense, you'll have to do that manually (or bother people that can make this change, maybe?).

## Thanks
- [0xatul](https://github.com/0xatul)
- [JoeMilian](https://github.com/JoeMilian)
- [ByteOven](https://github.com/ByteOven)
- [dee-see](https://gitlab.com/dee-see)
- [jub0bs](https://jub0bs.com)
