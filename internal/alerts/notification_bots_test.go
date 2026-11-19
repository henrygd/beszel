package alerts

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNormalizeBotWebhook(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "dingtalk console url",
			raw:  "https://oapi.dingtalk.com/robot/send?access_token=abc&secret=sec&keyword=告警",
			want: "dingtalk://oapi.dingtalk.com/abc?keyword=告警&secret=sec",
		},
		{
			name: "dingtalk generic prefix",
			raw:  "generic+https://oapi.dingtalk.com/robot/send?access_token=abc",
			want: "dingtalk://oapi.dingtalk.com/abc",
		},
		{
			name: "wecom console url",
			raw:  "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc-DEF_1&mentioned_list=@all",
			want: "wecom://abc-DEF_1?mentioned_list=%40all",
		},
		{
			name: "wecom generic scheme",
			raw:  "generic://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc",
			want: "wecom://abc",
		},
		{
			name: "feishu console url",
			raw:  "https://open.feishu.cn/open-apis/bot/v2/hook/tok?secret=s%2B1",
			want: "lark://open.feishu.cn/tok?secret=s%2B1",
		},
		{
			name: "lark console url",
			raw:  "https://open.larksuite.com/open-apis/bot/v2/hook/tok/",
			want: "lark://open.larksuite.com/tok",
		},
		{
			name: "feishu scheme",
			raw:  "feishu://tok?secret=s",
			want: "lark://open.feishu.cn/tok?secret=s",
		},
		{
			name: "feishu scheme with host",
			raw:  "feishu://open.larksuite.com/tok?secret=s",
			want: "lark://open.larksuite.com/tok?secret=s",
		},
		{
			name: "existing wecom url",
			raw:  "wecom://abc?mentioned_list=@all",
			want: "wecom://abc?mentioned_list=@all",
		},
		{
			name: "existing lark url",
			raw:  "lark://open.feishu.cn/tok?secret=s",
			want: "lark://open.feishu.cn/tok?secret=s",
		},
		{
			name: "unrelated generic url",
			raw:  "generic+https://example.com/hook?access_token=abc",
			want: "generic+https://example.com/hook?access_token=abc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeBotWebhook(tc.raw)
			if strings.Contains(tc.want, "://") && strings.Contains(got, "://") && got != tc.want {
				assertSameURL(t, got, tc.want)
				return
			}
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDingTalkRequest(t *testing.T) {
	now := time.UnixMilli(1700000000000)
	parsed, err := url.Parse("dingtalk://oapi.dingtalk.com/token?secret=SEC123&keyword=告警")
	if err != nil {
		t.Fatal(err)
	}
	req, host, err := newDingTalkRequest(parsed, "CPU", "usage is high", "https://example.com/system", "View", now)
	if err != nil {
		t.Fatal(err)
	}
	if host != dingtalkHost {
		t.Fatalf("host %s", host)
	}
	if req.URL.Scheme != "https" || req.URL.Host != dingtalkHost || req.URL.Path != "/robot/send" {
		t.Fatalf("endpoint %s", req.URL)
	}
	query := req.URL.Query()
	if query.Get("access_token") != "token" || query.Get("secret") != "" {
		t.Fatalf("query %s", req.URL.RawQuery)
	}
	if query.Get("timestamp") != "1700000000000" || query.Get("sign") != dingTalkSign("SEC123", 1700000000000) {
		t.Fatalf("signature query %s", req.URL.RawQuery)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		MsgType  string `json:"msgtype"`
		Markdown struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"markdown"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MsgType != "markdown" || payload.Markdown.Title != "CPU" {
		t.Fatalf("payload %+v", payload)
	}
	if !strings.Contains(payload.Markdown.Text, "告警") || !strings.Contains(payload.Markdown.Text, "usage is high") || !strings.Contains(payload.Markdown.Text, "[View](https://example.com/system)") {
		t.Fatalf("text %s", payload.Markdown.Text)
	}
	if strings.Count(payload.Markdown.Text, "告警") != 1 {
		t.Fatalf("keyword duplicated: %s", payload.Markdown.Text)
	}
	_, text := dingTalkMarkdown("CPU", "告警 usage", "", "", "告警")
	if strings.HasPrefix(text, "告警") {
		t.Fatalf("keyword prepended twice: %s", text)
	}
}

func TestFormatBotAlerts(t *testing.T) {
	feishu, err := url.Parse(normalizeBotWebhook("feishu://tok?secret=s"))
	if err != nil {
		t.Fatal(err)
	}
	gotURL, gotMessage := formatShoutrrrAlert(feishu, "CPU", "usage is high", "https://example.com", "View")
	parsed, err := url.Parse(gotURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "lark" || parsed.Host != feishuHost || parsed.Query().Get("title") != "CPU" || parsed.Query().Get("link") != "https://example.com" || parsed.Query().Get("secret") != "s" {
		t.Fatalf("feishu url %s", gotURL)
	}
	if gotMessage != "usage is high" {
		t.Fatalf("feishu message %s", gotMessage)
	}

	wecom, err := url.Parse(normalizeBotWebhook("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc&mentioned_list=@all"))
	if err != nil {
		t.Fatal(err)
	}
	gotURL, gotMessage = formatShoutrrrAlert(wecom, "CPU", "usage is high", "https://example.com", "View")
	parsed, err = url.Parse(gotURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "wecom" || parsed.Host != "abc" || parsed.Query().Get("mentioned_list") != "@all" {
		t.Fatalf("wecom url %s", gotURL)
	}
	if gotMessage != "CPU\n\nusage is high\n\nhttps://example.com" {
		t.Fatalf("wecom message %q", gotMessage)
	}
}

func TestDingTalkRequestShortURL(t *testing.T) {
	parsed, err := url.Parse("dingtalk://token")
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := newDingTalkRequest(parsed, "", "hello", "", "", time.UnixMilli(1))
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Host != dingtalkHost || req.URL.Query().Get("access_token") != "token" || req.URL.Query().Has("sign") {
		t.Fatalf("endpoint %s", req.URL)
	}
	body, _ := io.ReadAll(req.Body)
	if !strings.Contains(string(body), `"title":"Beszel"`) {
		t.Fatalf("body %s", body)
	}
}

func TestDingTalkRequestRequiresToken(t *testing.T) {
	parsed, err := url.Parse("dingtalk://oapi.dingtalk.com")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = newDingTalkRequest(parsed, "t", "m", "", "", time.Now())
	if err == nil || !strings.Contains(err.Error(), "access token") {
		t.Fatalf("got %v", err)
	}
}

func TestSendDingTalkBlocksInternalHost(t *testing.T) {
	parsed, err := url.Parse("dingtalk://127.0.0.1/token")
	if err != nil {
		t.Fatal(err)
	}
	err = sendDingTalk(parsed, "t", "m", "", "", true)
	if !errors.Is(err, errInternalDestination) {
		t.Fatalf("got %v", err)
	}
}

func TestSendDingTalkReadsAPIResult(t *testing.T) {
	previous := adminNotificationClient.Transport
	t.Cleanup(func() { adminNotificationClient.Transport = previous })

	parsed, err := url.Parse("dingtalk://token?keyword=告警")
	if err != nil {
		t.Fatal(err)
	}
	adminNotificationClient.Transport = notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != dingtalkHost || r.URL.Query().Get("access_token") != "token" {
			t.Errorf("request URL %s", r.URL)
		}
		return &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errcode":310000,"errmsg":"keywords not in content"}`)),
			Request:    r,
		}, nil
	})
	err = sendDingTalk(parsed, "CPU", "usage is high", "", "", false)
	if err == nil || !strings.Contains(err.Error(), "310000") {
		t.Fatalf("got %v", err)
	}

	adminNotificationClient.Transport = notificationRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Request:    r,
		}, nil
	})
	if err := sendDingTalk(parsed, "CPU", "usage is high", "", "", false); err != nil {
		t.Fatal(err)
	}
}

func assertSameURL(t *testing.T, got, want string) {
	t.Helper()
	left, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	right, err := url.Parse(want)
	if err != nil {
		t.Fatal(err)
	}
	if left.Scheme != right.Scheme || left.Host != right.Host || left.Path != right.Path || left.Query().Encode() != right.Query().Encode() {
		t.Fatalf("got %s, want %s", got, want)
	}
}
