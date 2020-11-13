// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package safehttp

import (
	"net/http"
)

type Task struct {
	rw  http.ResponseWriter
	req *IncomingRequest

	cfg HandlerConfig

	code   StatusCode
	header Header

	written      bool
	writtenError bool
}

// NewTask creates a ResponseWriter from a safehttp.Dispatcher, an
// http.ResponseWriter and a reference to the current IncomingRequest being served.
// The IncomingRequest will only be used by the commit phase, which only runs when
// the ServeMux is used, and can be passed as nil in tests.
// TODO: remove the IncomingRequest parameter.

// ProperNewTask TODO
func BrokenNewTask(rw http.ResponseWriter, dispatcher Dispatcher) *Task {
	return &Task{
		cfg:    HandlerConfig{Dispatcher: dispatcher},
		rw:     rw,
		header: newHeader(rw.Header()),
	}
}

type HandlerConfig struct {
	Handler      Handler
	Dispatcher   Dispatcher
	Interceptors []ConfiguredInterceptor
}

func ProcessRequest(cfg HandlerConfig, rw http.ResponseWriter, req *http.Request) {
	t := &Task{
		cfg:    cfg,
		rw:     rw,
		header: newHeader(rw.Header()),
		req:    NewIncomingRequest(req),
	}
	t.processRequest()
}

func (t *Task) processRequest() {
	// The `net/http` package recovers handler panics, but we cannot rely on that behavior here.
	// The reason is, we might need to run After/Commit stages of the interceptors before we
	// respond with a 500 Internal Server Error.
	defer func() {
		if r := recover(); r != nil {
			t.WriteError(StatusInternalServerError)
		}
	}()

	for _, it := range t.cfg.Interceptors {
		it.Before(t, t.req)
		if t.written {
			return
		}
	}

	t.cfg.Handler.ServeHTTP(t, t.req)
	if !t.written {
		t.NoContent()
	}
}

// Write dispatches the response to the Dispatcher. This will be written to the
// underlying http.ResponseWriter if the Dispatcher decides it's safe to do so.
//
// TODO: replace panics with proper error handling when writing the response fails.
func (t *Task) Write(resp Response) Result {
	if t.written {
		panic("ResponseWriter was already written to")
	}
	t.written = true
	t.commitPhase(resp)

	ct, err := t.cfg.Dispatcher.ContentType(resp)
	if err != nil {
		panic(err)
	}
	t.rw.Header().Set("Content-Type", ct)

	if t.code == 0 {
		t.code = StatusOK
	}
	t.rw.WriteHeader(int(t.code))

	if err := t.cfg.Dispatcher.Write(t.rw, resp); err != nil {
		panic(err)
	}
	return Result{}
}

// NoContent responds with a 204 No Content response.
//
// If the ResponseWriter has already been written to, then this method will panic.
func (t *Task) NoContent() Result {
	if t.written {
		panic("ResponseWriter was already written to")
	}
	t.written = true
	t.commitPhase(NoContentResponse{})
	t.rw.WriteHeader(int(StatusNoContent))
	return Result{}
}

// WriteError writes an error response (400-599) according to the provided
// status code.
//
// If the ResponseWriter has already been written to, then this method will panic.
func (t *Task) WriteError(code StatusCode) Result {
	// TODO: accept custom error responses that need to go through the dispatcher.
	if t.writtenError {
		panic("ResponseWriter.WriteError called twice")
	}
	t.written = true
	t.writtenError = true
	resp := &ErrorResponse{Code: code}
	t.errorPhase(resp)
	http.Error(t.rw, http.StatusText(int(resp.Code)), int(resp.Code))
	return Result{}
}

// Redirect responds with a redirect to the given url, using code as the status code.
//
// If the ResponseWriter has already been written to, then this method will panic.
func (t *Task) Redirect(r *IncomingRequest, url string, code StatusCode) Result {
	if code < 300 || code >= 400 {
		panic("wrong method called")
	}
	if t.written {
		panic("ResponseWriter was already written to")
	}
	t.written = true
	http.Redirect(t.rw, r.req, url, int(code))
	return Result{}
}

// Header returns the collection of headers that will be set
// on the response. Headers must be set before writing a
// response (e.g. Write, WriteTemplate).
func (t *Task) Header() Header {
	return t.header
}

// SetCookie adds a Set-Cookie header to the provided ResponseWriter's headers.
// The provided cookie must have a valid Name, otherwise an error will be
// returned.
func (t *Task) SetCookie(c *Cookie) error {
	return t.header.addCookie(c)
}

// SetCode allows setting a response status. If the response was already
// written, trying to set the status code will have no effect. This method will
// panic if an invalid status code is passed (i.e. not in the range 1XX-5XX).
//
// If SetCode was called before NoContent, Redirect or WriteError, the status
// code set by the latter will be the actual response status.
//
// TODO(empijei@, kele@, maramihali@): decide what should be done if the
// code passed is either 3XX (redirect) or 4XX-5XX (client/server error).
func (t *Task) SetCode(code StatusCode) {
	if t.written {
		return
	}
	if code < 100 || code >= 600 {
		panic("invalid status code")
	}
	t.code = code
}

// commitPhase calls the Commit phases of all the interceptors. This stage will
// run before a response is written to the ResponseWriter. If a response is
// written to the ResponseWriter in a Commit phase then the Commit phases of the
// remaining interceptors won't execute.
//
// TODO: BIG WARNING, if ResponseWriter.Write and ResponseWriter.WriteTemplate
// are called in Commit then this will recurse. CommitResponseWriter was an
// attempt to prevent this by not giving access to Write and WriteTemplate in
// the Commit phase.
func (t *Task) commitPhase(resp Response) {
	for i := len(t.cfg.Interceptors) - 1; i >= 0; i-- {
		t.cfg.Interceptors[i].Commit(t, t.req, resp)
	}
}

// errorPhase calls the OnError phases of all the interceptors associated with
// a handler. This stage runs before an error response is written to the
// ResponseWriter.
//
// TODO: BIG WARNING, if an interceptor attempts to write to the ResponseWriter
// in the OnError phase will result in an irrecoverable error.
func (t *Task) errorPhase(resp Response) {
	for i := len(t.cfg.Interceptors) - 1; i >= 0; i-- {
		t.cfg.Interceptors[i].OnError(t, t.req, resp)
	}
}

// Dispatcher is responsible for writing a response received from the
// ResponseWriter to the underlying http.ResponseWriter.
//
// The implementation of a custom Dispatcher should be thoroughly reviewed by
// the security team to avoid introducing vulnerabilities.
type Dispatcher interface {
	// Content-Type returns the Content-Type of the provided response if it is
	// of a safe type, supported by the Dispatcher, and should return an error
	// otherwise.
	//
	// Sending a response to the http.ResponseWriter without properly setting
	// CT is error-prone and could introduce vulnerabilities. Therefore, this
	// method should be used to set the Content-Type header before calling
	// Dispatcher.Write. Writing should not proceed if ContentType returns an
	// error.
	ContentType(resp Response) (string, error)

	// Write writes a Response to the underlying http.ResponseWriter.
	//
	// It should return an error if the writing operation fails or if the
	// provided Response should not be written to the http.ResponseWriter
	// because it's unsafe.
	Write(rw http.ResponseWriter, resp Response) error
}

// Result is the result of writing an HTTP response.
//
// Use ResponseWriter methods to obtain it.
type Result struct{}

// NotWritten returns a Result which indicates that nothing has been written yet. It
// can be used in all functions that return a Result, such as in the ServeHTTP method
// of a Handler or in the Before method of an Interceptor. When returned, NotWritten
// indicates that the writing of the response should take place later. When this
// is returned by the Before method in Interceptors the next Interceptor in line
// is run. When this is returned by a Handler, a 204 No Content response is written.
func NotWritten() Result {
	return Result{}
}
