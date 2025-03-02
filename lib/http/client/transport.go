package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	outgoinghandler "github.com/dev-wasm/dev-wasm-go/lib/wasi/http/outgoing-handler"
	"github.com/dev-wasm/dev-wasm-go/lib/wasi/http/types"
	"go.bytecodealliance.org/cm"
)

const DEFAULT_USER_AGENT = "WASI-HTTP-Go/0.0.2"

func OK[Shape, T, Err any](val cm.Result[Shape, T, Err]) *T {
	return (&val).OK()
}

type bytesReaderCloser struct {
	*bytes.Reader
}

// Close implements io.Closer.Close.
func (bytesReaderCloser) Close() error {
	return nil
}

func BodyReaderCloser(b []byte) io.ReadCloser {
	return bytesReaderCloser{bytes.NewReader(b)}
}

func schemeFromString(s string) types.Scheme {
	switch s {
	case "http":
		return types.SchemeHTTP()
	case "https":
		return types.SchemeHTTPS()
	default:
		return types.SchemeOther(s)
	}
}

func methodFromString(m string) types.Method {
	switch m {
	case "GET":
		return types.MethodGet()
	case "PUT":
		return types.MethodPut()
	case "POST":
		return types.MethodPost()
	case "DELETE":
		return types.MethodDelete()
	case "OPTIONS":
		return types.MethodOptions()
	case "PATCH":
		return types.MethodPatch()
	case "CONNECT":
		return types.MethodConnect()
	case "TRACE":
		return types.MethodTrace()
	default:
		return types.MethodOther(m)
	}
}

func Put(client *http.Client, uri, contentType string, body io.ReadCloser) (*http.Response, error) {
	u, e := url.Parse(uri)
	if e != nil {
		return nil, e
	}
	req := http.Request{
		Method: "PUT",
		URL:    u,
		Body:   body,
		Header: make(http.Header),
	}
	req.Header["Content-type"] = []string{contentType}
	return client.Do(&req)
}

type WasiRoundTripper struct{}

func initHeaders(r *http.Request) {
	if r.Header == nil {
		r.Header = http.Header{}
	}
	if _, ok := r.Header["User-Agent"]; !ok {
		r.Header["User-Agent"] = []string{DEFAULT_USER_AGENT}
	}
	if r.Close {
		r.Header["Connection"] = []string{"close"}
	}
	if r.Body != nil {
		if _, ok := r.Header["Content-Length"]; !ok {
			if r.ContentLength > 0 {
				r.Header["Content-Length"] = []string{strconv.Itoa(int(r.ContentLength))}
			}
		}
	}
}

func makeHeaders(r *http.Request) cm.List[cm.Tuple[types.FieldKey, types.FieldValue]] {
	strstr := []cm.Tuple[types.FieldKey, types.FieldValue]{}
	for k, v := range r.Header {
		for _, str := range v {
			strstr = append(
				strstr,
				cm.Tuple[types.FieldKey, types.FieldValue]{
					F0: types.FieldKey(k),
					F1: types.FieldValue(cm.ToList([]uint8(str))),
				},
			)
		}
	}
	return cm.ToList(strstr)
}

func getAuthority(r *http.Request) string {
	if len(r.Host) > 0 {
		return r.Host
	} else {
		return r.URL.Host
	}
}

func populateResponseHeaders(fields cm.List[cm.Tuple[types.FieldKey, types.FieldValue]], r *http.Response) {
	if r.Header == nil {
		r.Header = http.Header{}
	}
	for _, field := range fields.Slice() {
		key := string(field.F0)
		value := string(field.F1.Slice())
		r.Header[key] = append(r.Header[key], value)
	}
}

func (WasiRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	initHeaders(r)

	res := types.FieldsFromList(makeHeaders(r))
	headers := res.OK()

	method := methodFromString(r.Method)
	scheme := cm.Some(schemeFromString(r.URL.Scheme))

	path_with_query := cm.Some(r.URL.RequestURI())

	authority := cm.Some(getAuthority(r))

	req := types.NewOutgoingRequest(*headers)
	req.SetMethod(method)
	req.SetPathWithQuery(path_with_query)
	req.SetScheme(scheme)
	req.SetAuthority(authority)

	body := OK(req.Body())
	if r.Body != nil {
		b, err := io.ReadAll(io.Reader(r.Body))
		if err != nil {
			return nil, err
		}
		s := OK(body.Write())
		s.BlockingWriteAndFlush(cm.ToList([]uint8(b)))
		s.ResourceDrop()
	}

	hRes := outgoinghandler.Handle(req, cm.None[types.RequestOptions]())
	if !hRes.IsOK() {
		panic("Failed to call client.")
	}

	types.OutgoingBodyFinish(*body, cm.None[types.Fields]())

	future := hRes.OK()
	defer future.ResourceDrop()
	resultOption := future.Get()
	if !resultOption.None() {
		return nil, fmt.Errorf("result already taken")
	}
	poll := future.Subscribe()
	defer poll.ResourceDrop()
	poll.Block()
	resultOption = future.Get()
	result := resultOption.Some().OK().OK()
	defer result.ResourceDrop()

	response := http.Response{
		StatusCode: int(result.Status()),
		Header:     http.Header{},
	}

	responseHeaders := result.Headers()
	entries := responseHeaders.Entries()
	populateResponseHeaders(entries, &response)

	responseHeaders.ResourceDrop()

	responseBody := OK(result.Consume())
	defer responseBody.ResourceDrop()
	stream := OK(responseBody.Stream())
	defer stream.ResourceDrop()
	inputPoll := stream.Subscribe()
	defer inputPoll.ResourceDrop()

	data := []uint8{}
	for {
		inputPoll.Block()
		dataResult := stream.Read(64 * 1024)
		if dataResult.IsOK() {
			data = append(data, dataResult.OK().Slice()...)
		} else if dataResult.Err().Closed() {
			break
		} else {
			return nil, fmt.Errorf("error reading response stream")
		}
	}

	response.Body = bytesReaderCloser{bytes.NewReader(data)}

	return &response, nil
}
