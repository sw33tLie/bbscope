# bbscope
The ultimate scope gathering tool for HackerOne, Bugcrowd and Intigriti by sw33tLie.

## Install
```
GO111MODULE=on go get github.com/sw33tLie/bbscope
```

## Flags

HackerOne:
```
$ bbscope h1 -h

  -b, --bbpOnly             Only fetch programs offering monetary rewards
  -c, --categories string   Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, other, hardware, code) (default "all")
  -h, --help                help for h1
  -p, --pvtOnly             Only fetch data from private programs
  -t, --token string        HackerOne session token (__Host-session cookie)
  -u, --urlsToo             Also print the program URL (on each line)
```
Bugcrowd:
```
$ bbscope bc -h

  -b, --bbpOnly             Only fetch programs offering monetary rewards
  -c, --categories string   Scope categories, comma separated (Available: all, url, api, mobile, android, apple, other, hardware) (default "all")
      --concurrency int     Concurrency (default 2)
  -h, --help                help for bc
  -l, --list                List programs instead of grabbing their scope
  -p, --pvtOnly             Only fetch data from private programs
  -t, --token string        Bugcrowd session token (_crowdcontrol_session cookie)
  -u, --urlsToo             Also print the program URL (on each line)
```

Intigriti:
```
$ bbscope it -h

  -b, --bbpOnly             Only fetch programs offering monetary rewards
  -c, --categories string   Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, device, other) (default "all")
  -h, --help                help for it
  -l, --list                List programs instead of grabbing their scope
  -p, --pvtOnly             Only fetch data from private programs
  -t, --token string        Intigriti Authentication Bearer Token (From api.intigriti.com)
  -u, --urlsToo             Also print the program URL (on each line)
```
