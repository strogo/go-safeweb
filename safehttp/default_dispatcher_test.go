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

package safehttp_test

import (
	"github.com/google/go-safeweb/safehttp"
	"github.com/google/go-safeweb/safehttp/safehttptest"
	"github.com/google/safehtml"
	safetemplate "github.com/google/safehtml/template"
	"html/template"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestDefaultDispatcherValidResponse(t *testing.T) {
	tests := []struct {
		name        string
		write       func(w http.ResponseWriter) error
		wantStatus  safehttp.StatusCode
		wantHeaders map[string][]string
		wantBody    string
	}{
		{
			name: "Safe HTML Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				return d.Write(w, safehtml.HTMLEscaped("<h1>Hello World!</h1>"))
			},
			wantBody: "&lt;h1&gt;Hello World!&lt;/h1&gt;",
		},
		{
			name: "Safe HTML Template Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				t := safehttp.Template(safetemplate.Must(safetemplate.New("name").Parse("<h1>{{ . }}</h1>")))
				var data interface{}
				data = "This is an actual heading, though."
				resp := safehttp.TemplateResponse{
					Template: &t,
					Data:     &data,
					FuncMap:  map[string]interface{}{},
				}
				return d.ExecuteTemplate(w, resp)
			},
			wantBody: "<h1>This is an actual heading, though.</h1>",
		},
		{
			name: "Safe HTML Template Response with Token",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				noop := func() string { panic("this function should never be called") }
				t := safehttp.Template(safetemplate.Must(safetemplate.New("name").Funcs(map[string]interface{}{"Token": noop}).Parse(`<form><input type="hidden" name="token" value="{{Token}}">{{.}}</form>`)))
				var data interface{}
				data = "Content"
				fm := map[string]interface{}{
					"Token": func() string { return "Token-secret" },
				}
				resp := safehttp.TemplateResponse{
					Template: &t,
					Data:     &data,
					FuncMap:  fm,
				}

				return d.ExecuteTemplate(w, resp)
			},
			wantBody: `<form><input type="hidden" name="token" value="Token-secret">Content</form>`,
		},
		{
			name: "Safe HTML Template Response with  Nonce",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				noop := func() string { panic("this function should never be called") }
				t := safehttp.Template(safetemplate.Must(safetemplate.New("name").Funcs(map[string]interface{}{"Nonce": noop}).Parse(`<script nonce="{{Nonce}}" type="application/javascript">alert("script")</script><h1>{{.}}</h1>`)))
				var data interface{}
				data = "Content"
				fm := map[string]interface{}{
					"Nonce": func() string { return "Nonce-secret" },
				}
				resp := safehttp.TemplateResponse{
					Template: &t,
					Data:     &data,
					FuncMap:  fm,
				}

				return d.ExecuteTemplate(w, resp)
			},
			wantBody: `<script nonce="Nonce-secret" type="application/javascript">alert("script")</script><h1>Content</h1>`,
		},
		{
			name: "Valid JSON Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				data := struct {
					Field string `json:"field"`
				}{Field: "myField"}
				return d.WriteJSON(w, safehttp.JSONResponse{Data: data})
			},
			wantBody: ")]}',\n{\"field\":\"myField\"}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &strings.Builder{}
			rw := safehttptest.NewTestResponseWriter(b)

			err := tt.write(rw)

			if err != nil {
				t.Errorf("tt.write(rw): got error %v, want nil", err)
			}

			if gotBody := b.String(); tt.wantBody != gotBody {
				t.Errorf("response body: got %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}

func TestDefaultDispatcherInvalidResponse(t *testing.T) {
	tests := []struct {
		name  string
		write func(w http.ResponseWriter) error
		want  string
	}{
		{
			name: "Unsafe HTML Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				return d.Write(w, "<h1>Hello World!</h1>")
			},
			want: "",
		},
		{
			name: "Unsafe Template Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				t := safehttp.Template(template.Must(template.New("name").Parse("<h1>{{ . }}</h1>")))
				var data interface{}
				data = "This is an actual heading, though."
				resp := safehttp.TemplateResponse{
					Template: &t,
					Data:     &data,
					FuncMap:  map[string]interface{}{},
				}
				return d.ExecuteTemplate(w, resp)
			},
			want: "",
		},
		{
			name: "Invalid JSON Response",
			write: func(w http.ResponseWriter) error {
				d := &safehttp.DefaultDispatcher{}
				return d.WriteJSON(w, safehttp.JSONResponse{Data: math.Inf(1)})
			},
			want: ")]}',\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &strings.Builder{}
			rw := safehttptest.NewTestResponseWriter(b)

			if err := tt.write(rw); err == nil {
				t.Error("tt.write(rw): got nil, want error")
			}

			if want, got := tt.want, b.String(); want != got {
				t.Errorf("response body: got %q, want %q", got, want)
			}
		})
	}
}
