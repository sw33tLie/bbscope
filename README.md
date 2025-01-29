# 🌐 **bbscope**  
### The ultimate tool to gather scope details from:  
- [HackerOne](https://hackerone.com/) 🕵️‍♂️  
- [Bugcrowd](https://bugcrowd.com/) 🛡️  
- [Intigriti](https://intigriti.com) 🔍  
- [Immunefi](https://immunefi.com/) 🐛  
- [YesWeHack](https://yeswehack.com/) 💡  

Developed by **sw33tLie** to simplify your bug bounty workflows. 🎯  

---

## 🚀 **Overview**
Are you tired of manually gathering scope information from bug bounty platforms?  
Look no further! **bbscope** is designed to help you:  

- 📜 **List domains** in scope for your programs.  
- 📱 **Find Android APKs** you’re allowed to test.  
- 🛠️ **Grab binaries** for reverse engineering.  

No matter what your focus is, **bbscope** has you covered.  

---

## ⚙️ **Installation**
To get started, ensure you have a recent version of the Go compiler installed.  
Then, run the following command to install **bbscope**:

```bash
GO111MODULE=on go install github.com/sw33tLie/bbscope@latest
```

---

## 🧰 **Usage**
The basic syntax for using **bbscope** is:

```bash
bbscope (h1|bc|it|ywh|immunefi) -t <YOUR_TOKEN> <other-flags>
```

### 🔑 **How to Get Your Session Token**
Here’s how to retrieve your session token for each platform:  
- **HackerOne**:  
  Log in and grab your API token from your [API settings page](https://hackerone.com/settings/api_token/edit).  
  *(Required: `-u` flag for your username)*  

- **Bugcrowd**:  
  Log in and fetch the `_bugcrowd_session` cookie.  
  *(Note: This has replaced `_crowdcontrol_session`.)*  

- **Intigriti**:  
  Get your researcher API token from the [Personal Access Tokens page](https://app.intigriti.com/researcher/personal-access-tokens).  

- **YesWeHack**:  
  Intercept a request to `api.yeswehack.com` and find the `Authorization: Bearer XXX` header. `XXX` is your token.  

- **Immunefi**:  
  No token is required for this platform!  

> 📝 **Tip:** Use the `--help` flag to view all available options and flags.  

---

## 💡 **Examples**

Here are some common use cases for **bbscope**:  

### 1️⃣ **Print all in-scope targets from HackerOne programs offering rewards**  
```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -b -o t
```
Output:
```plaintext
app.example.com
*.user.example.com
*.demo.com
www.something.com
```

### 2️⃣ **List in-scope targets from private Bugcrowd programs with rewards**  
```bash
bbscope bc -t <YOUR_TOKEN> -b -p -o t
```

### 3️⃣ **Show in-scope targets + program URLs from Intigriti**  
```bash
bbscope it -t <YOUR_TOKEN> -o tu --oos
```

### 4️⃣ **Print all Android APKs in scope from HackerOne**  
```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o t -c android
```

### 5️⃣ **Get detailed in-scope targets with descriptions and program URLs (HackerOne)**  
```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o tdu -d ", "
```
Output:
```plaintext
something.com, Something's main website, https://hackerone.com/something
*.demo.com, All assets owned by Demo are in scope, https://hackerone.com/demo
```

### 6️⃣ **Fetch program URLs for private HackerOne programs**  
```bash
bbscope h1 -t <YOUR_TOKEN> -u <YOUR_H1_USERNAME> -o u -p | sort -u
```
Output:
```plaintext
https://hackerone.com/demo
https://hackerone.com/something
```

### 7️⃣ **Get the entire Immunefi scope**  
```bash
bbscope immunefi
```

---

## ⚠️ **Beware of Scope Oddities**
While most programs clearly outline their in-scope elements, some may have inconsistencies:  
- Assets might be categorized incorrectly.  
- For example, if you’re targeting URLs using `-c url`, consider cross-checking with `-c all` to avoid missing anything important.  

---

## 🙏 **Thanks**  
Special thanks to the amazing contributors and supporters:  
- [0xatul](https://github.com/0xatul)  
- [JoeMilian](https://github.com/JoeMilian)  
- [ByteOven](https://github.com/ByteOven)  
- [dee-see](https://gitlab.com/dee-see)  
- [jub0bs](https://jub0bs.com)  
- [0xbeefed](https://github.com/0xbeefed)  



### 🎉 **Enjoy using bbscope!**  
Let **bbscope** simplify your bug bounty research and help you focus on what matters most. Happy hacking! 🐛💻  
