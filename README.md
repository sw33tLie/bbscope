# bbscope

<p align="center">
  <img src="logo.svg" alt="bbscope-logo-white" width="500">
</p>

**bbscope** is a powerful scope aggregation tool for all major bug bounty platforms:
- [HackerOne](https://hackerone.com/)
- [Bugcrowd](https://bugcrowd.com/)
- [Intigriti](https://intigriti.com/)
- [Immunefi](https://immunefi.com/)
- [YesWeHack](https://yeswehack.com/)

Developed by [sw33tLie](https://x.com/sw33tLie), bbscope helps you efficiently collect and manage program scopes from the platforms where you're active. Whether you're hunting for domains, Android APKs, or binaries to reverse engineer, **bbscope** makes the process quick and simple.

Visit [bbscope.com](https://bbscope.com/) to explore a daily-updated list of public scopes from all supported platforms, stats, and more!

---

## 📦 Installation

Ensure you have a recent version of Go installed, then run:

```bash
go install github.com/sw33tLie/bbscope@latest
```

---

## 🔐 Authentication

Each supported platform requires specific authentication:

- **HackerOne:** Use your API token, available from [H1 API Token Settings](https://hackerone.com/settings/api_token/edit).  
  **Note:** The `-u <username>` flag is mandatory.
- **Bugcrowd:** You have two options:
  - **Option 1:** Supply your email, password, and OTP generation command. This allows bbscope to log in programmatically and obtain a valid token.
  - **Option 2:** Manually log in through your browser and then provide the `_bugcrowd_session` cookie value via the `-t <YOUR_TOKEN>` flag.
  *(Both methods require 2FA; see below for additional details.)*
- **Intigriti:** Generate a personal access token from [Intigriti Personal Access Tokens](https://app.intigriti.com/researcher/personal-access-tokens).
- **YesWeHack:** Use a bearer token collected from API requests. *(Requires 2FA, see below)*
- **Immunefi:** No token is required.

### Two-Factor Authentication (2FA) for Bugcrowd & YesWeHack

Bugcrowd and YesWeHack require two-factor authentication to access authenticated endpoints. We recommend installing the following [2FA CLI tool](https://github.com/rsc/2fa):

```bash
go install rsc.io/2fa@latest
```

Once installed, configure it for Bugcrowd (adjust similarly for YesWeHack):

```bash
2fa -add bugcrowd
2fa key for bugcrowd: your_2fa_key_here
```

Then, supply the OTP automatically using the `--otpcommand` flag in your **bbscope** command:

```bash
--otpcommand "2fa bugcrowd"
```

Replace `"2fa bugcrowd"` with `"2fa yeswehack"` as needed, or whatever name you gave to the 2FA code.

Please note that the `--otpcommand` flag simply runs a shell command to fetch the OTP, and it expects the OTP to be printed to stdout. You can use any other way to fetch the OTP, as long as it prints the OTP to stdout.

---

## 🛠️ Usage

Invoke **bbscope** with the appropriate subcommand and flags:

```bash
bbscope (h1|bc|it|ywh|immunefi) -t <YOUR_TOKEN> [options]
```

For a complete list of options, run:

```bash
bbscope --help
```

Note that subcommands have different options, so be sure to check the help for each subcommand for more information.

---

## 📖 Examples

### HackerOne

Get in-scope targets from bounty-based HackerOne programs:

```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -b -o t
```

List Android APKs from your HackerOne programs:

```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o t -c android
```

Include descriptions and program URLs with your targets:

```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o tdu -d ", "
```

Retrieve URLs from private HackerOne programs:

```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o u -p | sort -u
```

### Bugcrowd

List targets from private Bugcrowd programs that offer rewards, with automatic login:

```bash
bbscope bc -E <YOUR_EMAIL> -P "<YOUR_PASSWORD>" -b -p -o t --otpcommand "2fa bugcrowd"
```

Similarly, you can use the `-t <YOUR_TOKEN>` flag to manually log in and supply the `_bugcrowd_session` cookie value:

```bash
bbscope bc -t <YOUR_TOKEN> -b -p -o t
```

Note that the cookie value will expire after some minutes, so the first method is recommended.

### Intigriti

Get targets and program URLs from all Intigriti programs, including out-of-scope elements:

```bash
bbscope it -t <YOUR_TOKEN> -o tu --oos
```

### Immunefi

Retrieve all available scope data from Immunefi:

```bash
bbscope immunefi
```

---

## ⚠️ Scope Parsing Considerations

Bug bounty programs may not consistently categorize assets. When hunting for URLs with the `-c url` flag, consider also using `-c all` to ensure no relevant targets are missed.

---

## 🙏 Credits

Thanks to the following contributors:

- [0xatul](https://github.com/0xatul)
- [JoeMilian](https://github.com/JoeMilian)
- [ByteOven](https://github.com/ByteOven)
- [dee-see](https://gitlab.com/dee-see)
- [jub0bs](https://jub0bs.com)
- [0xbeefed](https://github.com/0xbeefed)
- [bsysop](https://x.com/bsysop)
