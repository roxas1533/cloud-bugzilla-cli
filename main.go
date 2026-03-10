package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	BaseURL   string            `toml:"base_url"`
	BrowseURL string            `toml:"browse_url"`
	DriveMap  map[string]string `toml:"drive_map"`
	Fix       FixTemplate       `toml:"fix_template"`
	Verify    VerifyTemplate    `toml:"verify_template"`
}

type FixTemplate struct {
	FixedFields string `toml:"fixed_fields"`
}

type VerifyTemplate struct {
	Result string `toml:"result"`
}

var cfg Config

func loadConfig() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := filepath.Join(home, ".config", "bugzilla-cli", "config.toml")
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
	if cfg.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "config: base_url is required")
		os.Exit(1)
	}
}

func convertDrivePath(path string) string {
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		letter := strings.ToUpper(string(path[0]))
		if unc, ok := cfg.DriveMap[letter]; ok {
			return unc + `\` + path[3:]
		}
	}
	return path
}

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
)

type BugResponse struct {
	Bugs []Bug `json:"bugs"`
}

type Bug struct {
	ID         int    `json:"id"`
	Summary    string `json:"summary"`
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
	Product    string `json:"product"`
	Component  string `json:"component"`
}

type CommentResponse struct {
	Bugs map[string]struct {
		Comments []Comment `json:"comments"`
	} `json:"bugs"`
}

type Comment struct {
	Count   int    `json:"count"`
	Creator string `json:"creator"`
	Time    string `json:"creation_time"`
	Text    string `json:"text"`
}

func apiRequest(method, path string, jsonBody []byte) []byte {
	apiKey := os.Getenv("BUGZILLA_API_KEY")

	url := cfg.BaseURL + path
	if apiKey != "" && jsonBody == nil {
		// GETリクエスト: クエリパラメータでapi_keyを送信
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		url += sep + "api_key=" + apiKey
	}

	var bodyReader io.Reader
	if jsonBody != nil && apiKey != "" {
		// PUT/POSTリクエスト: JSONボディにapi_keyを埋め込む
		var m map[string]interface{}
		_ = json.Unmarshal(jsonBody, &m)
		m["api_key"] = apiKey
		jsonBody, _ = json.Marshal(m)
	}
	if jsonBody != nil {
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if jsonBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "API error %d: %s\n", resp.StatusCode, body)
		os.Exit(1)
	}
	return body
}

func apiGet(path string) []byte {
	return apiRequest("GET", path, nil)
}

var scanner = bufio.NewScanner(os.Stdin)

func prompt(label string, required bool) string {
	suffix := ""
	if !required {
		suffix = " (空Enterでスキップ)"
	}
	fmt.Fprintf(os.Stderr, "%s%s%s:%s ", colorBold, label, suffix, colorReset)
	scanner.Scan()
	val := strings.TrimSpace(scanner.Text())
	if required && val == "" {
		fmt.Fprintf(os.Stderr, "%sは必須です\n", label)
		os.Exit(1)
	}
	return val
}

func fetchBug(bugID string) (Bug, []Comment) {
	var bugResp BugResponse
	if err := json.Unmarshal(apiGet("/bug/"+bugID), &bugResp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(bugResp.Bugs) == 0 {
		fmt.Fprintf(os.Stderr, "Bug #%s not found\n", bugID)
		os.Exit(1)
	}

	var comments []Comment
	var commentResp CommentResponse
	if err := json.Unmarshal(apiGet("/bug/"+bugID+"/comment"), &commentResp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if bc, ok := commentResp.Bugs[bugID]; ok {
		comments = bc.Comments
	}

	return bugResp.Bugs[0], comments
}

func cmdShow(bugID string, markdown bool) {
	bug, comments := fetchBug(bugID)

	status := bug.Status
	if bug.Resolution != "" {
		status += " / " + bug.Resolution
	}

	if markdown {
		printShowMarkdown(bug, comments, status)
	} else {
		printShowTerminal(bug, comments, status)
	}
}

func printShowTerminal(bug Bug, comments []Comment, status string) {
	fmt.Printf("%s%sBug #%d: %s%s\n", colorBold, colorCyan, bug.ID, bug.Summary, colorReset)
	fmt.Printf("%sStatus:%s  %s\n", colorBold, colorReset, status)
	fmt.Printf("%sProduct:%s %s\n", colorBold, colorReset, bug.Product)
	fmt.Printf("%sComponent:%s %s\n", colorBold, colorReset, bug.Component)

	fmt.Printf("%sURL:%s      %s\n", colorBold, colorReset, fmt.Sprintf(cfg.BrowseURL, bug.ID))

	if len(comments) == 0 {
		return
	}

	fmt.Printf("\n%s--- Comments (%d) ---%s\n", colorDim, len(comments), colorReset)
	for _, c := range comments {
		date := c.Time
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Printf("\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorDim, colorReset)
		fmt.Printf("%s%s#%d%s %s[%s]%s %s%s%s\n", colorBold, colorYellow, c.Count, colorReset, colorDim, date, colorReset, colorGreen, c.Creator, colorReset)
		fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorDim, colorReset)
		fmt.Printf("%s\n", strings.TrimRight(c.Text, "\n"))
	}
}

func printShowMarkdown(bug Bug, comments []Comment, status string) {
	fmt.Printf("# Bug #%d: %s\n\n", bug.ID, bug.Summary)
	fmt.Printf("| Field | Value |\n|---|---|\n")
	fmt.Printf("| Status | %s |\n", status)
	fmt.Printf("| Product | %s |\n", bug.Product)
	fmt.Printf("| Component | %s |\n", bug.Component)
	fmt.Printf("| URL | %s |\n", fmt.Sprintf(cfg.BrowseURL, bug.ID))

	if len(comments) == 0 {
		return
	}

	fmt.Printf("\n## Comments (%d)\n", len(comments))
	for _, c := range comments {
		date := c.Time
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Printf("\n### #%d %s (%s)\n\n", c.Count, c.Creator, date)
		fmt.Printf("%s\n", strings.TrimRight(c.Text, "\n"))
	}
}

func buildFixComment(version, cause, summary, impact, testfile, spread string) string {
	var b strings.Builder
	b.WriteString("【修正バージョン】\n" + version + "\n")
	if cause != "" {
		b.WriteString("\n【原因】\n" + cause + "\n")
	}
	b.WriteString("\n【修正概要】\n" + summary + "\n")
	b.WriteString("\n【影響する機能（修正者による確認内容）】\n" + impact + "\n")
	b.WriteString("\n【検証項目表名】\n" + testfile + "\n")
	if spread == "" {
		spread = "なし"
	}
	b.WriteString("\n【水平展開先】\n" + spread + "\n")
	b.WriteString("\n" + cfg.Fix.FixedFields)
	return b.String()
}

func cmdFix(bugID string, flags map[string]string) {
	// 必須フラグがすべて揃っていればフラグモード（対話なし）
	_, hasVersion := flags["version"]
	_, hasSummary := flags["summary"]
	_, hasImpact := flags["impact"]
	_, hasTestfile := flags["testfile"]
	flagMode := hasVersion && hasSummary && hasImpact && hasTestfile

	get := func(key, label string, required bool) string {
		if v, ok := flags[key]; ok {
			return v
		}
		if flagMode {
			return ""
		}
		return prompt(label, required)
	}

	version := get("version", "修正バージョン", true)
	cause := get("cause", "原因", false)
	summary := get("summary", "修正概要", true)
	impact := get("impact", "影響する機能（修正者による確認内容）", true)
	testfile := convertDrivePath(get("testfile", "検証項目表名", true))
	spread := get("spread", "水平展開先", false)

	comment := buildFixComment(version, cause, summary, impact, testfile, spread)

	// ステータス変更 + Assignee変更 + コメント追加を一括送信
	putData := map[string]interface{}{
		"status":     "RESOLVED",
		"resolution": "FIXED",
		"comment": map[string]string{
			"body": comment,
		},
	}
	if user := os.Getenv("BUGZILLA_USER"); user != "" {
		putData["assigned_to"] = user
	}
	body, _ := json.Marshal(putData)
	apiRequest("PUT", "/bug/"+bugID, body)

	fmt.Fprintf(os.Stderr, "%s%sBug #%s → RESOLVED/FIXED%s\n", colorBold, colorGreen, bugID, colorReset)
}

func cmdVerify(bugID string, flags map[string]string) {
	_, hasEnv := flags["env"]
	_, hasTestfile := flags["testfile"]
	flagMode := hasEnv && hasTestfile

	get := func(key, label string, required bool) string {
		if v, ok := flags[key]; ok {
			return v
		}
		if flagMode {
			return ""
		}
		return prompt(label, required)
	}

	env := get("env", "確認環境", true)
	testfile := convertDrivePath(get("testfile", "確認内容（検証項目のパス・ファイル名）", true))

	comment := fmt.Sprintf("【確認結果】\n%s\n\n【確認環境】\n%s\n\n【確認内容（検証項目のパス・ファイル名を記載）】\n%s\n", cfg.Verify.Result, env, testfile)

	putData := map[string]interface{}{
		"status": "VERIFIED",
		"comment": map[string]string{
			"body": comment,
		},
	}
	body, _ := json.Marshal(putData)
	apiRequest("PUT", "/bug/"+bugID, body)

	fmt.Fprintf(os.Stderr, "%s%sBug #%s → VERIFIED%s\n", colorBold, colorGreen, bugID, colorReset)
}

// boolFlags: フラグ名のみで値を取らないフラグ（例: "md"）
var boolFlags = map[string]bool{"md": true}

func parseFlags(args []string) (map[string]string, []string) {
	flags := map[string]string{}
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			key := strings.TrimPrefix(args[i], "--")
			if boolFlags[key] {
				flags[key] = ""
			} else if i+1 < len(args) {
				i++
				flags[key] = args[i]
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	return flags, positional
}

func main() {
	loadConfig()

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: bugzilla <show|fix> [options] <bug_id>")
		os.Exit(1)
	}

	cmd := os.Args[1]
	flags, positional := parseFlags(os.Args[2:])
	_, markdown := flags["md"]
	if markdown {
		delete(flags, "md")
	}

	switch cmd {
	case "show":
		if len(positional) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: bugzilla show [--md] <bug_id>")
			os.Exit(1)
		}
		cmdShow(positional[0], markdown)
	case "fix":
		if len(positional) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: bugzilla fix <bug_id> [--version V --summary S --impact I --testfile T] [--cause C] [--spread S]")
			os.Exit(1)
		}
		cmdFix(positional[0], flags)
	case "verify":
		if len(positional) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: bugzilla verify <bug_id> [--env E --testfile T]")
			os.Exit(1)
		}
		cmdVerify(positional[0], flags)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}
