# Bugcrowd

## Authentication

Bugcrowd supports three authentication modes:

### 1. Full authentication (email + password + OTP)

Performs a full login flow with TOTP two-factor authentication.

```yaml
bugcrowd:
  email: "you@example.com"
  password: "your_password"
  otpsecret: "YOUR_BASE32_TOTP_SECRET"
```

CLI flags:

```bash
bbscope poll bc --email you@example.com --password pass --otp-secret SECRET
```

### 2. Token authentication

Use a session token directly:

```bash
bbscope poll bc --token "your_session_token"
```

### 3. Public-only mode (no auth)

Fetches only publicly visible programs without any credentials:

```bash
bbscope poll bc --public-only
```

Environment variable for the web server:

```
BC_PUBLIC_ONLY=1
```

## Environment variables (web server)

```
BC_EMAIL=you@example.com
BC_PASSWORD=your_password
BC_OTP=YOUR_TOTP_SECRET
```

Or for public-only:

```
BC_PUBLIC_ONLY=1
```

## Notes

- Bugcrowd rate-limits requests to 1 per second.
- The poller includes WAF ban detection — if a ban is detected, it logs a warning.

## Platform name

Used in database records and API responses: **`bc`**
