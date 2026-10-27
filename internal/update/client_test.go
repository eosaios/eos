package update

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNewHTTPClientEmptyReturnsNil(t *testing.T) {
	client, err := NewHTTPClient("")
	if err != nil {
		t.Fatalf("NewHTTPClient(\"\"): %v", err)
	}
	if client != nil {
		t.Errorf("空代理地址应返回 nil 客户端（沿用默认），got %+v", client)
	}
	client, err = NewHTTPClient("  ")
	if err != nil || client != nil {
		t.Errorf("空白代理地址应返回 nil, nil，got %+v / %v", client, err)
	}
}

func TestNewHTTPClientRejectsInvalidURL(t *testing.T) {
	cases := []string{
		"ftp://127.0.0.1:8080", // scheme 非法
		"http://",              // 缺 host
		"http:///path",         // 缺 host
	}
	for _, raw := range cases {
		if _, err := NewHTTPClient(raw); err == nil {
			t.Errorf("NewHTTPClient(%q) 应报错", raw)
		}
	}
}

func TestNewHTTPClientWiresProxy(t *testing.T) {
	client, err := NewHTTPClient("http://127.0.0.1:7897")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	if client == nil {
		t.Fatal("NewHTTPClient 返回 nil")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型 %T，应为 *http.Transport", client.Transport)
	}
	req, err := http.NewRequest(http.MethodGet, "https://github.com/eosaios/eos/releases/download/v1.0.0/eos-cli.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy(req): %v", err)
	}
	want, _ := url.Parse("http://127.0.0.1:7897")
	if proxy == nil || proxy.String() != want.String() {
		t.Errorf("请求应经代理 %s，got %v", want, proxy)
	}
}

func TestNewHTTPClientProxyHostKeptForHttpsTarget(t *testing.T) {
	client, err := NewHTTPClient("http://proxy.example.com:8080")
	if err != nil {
		t.Fatalf("NewHTTPClient: %v", err)
	}
	transport := client.Transport.(*http.Transport)
	req, _ := http.NewRequest(http.MethodGet, "https://objects.githubusercontent.com/x.zip", nil)
	proxy, err := transport.Proxy(req)
	if err != nil || proxy == nil {
		t.Fatalf("https 目标也应走代理: %v / %v", proxy, err)
	}
}
