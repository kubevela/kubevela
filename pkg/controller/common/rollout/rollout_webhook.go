package rollout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/common"
)

// issue an http call to the an end ponit
func makeHTTPRequest(ctx context.Context, webhookEndPoint string, payload interface{}) ([]byte, int, error) {
	payloadBin, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	hook, err := url.Parse(webhookEndPoint)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", hook.String(), bytes.NewBuffer(payloadBin))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Content-Type", "application/json")

	// issue request with retry
	var r *http.Response
	var body []byte
	err = retry.OnError(retry.DefaultBackoff,
		func(error) bool {
			// not sure what not to retry on
			return true
		}, func() error {
			var requestErr error
			r, requestErr = http.DefaultClient.Do(req.WithContext(ctx))
			defer func() {
				_ = r.Body.Close()
			}()
			if requestErr != nil {
				return requestErr
			}
			body, requestErr = ioutil.ReadAll(r.Body)
			if requestErr != nil {
				return requestErr
			}
			if r.StatusCode == http.StatusInternalServerError ||
				r.StatusCode == http.StatusServiceUnavailable {
				requestErr = fmt.Errorf("internal server error, status code = %d", r.StatusCode)
			}
			return requestErr
		})

	// failed even with retry
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return body, r.StatusCode, nil
}

// callWebhook does a HTTP POST to an external service and
// returns an error if the response status code is non-2xx
func callWebhook(ctx context.Context, resource klog.KMetadata, phase v1alpha1.RollingState,
	w v1alpha1.RolloutWebhook) error {
	payload := v1alpha1.RolloutWebhookPayload{
		Name:      resource.GetName(),
		Namespace: resource.GetNamespace(),
		Phase:     phase,
	}

	if w.Metadata != nil {
		payload.Metadata = *w.Metadata
	}
	// make the http request
	_, status, err := makeHTTPRequest(ctx, w.URL, payload)
	if err != nil {
		return err
	}
	if len(w.ExpectedStatus) == 0 {
		if status > http.StatusAccepted {
			err := fmt.Errorf("we fail the webhook request based on status, http status = %d", status)
			return err
		}
		return nil
	}
	// check if the returned status is expected
	accepted := false
	for _, es := range w.ExpectedStatus {
		if es == status {
			accepted = true
			break
		}
	}
	if !accepted {
		err := fmt.Errorf("http request to the webhook not accepeted, http status = %d", status)
		klog.V(common.LogDebug).InfoS("the status is not expected", "expected status", w.ExpectedStatus)
		return err
	}
	return nil
}
