/*
Copyright 2026 Agentic Layer.

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

package litellm

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSecretHash_MissingSecretReturnsEmptyHash(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	c := fake.NewClientBuilder().WithScheme(s).Build()

	got, err := computeSecretHash(context.Background(), c, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == "" || len(got) != 16 {
		t.Errorf("expected 16-char hash, got %q", got)
	}
}

func TestSecretHash_DeterministicForSameData(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ApiKeySecretName, Namespace: "default"},
		Data:       map[string][]byte{"OPENAI_API_KEY": []byte("sk-123")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()

	first, err := computeSecretHash(context.Background(), c, "default")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := computeSecretHash(context.Background(), c, "default")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Errorf("hashes differ for identical data: %q vs %q", first, second)
	}
}

func TestSecretHash_ChangesWhenDataChanges(t *testing.T) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	build := func(data map[string][]byte) string {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: ApiKeySecretName, Namespace: "default"},
			Data:       data,
		}
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
		got, err := computeSecretHash(context.Background(), c, "default")
		if err != nil {
			t.Fatalf("computeSecretHash: %v", err)
		}
		return got
	}
	a := build(map[string][]byte{"K": []byte("v1")})
	b := build(map[string][]byte{"K": []byte("v2")})
	if a == b {
		t.Errorf("hash should change when data changes; got identical %q", a)
	}
}
