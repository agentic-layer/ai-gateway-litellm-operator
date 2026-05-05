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
	"crypto/sha256"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// computeSecretHash returns a deterministic short hash of the api-key secret in
// the given namespace. If the secret does not exist, returns the hash of an
// empty payload — this is intentional so the deployment can still be created
// before the secret is set up. Errors are returned only for non-NotFound errors.
func computeSecretHash(ctx context.Context, c client.Reader, namespace string) (string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Name: ApiKeySecretName, Namespace: namespace}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			empty := sha256.Sum256(nil)
			return fmt.Sprintf("%x", empty)[:16], nil
		}
		return "", fmt.Errorf("failed to get secret %s: %w", ApiKeySecretName, err)
	}

	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(secret.Data[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}
