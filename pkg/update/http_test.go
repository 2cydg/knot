package update

import (
	"io"
	"net/http"
	"strings"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testHTTPClient(routes map[string]testHTTPResponse) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		route, ok := routes[req.URL.String()]
		if !ok {
			return testResponse(http.StatusNotFound, "missing"), nil
		}
		return testResponse(route.status, route.body), nil
	})}
}

type testHTTPResponse struct {
	status int
	body   string
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
