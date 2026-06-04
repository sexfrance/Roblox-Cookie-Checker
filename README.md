<!-- SPONSOR-START -->
---

<div align="center">

### 🌐 Need Proxies? Check out my services

<a href="https://vaultproxies.com" target="_blank" rel="noopener noreferrer">
  <img src="https://i.imgur.com/TF165pP.gif" alt="VaultProxies">
</a>
<p></p>

<table>
  <tr>
    <th>Service</th>
    <th>Pricing</th>
    <th>Features</th>
  </tr>
  <tr>
    <td><b><a href="https://vaultproxies.com" target="_blank" rel="noopener noreferrer">🔮 VaultProxies</a></b></td>
    <td><code>$1.00/GB</code> residential</td>
    <td>Residential · IPv6 · Residential Unlimited · Datacenter</td>
  </tr>
  <tr>
    <td><b><a href="https://nullproxies.com" target="_blank" rel="noopener noreferrer">🌑 NullProxies</a></b></td>
    <td><code>$0.75/GB</code> residential</td>
    <td>Residential · Residential Unlimited · DC Unlimited · Mobile Proxies</td>
  </tr>
  <tr>
    <td><b><a href="https://strikeproxy.net" target="_blank" rel="noopener noreferrer">⚡ StrikeProxy</a></b></td>
    <td><code>$0.75/GB</code> residential</td>
    <td>Residential · Residential Unlimited · DC Unlimited · Mobile Proxies</td>
  </tr>
</table>
</div>

<!-- SPONSOR-END -->

<div align="center">
  <h2 align="center">Roblox Cookie Checker</h2>
  <p align="center">
    A high-performance Go-based bulk checker for Roblox <code>.ROBLOSECURITY</code> cookies. Validates cookies against the Roblox economy API, outputs valid/invalid accounts, supports rotating residential proxies, and automatically removes checked cookies from the input file.
    <br />
    <br />
    <a href="https://discord.cyberious.xyz">💬 Discord</a>
    ·
    <a href="#-changelog">📜 ChangeLog</a>
    ·
    <a href="https://github.com/sexfrance/Roblox-Cookie-Checker/issues">⚠️ Report Bug</a>
    ·
    <a href="https://github.com/sexfrance/Roblox-Cookie-Checker/issues">💡 Request Feature</a>
  </p>
</div>

---

### ⚙️ Installation

#### Option A — Pre-built binary (Windows, no setup)

- Download `cookie-checker.exe` from the [releases](https://github.com/sexfrance/Roblox-Cookie-Checker/releases)
- Place your cookies in `input/cookies.txt`
- Place your proxies in `input/proxies.txt`
- Edit `input/config.toml` to tune threads / proxy settings
- Run `cookie-checker.exe`

> No Python, no dependencies — single self-contained binary.

#### Option B — Build from source (Go)

**Requirements:** [Go 1.21+](https://go.dev/dl/)

```bash
# 1. Clone the repo
git clone https://github.com/sexfrance/Roblox-Cookie-Checker
cd Roblox-Cookie-Checker

# 2. Download dependencies
go mod download

# 3. Build
go build -o cookie-checker.exe .

# 4. Run
./cookie-checker.exe
```

> Cross-compile for Linux/macOS: `GOOS=linux go build -o cookie-checker .`

---

### 🔥 Features

- Single HTTP request per cookie (`economy.roblox.com/v1/user/currency`) — validates and retrieves Robux in one shot
- Concurrent goroutines — handles 200+ threads with minimal memory overhead
- Rotating proxy support — `rotate = true` opens a fresh TCP connection per request so every request gets a new exit IP
- Resume support — already-checked cookies are skipped on restart
- Auto input cleanup — valid/invalid cookies are removed from `input/cookies.txt` every 500 results (and on exit), errored cookies stay for retry
- Retry on transport errors — connection failures and 429s retry up to 3× with proxy rotation
- Accepts all common input formats (raw cookie, `user:cookie`, `user:pass:cookie`, creator accounts.txt, creator cookies.txt)
- Output: `user:pass:cookie` (or `user:cookie` for raw inputs)

---

### 📝 Usage

1. **Add cookies** — put one cookie per line in `input/cookies.txt`:

   ```
   # raw .ROBLOSECURITY value
   _|WARNING:-DO-NOT-SHARE-THIS...|_CAEa...

   # labeled
   username:_|WARNING:...|_CAEa...

   # creator accounts.txt format
   username:password:_|WARNING:...|_CAEa...

   # creator cookies.txt (full cookie jar — .ROBLOSECURITY extracted automatically)
   rbx-ip2=1; ...; .ROBLOSECURITY=_|WARNING:...|_CAEa...
   ```

2. **Add proxies** — put one proxy per line in `input/proxies.txt`:

   ```
   host:port
   host:port:user:pass
   user:pass@host:port
   http://user:pass@host:port
   socks5://user:pass@host:port
   ```

3. **Run**:

   ```bash
   cookie-checker.exe

   # override threads
   cookie-checker.exe --threads 500

   # check only first 1000 cookies
   cookie-checker.exe -n 1000

   # use a single fixed proxy
   cookie-checker.exe --proxy-url http://user:pass@host:port

   # disable proxies
   cookie-checker.exe --no-proxy
   ```

4. **CLI flags**:

   | Flag | Default | Description |
   | --- | --- | --- |
   | `--config` | `input/config.toml` | Config file path |
   | `--threads` | from config | Concurrent goroutines |
   | `-n` | `0` (all) | Check only first N cookies |
   | `--cookies` | from config | Input cookies file |
   | `--proxy-file` | from config | Proxy list file |
   | `--proxy-url` | — | Single fixed proxy URL |
   | `--no-proxy` | off | Disable all proxies |
   | `--output-dir` | from config | Output directory |
   | `--timeout` | from config | Per-request timeout (seconds) |
   | `--debug` | off | Show full error messages |

---

### 🗂️ File structure

```
Roblox-Cookie-Checker/
  cookie-checker.exe   ← run this
  main.go
  go.mod
  input/
    config.toml        ← threads, proxy settings, paths
    cookies.txt        ← one cookie per line (any format)
    proxies.txt        ← one proxy per line
  output/
    valid.txt          ← user:pass:cookie  (or user:cookie)
    invalid.txt        ← dead cookies
```

---

### ⚙️ Config (`input/config.toml`)

```toml
[run]
threads   = 200     # concurrent goroutines
count     = 0       # 0 = check all
timeout_s = 10.0    # per-request timeout
debug     = false

[proxy]
mode   = "pool"     # none | fixed | pool
url    = ""         # used when mode = "fixed"
file   = "input/proxies.txt"
# rotate = true  → new TCP connection per request (best for rotating residential)
# rotate = false → reuse connections (faster, needs large static pool)
rotate = true

[input]
cookies_file = "input/cookies.txt"

[output]
dir          = "output"
valid_file   = "valid.txt"
invalid_file = "invalid.txt"
```

---

### ❗ Disclaimers

- This project is for educational and research purposes only
- The author is not responsible for any misuse of this tool
- Only check cookies/accounts you own or have explicit permission to test

---

### 📜 ChangeLog

```diff
v0.0.1 ⋮ 06/04/2026
! Initial release.
```

<p align="center">
  <img src="https://img.shields.io/github/license/sexfrance/Roblox-Cookie-Checker.svg?style=for-the-badge&labelColor=black&color=f429ff&logo=IOTA"/>
  <img src="https://img.shields.io/github/stars/sexfrance/Roblox-Cookie-Checker.svg?style=for-the-badge&labelColor=black&color=f429ff&logo=IOTA"/>
  <img src="https://img.shields.io/github/languages/top/sexfrance/Roblox-Cookie-Checker.svg?style=for-the-badge&labelColor=black&color=f429ff&logo=go"/>
</p>
