package alerts

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	dingtalkHost = "oapi.dingtalk.com"
	wecomHost    = "qyapi.weixin.qq.com"
	feishuHost   = "open.feishu.cn"
	larkHost     = "open.larksuite.com"
)

var adminNotificationClient = &http.Client{Timeout: 15 * time.Second}

// normalizeBotWebhook turns DingTalk, WeCom, and Feishu bot webhook URLs into
// the service URLs the sender understands. Other URLs are returned unchanged.
// feishu:// is an alias of Shoutrrr's lark:// scheme.
func normalizeBotWebhook(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if rest, ok := strings.CutPrefix(raw, "generic+"); ok {
		if rewritten, ok := rewriteOfficialWebhook(rest); ok {
			return rewritten
		}
		return raw
	}
	if strings.HasPrefix(strings.ToLower(raw), "feishu://") {
		if rewritten, ok := rewriteFeishuScheme(raw); ok {
			return rewritten
		}
	}
	if rewritten, ok := rewriteOfficialWebhook(raw); ok {
		return rewritten
	}
	return raw
}

func rewriteOfficialWebhook(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "generic":
	default:
		return "", false
	}
	query := parsed.Query()
	switch strings.ToLower(parsed.Hostname()) {
	case dingtalkHost:
		token := query.Get("access_token")
		if token == "" || !strings.Contains(parsed.Path, "/robot/send") {
			return "", false
		}
		return dingtalkServiceURL(dingtalkHost, token, query.Get("secret"), query.Get("keyword")), true
	case wecomHost:
		key := query.Get("key")
		if key == "" || !strings.Contains(parsed.Path, "/webhook/send") {
			return "", false
		}
		values := url.Values{}
		if mentioned := query.Get("mentioned_list"); mentioned != "" {
			values.Set("mentioned_list", mentioned)
		}
		if mobiles := query.Get("mentioned_mobile_list"); mobiles != "" {
			values.Set("mentioned_mobile_list", mobiles)
		}
		return serviceURL("wecom", key, "", values), true
	case feishuHost, larkHost:
		if !strings.Contains(parsed.Path, "/hook/") {
			return "", false
		}
		token := lastPathSegment(parsed.Path)
		if token == "" || token == "hook" {
			return "", false
		}
		values := url.Values{}
		if secret := query.Get("secret"); secret != "" {
			values.Set("secret", secret)
		}
		return serviceURL("lark", strings.ToLower(parsed.Hostname()), "/"+token, values), true
	default:
		return "", false
	}
}

func rewriteFeishuScheme(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	token := lastPathSegment(parsed.Path)
	host := parsed.Hostname()
	if token == "" {
		if host == "" || strings.Contains(host, ".") {
			return "", false
		}
		token = host
		host = feishuHost
	}
	if host == "" {
		host = feishuHost
	}
	if !strings.EqualFold(host, feishuHost) && !strings.EqualFold(host, larkHost) {
		return "", false
	}
	values := url.Values{}
	if secret := parsed.Query().Get("secret"); secret != "" {
		values.Set("secret", secret)
	}
	return serviceURL("lark", strings.ToLower(host), "/"+token, values), true
}

func dingtalkServiceURL(apiHost, token, secret, keyword string) string {
	values := url.Values{}
	if secret != "" {
		values.Set("secret", secret)
	}
	if keyword != "" {
		values.Set("keyword", keyword)
	}
	return serviceURL("dingtalk", apiHost, "/"+token, values)
}

func serviceURL(scheme, host, path string, query url.Values) string {
	parsed := &url.URL{Scheme: scheme, Host: host, Path: path}
	if len(query) > 0 {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func lastPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func sendDingTalk(parsed *url.URL, title, message, link, linkText string, restricted bool) error {
	req, apiHost, err := newDingTalkRequest(parsed, title, message, link, linkText, time.Now())
	if err != nil {
		return err
	}
	client := adminNotificationClient
	if restricted {
		client = publicNotificationClient
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, errInternalDestination) {
			return errInternalDestination
		}
		return fmt.Errorf("dingtalk notification to %s failed", apiHost)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("dingtalk notification to %s failed", apiHost)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dingtalk notification to %s failed: %s", apiHost, resp.Status)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("dingtalk notification to %s failed", apiHost)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("dingtalk notification failed: %d %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

func newDingTalkRequest(parsed *url.URL, title, message, link, linkText string, now time.Time) (*http.Request, string, error) {
	apiHost, token, secret, keyword, err := parseDingTalkURL(parsed)
	if err != nil {
		return nil, "", err
	}
	cardTitle, text := dingTalkMarkdown(title, message, link, linkText, keyword)
	payload, err := json.Marshal(dingTalkPayload{
		MsgType:  "markdown",
		Markdown: dingTalkMarkdownBody{Title: cardTitle, Text: text},
	})
	if err != nil {
		return nil, "", err
	}
	endpoint := &url.URL{Scheme: "https", Host: apiHost, Path: "/robot/send"}
	query := url.Values{}
	query.Set("access_token", token)
	if secret != "" {
		timestamp := now.UnixMilli()
		query.Set("timestamp", strconv.FormatInt(timestamp, 10))
		// DingTalk signs timestamp + "\n" + secret with HMAC-SHA256, keyed by the secret.
		query.Set("sign", dingTalkSign(secret, timestamp))
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, apiHost, nil
}

func parseDingTalkURL(parsed *url.URL) (apiHost, token, secret, keyword string, err error) {
	query := parsed.Query()
	secret = query.Get("secret")
	keyword = query.Get("keyword")
	token = lastPathSegment(parsed.Path)
	apiHost = parsed.Host
	if token == "" {
		if apiHost == "" || strings.ContainsAny(apiHost, ".:") {
			return "", "", "", "", errors.New("dingtalk access token is required")
		}
		token = apiHost
		apiHost = dingtalkHost
	}
	if apiHost == "" {
		apiHost = dingtalkHost
	}
	return apiHost, token, secret, keyword, nil
}

func dingTalkMarkdown(title, message, link, linkText, keyword string) (string, string) {
	cardTitle := title
	if cardTitle == "" {
		cardTitle = "Beszel"
	}
	var body strings.Builder
	if keyword != "" && !strings.Contains(title, keyword) && !strings.Contains(message, keyword) {
		body.WriteString(keyword)
		body.WriteString("\n\n")
	}
	if title != "" {
		body.WriteString("#### ")
		body.WriteString(title)
		body.WriteString("\n\n")
	}
	body.WriteString(message)
	if link != "" {
		label := linkText
		if label == "" {
			label = link
		}
		body.WriteString("\n\n[")
		body.WriteString(label)
		body.WriteString("](")
		body.WriteString(link)
		body.WriteString(")")
	}
	return cardTitle, body.String()
}

type dingTalkPayload struct {
	MsgType  string               `json:"msgtype"`
	Markdown dingTalkMarkdownBody `json:"markdown"`
}

type dingTalkMarkdownBody struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

func dingTalkSign(secret string, timestamp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "\n" + secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
