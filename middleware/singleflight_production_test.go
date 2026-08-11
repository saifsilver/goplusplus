package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/middleware"
)

func TestSingleflightDoesNotShareAcrossAuthenticatedUsers(t *testing.T) {
	app := gpp.New()
	app.Use(func(c *gpp.Context) error {
		c.Set("sub", c.GetHeader("X-Test-Subject"))
		return c.Next()
	}, middleware.Singleflight())

	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/profile", func(c *gpp.Context) error {
		executions.Add(1)
		started <- struct{}{}
		<-release
		return c.String(http.StatusOK, "%s", c.UserSubject())
	})

	responses := make(chan string, 2)
	for _, subject := range []string{"user-a", "user-b"} {
		go func() {
			request := httptest.NewRequest(http.MethodGet, "/profile", nil)
			request.Header.Set("X-Test-Subject", subject)
			recorder := httptest.NewRecorder()
			app.ServeHTTP(recorder, request)
			responses <- recorder.Body.String()
		}()
	}
	waitForSignals(t, started, 2)
	close(release)
	seen := map[string]bool{<-responses: true, <-responses: true}
	if !seen["user-a"] || !seen["user-b"] {
		t.Fatalf("cross-user responses = %#v", seen)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightDoesNotReplayHandlerErrors(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/failure", func(*gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		return errors.New("backend failed")
	})

	responses := make(chan int, 2)
	request := func() {
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))
		responses <- recorder.Code
	}
	go request()
	<-started
	go request()
	close(release)
	for range 2 {
		if status := <-responses; status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", status)
		}
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightPanicDoesNotBlockFollowers(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Recovery(), middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/panic", func(*gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		panic("boom")
	})

	done := make(chan struct{}, 2)
	request := func() {
		app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/panic", nil))
		done <- struct{}{}
	}
	go request()
	<-started
	go request()
	close(release)
	waitForSignals(t, done, 2)
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightWaiterHonorsCancellation(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	app.GET("/slow", func(c *gpp.Context) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		return c.String(http.StatusOK, "done")
	})

	leaderDone := make(chan struct{})
	go func() {
		app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
		close(leaderDone)
	}()
	<-started
	waiterContext, cancel := context.WithCancel(context.Background())
	waiterDone := make(chan struct{})
	go func() {
		request := httptest.NewRequest(http.MethodGet, "/slow", nil).WithContext(waiterContext)
		app.ServeHTTP(httptest.NewRecorder(), request)
		close(waiterDone)
	}()
	cancel()
	select {
	case <-waiterDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled follower remained blocked")
	}
	close(release)
	<-leaderDone
}

func TestSingleflightDoesNotShareSetCookieResponses(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/session", func(c *gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: "session", Value: "unique"})
		return c.String(http.StatusOK, "created")
	})

	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/session", nil))
		}()
	}
	<-started
	close(release)
	wait.Wait()
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightHonorsDynamicVaryHeaders(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/variant", func(c *gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		c.SetHeader("Vary", "X-Variant")
		return c.String(http.StatusOK, "%s", c.GetHeader("X-Variant"))
	})

	responses := make(chan string, 2)
	request := func(variant string) {
		req := httptest.NewRequest(http.MethodGet, "/variant", nil)
		req.Header.Set("X-Variant", variant)
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, req)
		responses <- recorder.Body.String()
	}
	go request("a")
	<-started
	go request("b")
	close(release)
	seen := map[string]bool{<-responses: true, <-responses: true}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("variant responses = %#v", seen)
	}
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightDoesNotShareOversizedResponses(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.Singleflight(middleware.SingleflightConfig{MaxResponseBytes: 4}))
	started := make(chan struct{})
	release := make(chan struct{})
	var executions atomic.Int32
	app.GET("/large", func(c *gpp.Context) error {
		if executions.Add(1) == 1 {
			close(started)
			<-release
		}
		return c.String(http.StatusOK, "larger-than-four")
	})

	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			app.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/large", nil))
		}()
	}
	<-started
	close(release)
	wait.Wait()
	if got := executions.Load(); got != 2 {
		t.Fatalf("handler executions = %d, want 2", got)
	}
}

func TestSingleflightPreservesFollowerRequestID(t *testing.T) {
	app := gpp.New()
	app.Use(middleware.RequestID(), middleware.Singleflight())
	started := make(chan struct{})
	release := make(chan struct{})
	app.GET("/resource", func(c *gpp.Context) error {
		select {
		case <-started:
		default:
			close(started)
			<-release
		}
		return c.String(http.StatusOK, "resource")
	})

	type result struct {
		requestID string
		cache     string
	}
	results := make(chan result, 2)
	request := func(requestID string) {
		req := httptest.NewRequest(http.MethodGet, "/resource", nil)
		req.Header.Set("X-Request-ID", requestID)
		recorder := httptest.NewRecorder()
		app.ServeHTTP(recorder, req)
		results <- result{requestID: recorder.Header().Get("X-Request-ID"), cache: recorder.Header().Get("X-Singleflight")}
	}
	go request("request-a")
	<-started
	go request("request-b")
	time.Sleep(10 * time.Millisecond)
	close(release)
	first, second := <-results, <-results
	seen := map[string]bool{first.requestID: true, second.requestID: true}
	if !seen["request-a"] || !seen["request-b"] {
		t.Fatalf("response request IDs = %#v", seen)
	}
	if first.cache != "HIT" && second.cache != "HIT" {
		t.Fatal("requests were not coalesced")
	}
}

func waitForSignals(t *testing.T, signals <-chan struct{}, count int) {
	t.Helper()
	for range count {
		select {
		case <-signals:
		case <-time.After(time.Second):
			t.Fatalf("received fewer than %d signals", count)
		}
	}
}
