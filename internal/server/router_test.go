package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRouter_Empty(t *testing.T) {
	router := testRouter(t)

	statusCode, _ := sendRequest(router, "http://example.com/")

	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
}

func TestRouter_ActiveServiceForHost(t *testing.T) {
	router := testRouter(t)
	_, target := testBackend(t, "first", http.StatusOK)

	require.NoError(t, router.SetServiceTarget("dummy.example.com", target, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://dummy.example.com/")

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)
}

func TestRouter_ActiveServiceWithoutHost(t *testing.T) {
	router := testRouter(t)
	_, target := testBackend(t, "first", http.StatusOK)

	require.NoError(t, router.SetServiceTarget("", target, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://dummy.example.com/")

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)
}

func TestRouter_ReplacingActiveService(t *testing.T) {
	router := testRouter(t)
	_, first := testBackend(t, "first", http.StatusOK)
	_, second := testBackend(t, "second", http.StatusOK)

	require.NoError(t, router.SetServiceTarget("dummy.example.com", first, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://dummy.example.com/")

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)

	require.NoError(t, router.SetServiceTarget("dummy.example.com", second, DefaultDeployTimeout))

	statusCode, body = sendRequest(router, "http://dummy.example.com/")

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "second", body)
}

func TestRouter_RoutingMultipleHosts(t *testing.T) {
	router := testRouter(t)
	_, first := testBackend(t, "first", http.StatusOK)
	_, second := testBackend(t, "second", http.StatusOK)

	require.NoError(t, router.SetServiceTarget("s1.example.com", first, DefaultDeployTimeout))
	require.NoError(t, router.SetServiceTarget("s2.example.com", second, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://s1.example.com/")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)

	statusCode, body = sendRequest(router, "http://s2.example.com/")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "second", body)
}

func TestRouter_TargetWithoutHostActsAsWildcard(t *testing.T) {
	router := testRouter(t)
	_, first := testBackend(t, "first", http.StatusOK)
	_, second := testBackend(t, "second", http.StatusOK)

	require.NoError(t, router.SetServiceTarget("s1.example.com", first, DefaultDeployTimeout))
	require.NoError(t, router.SetServiceTarget("", second, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://s1.example.com/")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)

	statusCode, body = sendRequest(router, "http://s2.example.com/")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "second", body)

	statusCode, body = sendRequest(router, "http://s3.example.com/")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "second", body)
}

func TestRouter_ServiceFailingToBecomeHealthy(t *testing.T) {
	router := testRouter(t)
	_, target := testBackend(t, "", http.StatusInternalServerError)

	err := router.SetServiceTarget("example.com", target, time.Millisecond*20)
	assert.Equal(t, ErrorTargetFailedToBecomeHealthy, err)

	statusCode, _ := sendRequest(router, "http://dummy.example.com/")

	assert.Equal(t, http.StatusServiceUnavailable, statusCode)
}

func TestRouter_RestoreLastSavedState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")

	_, first := testBackend(t, "first", http.StatusOK)
	_, second := testBackend(t, "second", http.StatusOK)
	second.options = TargetOptions{TLSHostname: "other.example.com"}

	router := NewRouter(statePath)
	require.NoError(t, router.SetServiceTarget("", first, DefaultDeployTimeout))
	require.NoError(t, router.SetServiceTarget("other.example.com", second, DefaultDeployTimeout))

	statusCode, body := sendRequest(router, "http://something.example.com")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)

	statusCode, _ = sendRequest(router, "http://other.example.com/")
	assert.Equal(t, http.StatusMovedPermanently, statusCode)

	router = NewRouter(statePath)
	router.RestoreLastSavedState()

	statusCode, body = sendRequest(router, "http://something.example.com")
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, "first", body)

	statusCode, _ = sendRequest(router, "http://other.example.com/")
	assert.Equal(t, http.StatusMovedPermanently, statusCode)
}

// Helpers

func testRouter(t *testing.T) *Router {
	statePath := filepath.Join(t.TempDir(), "state.json")
	return NewRouter(statePath)
}

func sendRequest(router *Router, url string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Result().StatusCode, string(w.Body.String())
}
