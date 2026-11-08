package controllers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	typedCore "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

type recordedRequest struct {
	method      string
	path        string
	contentType string
	body        []byte
}

func newNodeAPIServer(t *testing.T, patchStatus int) (*typedCore.CoreV1Client, func() []recordedRequest) {
	t.Helper()

	var mu sync.Mutex
	var reqs []recordedRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, recordedRequest{r.Method, r.URL.Path, r.Header.Get("Content-Type"), body})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPatch && patchStatus == http.StatusOK:
			_ = json.NewEncoder(w).Encode(&corev1.Node{
				TypeMeta:   metav1.TypeMeta{Kind: "Node", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"manufacturer": "dell"}},
			})
		case r.Method == http.MethodPatch:
			w.WriteHeader(patchStatus)
			_ = json.NewEncoder(w).Encode(&metav1.Status{
				TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
				Status:   metav1.StatusFailure,
				Code:     int32(patchStatus),
				Reason:   metav1.StatusReasonNotFound,
			})
		default:
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(&metav1.Status{
				TypeMeta: metav1.TypeMeta{Kind: "Status", APIVersion: "v1"},
				Status:   metav1.StatusFailure,
				Code:     http.StatusConflict,
				Reason:   metav1.StatusReasonConflict,
			})
		}
	}))
	t.Cleanup(srv.Close)

	c, err := typedCore.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	return c, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), reqs...)
	}
}

func TestPatchNodeLabelsUsesMergePatch(t *testing.T) {
	c, got := newNodeAPIServer(t, http.StatusOK)

	labels := map[string]string{"manufacturer": "dell", "model": "r640"}
	node, err := patchNodeLabels(context.Background(), c, "node1", labels)
	if err != nil {
		t.Fatalf("expected patch to succeed against a node being written by others, got: %v", err)
	}
	if node.Name != "node1" {
		t.Fatalf("expected returned node node1, got %q", node.Name)
	}

	reqs := got()
	if len(reqs) != 1 {
		t.Fatalf("expected exactly one request, got %d: %+v", len(reqs), reqs)
	}
	r := reqs[0]
	if r.method != http.MethodPatch || r.path != "/api/v1/nodes/node1" {
		t.Fatalf("expected PATCH /api/v1/nodes/node1, got %s %s", r.method, r.path)
	}
	if r.contentType != string(types.MergePatchType) {
		t.Fatalf("expected content type %s, got %s", types.MergePatchType, r.contentType)
	}

	// the body must hold only metadata.labels: no resourceVersion, no spec/status
	var body map[string]map[string]any
	if err := json.Unmarshal(r.body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || len(body["metadata"]) != 1 {
		t.Fatalf("expected body with only metadata.labels, got %s", r.body)
	}
	sent, _ := body["metadata"]["labels"].(map[string]any)
	if len(sent) != len(labels) {
		t.Fatalf("expected labels %v, got %s", labels, r.body)
	}
	for k, v := range labels {
		if sent[k] != v {
			t.Fatalf("expected label %s=%s, got %s", k, v, r.body)
		}
	}
}

func TestPatchNodeLabelsNotFound(t *testing.T) {
	c, _ := newNodeAPIServer(t, http.StatusNotFound)

	_, err := patchNodeLabels(context.Background(), c, "node1", map[string]string{"a": "b"})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound to propagate so the caller can skip the node, got: %v", err)
	}
}
