package client

import (
	"NUMParser/config"
	"context"
	"errors"
	"github.com/parnurzeal/gorequest"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var Err404 = errors.New("404 not found")

const (
	dnsResolverIP        = "151.80.222.79:53"
	dnsResolverProto     = "udp"
	dnsResolverTimeoutMs = 3000
)

var nicHTTPClient = func() *http.Client {
	dialer := &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Duration(dnsResolverTimeoutMs) * time.Millisecond,
				}
				return d.DialContext(ctx, dnsResolverProto, dnsResolverIP)
			},
		},
		Timeout:   120 * time.Second,
		KeepAlive: 120 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{Transport: transport, Timeout: 120 * time.Second}
}()

var (
	transportsMu   sync.RWMutex
	transportsPool = make(map[string]*http.Transport)
)

func getPooledTransport(proxyURL string) *http.Transport {
	transportsMu.RLock()
	tr, ok := transportsPool[proxyURL]
	transportsMu.RUnlock()
	if ok {
		return tr
	}

	transportsMu.Lock()
	defer transportsMu.Unlock()
	if tr, ok = transportsPool[proxyURL]; ok {
		return tr
	}

	tr = &http.Transport{
		DisableKeepAlives:     false,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			tr.Proxy = http.ProxyURL(parsed)
		} else {
			tr.Proxy = http.ProxyFromEnvironment
		}
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}

	transportsPool[proxyURL] = tr
	return tr
}

func GetNic(link, referer, cookie string) (string, error) {
	req, err := http.NewRequest("GET", link, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.100 Safari/537.36")
	if cookie != "" {
		req.Header.Set("cookie", cookie)
	}
	if referer != "" {
		req.Header.Set("referer", referer)
	}

	resp, err := nicHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode == 404 {
		log.Println("Error get link:", link, resp.StatusCode, resp.Status)
		return "", Err404
	} else if resp.StatusCode != 200 {
		log.Println("Error get link:", link, resp.StatusCode, resp.Status)
		return "", errors.New(resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func Get(link string) *gorequest.SuperAgent {
	return GetParam(link, "", "")
}

func GetParam(link, referer, cookie string) *gorequest.SuperAgent {
	agent := gorequest.New()

	if cookie != "" {
		header := http.Header{}
		header.Add("Cookie", cookie)
		request := http.Request{
			Header: header,
		}
		agent.Cookies = request.Cookies()
	}

	if referer != "" {
		agent.AppendHeader("referer", referer)
	}

	var proxyHost string
	if config.UseProxy {
		proxyHost = getProxyFromList()
		if proxyHost == "" {
			proxyHost = config.ProxyHost
		}
	} else if config.ProxyHost != "" {
		proxyHost = config.ProxyHost
	}

	tr := getPooledTransport(proxyHost)
	agent.Transport = tr
	agent.Client.Transport = tr

	agent.Timeout(30 * time.Second)
	return agent.Get(link)
}
