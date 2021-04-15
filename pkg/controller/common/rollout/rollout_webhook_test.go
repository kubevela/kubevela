/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rollout

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const mockUrlBase = "127.0.0.1:"

func TestMakeHTTPRequest(t *testing.T) {
	ctx := context.TODO()
	type mockHTTPParameter struct {
		method     string
		statusCode int
		body       string
	}
	type want struct {
		err        error
		statusCode int
		body       string
	}
	tests := map[string]struct {
		url           string
		method        string
		payload       interface{}
		httpParameter mockHTTPParameter
		want          want
	}{
		"Test normal case": {
			method:  http.MethodPost,
			payload: "doesn't matter",
			httpParameter: mockHTTPParameter{
				method:     http.MethodPost,
				statusCode: http.StatusAccepted,
				body:       "all good",
			},
			want: want{
				err:        nil,
				statusCode: http.StatusAccepted,
				body:       "all good",
			},
		},
		"Test http failed case with retry": {
			url:     "127.0.0.1:13622",
			method:  http.MethodPost,
			payload: "doesn't matter",
			httpParameter: mockHTTPParameter{
				method:     http.MethodGet,
				statusCode: http.StatusAccepted,
				body:       "doesn't matter",
			},
			want: want{
				err:        fmt.Errorf("internal server error, status code = %d", http.StatusNotImplemented),
				statusCode: -1,
				body:       "",
			},
		},
		"Test failed case with retry": {
			method:  http.MethodPost,
			payload: "doesn't matter",
			httpParameter: mockHTTPParameter{
				method:     http.MethodPost,
				statusCode: http.StatusNotImplemented,
				body:       "please retry",
			},
			want: want{
				err:        fmt.Errorf("internal server error, status code = %d", http.StatusNotImplemented),
				statusCode: http.StatusNotImplemented,
				body:       "",
			},
		},
		"Test client error failed case": {
			method:  http.MethodPost,
			payload: "doesn't matter",
			httpParameter: mockHTTPParameter{
				method:     http.MethodPost,
				statusCode: http.StatusBadRequest,
				body:       "bad request",
			},
			want: want{
				err:        nil,
				statusCode: http.StatusBadRequest,
				body:       "bad request",
			},
		},
	}
	for testName, tt := range tests {
		func(testName string) {
			mockUrl := mockUrlBase + strconv.FormatInt(rand.Int63n(128)+1000, 10)
			// generate a test server so we can capture and inspect the request
			testServer := NewMock(tt.httpParameter.method, mockUrl, tt.httpParameter.statusCode, tt.httpParameter.body)
			defer testServer.Close()
			if len(tt.url) == 0 {
				tt.url = mockUrl
			}
			gotReply, gotCode, gotErr := makeHTTPRequest(ctx, "http://"+tt.url, tt.method, tt.payload)
			if gotCode != tt.want.statusCode {
				t.Errorf("\n%s\nr.Reconcile(...): want code `%d`, got code:`%d` got err: %v \n", testName, tt.want.statusCode,
					gotCode, gotErr)
			}
			if gotCode == -1 {
				// we don't know exactly what error we should get when the network call failed
				if gotErr == nil {
					t.Errorf("\n%s\nr.Reconcile(...): want some error, got error:`%s`\n", testName, gotErr)
				}
			} else {
				if (tt.want.err == nil && gotErr != nil) || (tt.want.err != nil && gotErr == nil) {
					t.Errorf("\n%s\nr.Reconcile(...): want error `%s`, got error:`%s`\n", testName, tt.want.err, gotErr)
				}
				if tt.want.err != nil && gotErr != nil && gotErr.Error() != tt.want.err.Error() {
					t.Errorf("\n%s\nr.Reconcile(...): want error `%s`, got error:`%s`\n", testName, tt.want.err, gotErr)
				}
			}
			if string(gotReply) != tt.want.body {
				t.Errorf("\n%s\nr.Reconcile(...): want reply `%s`, got reply:`%s`\n", testName, tt.want.body, string(gotReply))
			}
		}(testName)
	}
}

func TestCallWebhook(t *testing.T) {
	ctx := context.TODO()
	body := "all good"
	res := v1alpha1.PodSpecWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	type args struct {
		resource oam.Object
		phase    string
		rw       v1alpha1.RolloutWebhook
	}
	tests := map[string]struct {
		returnedStatusCode int
		args               args
		wantErr            error
	}{
		"Test success case": {
			returnedStatusCode: http.StatusAccepted,
			args: args{
				resource: &res,
				phase:    string(v1alpha1.RollingInBatchesState),
				rw:       v1alpha1.RolloutWebhook{},
			},
			wantErr: nil,
		},
		"Test failed default case": {
			returnedStatusCode: http.StatusAlreadyReported,
			args: args{
				resource: &res,
				phase:    string(v1alpha1.RollingInBatchesState),
				rw:       v1alpha1.RolloutWebhook{},
			},
			wantErr: fmt.Errorf("we fail the webhook request based on status, http status = %d", http.StatusAlreadyReported),
		},
		"Test expected treated as success case": {
			returnedStatusCode: http.StatusAlreadyReported,
			args: args{
				resource: &res,
				phase:    string(v1alpha1.RollingInBatchesState),
				rw: v1alpha1.RolloutWebhook{
					ExpectedStatus: []int{http.StatusNoContent, http.StatusAlreadyReported},
				},
			},
			wantErr: nil,
		},
		"Test not expected treated as failed case": {
			returnedStatusCode: http.StatusGone,
			args: args{
				resource: &res,
				phase:    string(v1alpha1.RolloutFailedState),
				rw: v1alpha1.RolloutWebhook{
					ExpectedStatus: []int{http.StatusNoContent, http.StatusAlreadyReported},
				},
			},
			wantErr: fmt.Errorf("http request to the webhook not accepeted, http status = %d", http.StatusGone),
		},
	}
	for name, tt := range tests {
		func(name string) {
			url := mockUrlBase + strconv.FormatInt(rand.Int63n(4848)+1000, 10)
			tt.args.rw.URL = "http://" + url
			// generate a test server so we can capture and inspect the request
			testServer := NewMock(http.MethodPost, url, tt.returnedStatusCode, body)
			defer testServer.Close()

			gotErr := callWebhook(ctx, tt.args.resource, tt.args.phase, tt.args.rw)
			if (tt.wantErr == nil && gotErr != nil) || (tt.wantErr != nil && gotErr == nil) {
				t.Errorf("\n%s\nr.Reconcile(...): want error `%s`, got error:`%s`\n", name, tt.wantErr, gotErr)
			}
			if tt.wantErr != nil && gotErr != nil && gotErr.Error() != tt.wantErr.Error() {
				t.Errorf("\n%s\nr.Reconcile(...): want error `%s`, got error:`%s`\n", name, tt.wantErr, gotErr)
			}
		}(name)
	}
}

func NewMock(method, mockUrl string, statusCode int, body string) *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == method {
			w.WriteHeader(statusCode)
			w.Write([]byte(body))
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	l, err := net.Listen("tcp", mockUrl)
	if err != nil {
		panic(err)
	}
	ts.Listener.Close()
	ts.Listener = l
	ts.Start()
	return ts
}
