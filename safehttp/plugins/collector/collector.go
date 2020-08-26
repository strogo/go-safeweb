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

package collector

import (
	"encoding/json"
	"io/ioutil"

	"github.com/google/go-safeweb/safehttp"
)

// Report represents a generic report as specified by https://w3c.github.io/reporting/#serialize-reports
type Report struct {
	// Type represents the type of the report. This will control how Body looks
	// like.
	Type string
	// Age represents the number of milliseconds since the violation causing the
	// report occured.
	Age uint64
	// URL is the address of the Document or Worker from which the report was
	// generated.
	URL string
	// UserAgent contains the value of the User-Agent header of the request from
	// which the report was generated.
	UserAgent string
	// Body contains the body of the report. This will be different for every Type.
	// If Type is csp-violation then Body will be a CSPReport. Otherwise Body will
	// be a map[string]interface{} containing the JavaScript object that was passed
	// as unmarshalled using encoding/json.
	Body interface{}
}

// CSPReport represents a CSP violation report as specified by https://www.w3.org/TR/CSP3/#deprecated-serialize-violation
type CSPReport struct {
	// BlockedURL is the URL of the resource that was blocked from loading by the
	// Content Security Policy. If the blocked URL is from a different origin than
	// the DocumentURL, then the blocked URL is truncated to contain just the scheme,
	// host, and port.
	BlockedURL string
	// Disposition is either "enforce" or "report" depending on whether the Content-Security-Policy
	// header or the Content-Security-Policy-Report-Only header is used.
	Disposition string
	// DocumentURL is the URL of the document in which the violation occurred.
	DocumentURL string
	// EffectiveDirective is the directive whose enforcement caused the violation.
	EffectiveDirective string
	// OriginalPolicy is the original policy as specified by the Content Security
	// Policy header.
	OriginalPolicy string
	// Referrer is the referrer of the document in which the violation occurred.
	Referrer string
	// Sample is the first 40 characters of the inline script, event handler,
	// or style that caused the violation.
	Sample string
	// StatusCode the HTTP status code of the resource on which the global object
	// was instantiated.
	StatusCode uint
	// ViolatedDirective is the name of the policy section that was violated.
	ViolatedDirective string
	// SourceFile represents the URL of the document or worker in which the violation
	// was found.
	SourceFile string
	// LineNumber is the line number in the document or worker at which the violation
	// occurred.
	LineNumber uint
	// ColumnNumber is the column number in the document or worker at which the violation
	// occurred.
	ColumnNumber uint
}

// HandlerBuilder creates a safehttp.Handler based on the report handlers that
// it's created with.
type HandlerBuilder struct {
	// Handler handles all generic reports received with the Content-Type application/reports+json.
	Handler func(Report)
	// CSPHandler handles all CSP reports received with the Content-Type application/csp-report.
	CSPHandler func(CSPReport)
}

// Build builds a safehttp.Handler which calls the given Handler or CSPHandler when
// a violation report is received. Make sure to register the handler to receive POST
// requests. If the handler recieves anything other that POST requests it will
// respond with a 405 Method Not Allowed.
func (hb HandlerBuilder) Build() safehttp.Handler {
	return safehttp.HandlerFunc(func(w *safehttp.ResponseWriter, r *safehttp.IncomingRequest) safehttp.Result {
		if r.Method() != safehttp.MethodPost {
			return w.ClientError(safehttp.StatusMethodNotAllowed)
		}

		b, err := ioutil.ReadAll(r.Body())
		if err != nil {
			return w.ClientError(safehttp.StatusBadRequest)
		}

		ct := r.Header.Get("Content-Type")
		if ct == "application/csp-report" && hb.CSPHandler != nil {
			return handleDeprecatedCSPReports(hb.CSPHandler, w, b)
		} else if ct == "application/reports+json" && hb.Handler != nil {
			return handleReport(hb.Handler, w, b)
		}

		return w.ClientError(safehttp.StatusUnsupportedMediaType)
	})
}

func stringOrEmpty(x interface{}) string {
	s, ok := x.(string)
	if !ok {
		return ""
	}
	return s
}

func uintOrZero(x interface{}) uint {
	// Must assert as float64 and then convert to uint since this is how json stores
	// its numbers.
	f, ok := x.(float64)
	if !ok || f < 0 {
		return 0
	}
	return uint(f)
}

func handleDeprecatedCSPReports(h func(CSPReport), w *safehttp.ResponseWriter, b []byte) safehttp.Result {
	// In CSP2 it is clearly stated that a report has a single key 'csp-report'
	// which holds the report object. Like this:
	// {
	//   "csp-report": {
	//     // report goes here
	//   }
	// }
	// Source: https://www.w3.org/TR/CSP2/#violation-reports
	//
	// But in the CSP3 spec this 'csp-report' key is never mentioned. So the report
	// would look like this:
	// {
	//   // report goes here
	// }
	// Source: https://w3c.github.io/webappsec-csp/#deprecated-serialize-violation
	//
	// Because of this we have to support both. :/
	m := make(map[string]interface{})
	if err := json.Unmarshal(b, &m); err != nil {
		return w.ClientError(safehttp.StatusBadRequest)
	}

	if report, exists := m["csp-report"]; exists {
		var ok bool
		m, ok = report.(map[string]interface{})
		if !ok {
			return w.ClientError(safehttp.StatusBadRequest)
		}
	}

	ln := uintOrZero(m["lineno"])
	if ln == 0 {
		ln = uintOrZero(m["line-number"])
	}
	cn := uintOrZero(m["colno"])
	if cn == 0 {
		cn = uintOrZero(m["column-number"])
	}

	r := CSPReport{
		BlockedURL:         stringOrEmpty(m["blocked-uri"]),
		Disposition:        stringOrEmpty(m["disposition"]),
		DocumentURL:        stringOrEmpty(m["document-uri"]),
		EffectiveDirective: stringOrEmpty(m["effective-directive"]),
		OriginalPolicy:     stringOrEmpty(m["original-policy"]),
		Referrer:           stringOrEmpty(m["referrer"]),
		Sample:             stringOrEmpty(m["script-sample"]),
		StatusCode:         uintOrZero(m["status-code"]),
		ViolatedDirective:  stringOrEmpty(m["violated-directive"]),
		SourceFile:         stringOrEmpty(m["source-file"]),
		LineNumber:         ln,
		ColumnNumber:       cn,
	}
	h(r)

	return w.NoContent()
}

func handleReport(h func(Report), w *safehttp.ResponseWriter, b []byte) safehttp.Result {
	var rList []Report
	if err := json.Unmarshal(b, &rList); err != nil {
		return w.ClientError(safehttp.StatusBadRequest)
	}

	badRequest := false
	for _, r := range rList {
		m, ok := r.Body.(map[string]interface{})
		if !ok {
			badRequest = true
			continue
		}

		if r.Type == "csp-violation" {
			// https://w3c.github.io/webappsec-csp/#reporting
			r.Body = CSPReport{
				BlockedURL:         stringOrEmpty(m["blockedURL"]),
				Disposition:        stringOrEmpty(m["disposition"]),
				DocumentURL:        stringOrEmpty(m["documentURL"]),
				EffectiveDirective: stringOrEmpty(m["effectiveDirective"]),
				OriginalPolicy:     stringOrEmpty(m["originalPolicy"]),
				Referrer:           stringOrEmpty(m["referrer"]),
				Sample:             stringOrEmpty(m["sample"]),
				StatusCode:         uintOrZero(m["statusCode"]),
				// In CSP3 ViolatedDirective has been removed but is kept as
				// a copy of EffectiveDirective for backwards compatibility.
				ViolatedDirective: stringOrEmpty(m["effectiveDirective"]),
				SourceFile:        stringOrEmpty(m["sourceFile"]),
				LineNumber:        uintOrZero(m["lineNumber"]),
				ColumnNumber:      uintOrZero(m["columnNumber"]),
			}
		}
		h(r)
	}

	if badRequest {
		return w.ClientError(safehttp.StatusBadRequest)
	}

	return w.NoContent()
}
