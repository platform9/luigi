package controllers

import (
	"context"
	"testing"

	plumberv1 "github.com/platform9/luigi/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Regression test for PMK-7006: the decoder must be set without relying on
// InjectDecoder, which controller-runtime >= 0.15 no longer calls.
func TestNetworkPluginsValidatorCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := plumberv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	v := NewNetworkPluginsValidator(
		fake.NewClientBuilder().WithScheme(scheme).Build(),
		admission.NewDecoder(scheme),
	)
	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{Raw: []byte(
			`{"apiVersion":"plumber.k8s.pf9.io/v1","kind":"NetworkPlugins","metadata":{"name":"default"}}`)},
	}}

	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Fatalf("expected create to be allowed, got: %+v", resp.Result)
	}
}
