package utils

import (
	"net/http"

	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/klog/v2"
)

// TryCancelRequest tries to cancel the request by traversing round trippers
func TryCancelRequest(rt http.RoundTripper, req *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	switch rt := rt.(type) {
	case canceler:
		rt.CancelRequest(req)
	case utilnet.RoundTripperWrapper:
		TryCancelRequest(rt.WrappedRoundTripper(), req)
	default:
		klog.Warningf("Unable to cancel request for %T", rt)
	}
}
