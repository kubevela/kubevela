package rollout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

func Test_callWebhook(t *testing.T) {
	ctx := context.TODO()
	type args struct {
		resource klog.KMetadata
		phase    v1alpha1.RollingState
		w        v1alpha1.RolloutWebhook
	}
	var tests []struct {
		name       string
		statusCode int
		body       string
		args       args
		wantErr    bool
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// generate a test server so we can capture and inspect the request
			testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				res.WriteHeader(tt.statusCode)
				res.Write([]byte(tt.body))
			}))
			defer func() { testServer.Close() }()

			if err := callWebhook(ctx, tt.args.resource, tt.args.phase, tt.args.w); (err != nil) != tt.wantErr {
				t.Errorf("callWebhook() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
