package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var proxyTestVersion = &version{
	name: "foo",
	sequins: &sequins{
		config: sequinsConfig{
			Sharding: shardingConfig{
				ProxyTimeout:      duration{30 * time.Millisecond},
				ProxyStageTimeout: duration{10 * time.Millisecond},
			},
		},
	},
}

func httptestHost(s *httptest.Server) string {
	parsed, _ := url.Parse(s.URL)
	return parsed.Host
}

func TestProxySinglePeer(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "all good")
	}))

	peers := []string{httptestHost(peer)}

	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	assert.NoError(t, err, "simple proxying should work")
	assert.NotNil(t, resp, "simple proxying should work")
}

func TestProxySlowPeer(t *testing.T) {
	slowPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintln(w, "sorry, did you need something?")
	}))

	goodPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "all good")
	}))

	notReachedPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.FailNow(t, "proxying should succeed before getting to a third peer")
	}))

	peers := []string{httptestHost(slowPeer), httptestHost(goodPeer), httptestHost(notReachedPeer)}
	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	require.NoError(t, err, "proxying should work on the second peer")
	require.NotNil(t, resp, "proxying should work on the second peer")

	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "reading proxied response")
	assert.Equal(t, "all good\n", string(b))
}

func TestProxyErrorPeer(t *testing.T) {
	errorPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	goodPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "all good")
	}))

	notReachedPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Fail(t, "proxying should succeed before getting to a third peer")
	}))

	peers := []string{httptestHost(errorPeer), httptestHost(goodPeer), httptestHost(notReachedPeer)}
	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	require.NoError(t, err, "proxying should work on the second peer")
	require.NotNil(t, resp, "proxying should work on the second peer")

	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "reading proxied response")
	assert.Equal(t, "all good\n", string(b))
}

func TestProxySlowPeerErrorPeer(t *testing.T) {
	slowPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Millisecond)
		fmt.Fprintln(w, "all good, sorry to keep you waiting")
	}))

	errorPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	peers := []string{httptestHost(slowPeer), httptestHost(errorPeer)}
	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	require.NoError(t, err, "proxying should work on the first peer")
	require.NotNil(t, resp, "proxying should work on the first peer")

	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err, "reading proxied response")
	assert.Equal(t, "all good, sorry to keep you waiting\n", string(b))
}

func TestProxyTimeout(t *testing.T) {
	slowPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintln(w, "sorry, did you need something?")
	}))

	notReachedPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Fail(t, "proxying should never try a fourth peer")
	}))

	peers := []string{
		httptestHost(slowPeer),
		httptestHost(slowPeer),
		httptestHost(slowPeer),
		httptestHost(notReachedPeer),
	}

	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	assert.Equal(t, errProxyTimeout, err, "proxying should time out with all slow peers")
	assert.Nil(t, resp, "proxying should time out with all slow peers")
}

func TestProxyErrors(t *testing.T) {
	errorPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	peers := []string{
		httptestHost(errorPeer),
		httptestHost(errorPeer),
		httptestHost(errorPeer),
	}

	r, _ := http.NewRequest("GET", "http://localhost", nil)
	resp, err := proxyTestVersion.proxyRequest(r, peers)
	assert.Equal(t, errNoAvailablePeers, err, "proxying should return errNoAvailablePeers if all error")
	assert.Nil(t, resp, "proxying should return errNoAvailablePeers if all error")
}
