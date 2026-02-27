# YesWeHack

## Authentication

YesWeHack uses email + password + TOTP.

### Config file

```yaml
yeswehack:
  email: "you@example.com"
  password: "your_password"
  otpsecret: "YOUR_TOTP_SECRET"
```

### CLI flags

```bash
bbscope poll ywh --email you@example.com --password pass --otp-secret SECRET
```

Or with a bearer token:

```bash
bbscope poll ywh --token "your_bearer_token"
```

### Environment variables (web server)

```
YWH_EMAIL=you@example.com
YWH_PASSWORD=your_password
YWH_OTP=YOUR_TOTP_SECRET
```

## What it fetches

- All accessible programs via the YesWeHack API
- Paginated program listing
- In-scope and out-of-scope targets with categories

## Platform name

Used in database records and API responses: **`ywh`**
