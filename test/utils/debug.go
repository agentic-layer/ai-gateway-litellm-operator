/*
Copyright 2026.

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

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
)

// systemNamespacePrefixes are skipped when auto-collecting diagnostics.
var systemNamespacePrefixes = []string{"kube-", "cert-manager", "local-path-storage", "gmp-"}

// interestingKinds are dumped cluster-wide to capture operator-managed state.
var interestingKinds = []string{
	"aigateway",
	"aigatewayclass",
	"toolgateway",
	"toolserver",
	"toolroute",
	"toolgatewayclass",
	"guard",
	"guardrailprovider",
}

// newDebugDir creates a fresh timestamped directory under $TMPDIR to collect
// failure artifacts for a single test. Returns the absolute path.
func newDebugDir() (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(os.TempDir(), ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create debug dir %s: %w", dir, err)
	}
	return dir, nil
}

// kubectlToFile runs `kubectl args...` and writes combined output to <dir>/<filename>.
func kubectlToFile(dir, filename string, args ...string) {
	path := filepath.Join(dir, filename)
	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		out = append(out, []byte(fmt.Sprintf("\n\n[kubectl error: %v]\n", err))...)
	}
	_ = os.WriteFile(path, out, 0o644)
}

// kubectlToGinkgo runs `kubectl args...` and writes the combined output to
// GinkgoWriter under the given header.
func kubectlToGinkgo(header string, args ...string) {
	out, err := exec.Command("kubectl", args...).CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "%s [error: %v]:\n%s\n", header, err, out)
		return
	}
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "%s:\n%s\n", header, out)
}

// listUserNamespaces returns all cluster namespaces minus the ones matching
// systemNamespacePrefixes or systemNamespaces.
func listUserNamespaces() []string {
	out, err := exec.Command("kubectl", "get", "ns", "-o", "name").CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "failed to list namespaces: %v\n%s\n", err, out)
		return nil
	}
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		ns := strings.TrimPrefix(line, "namespace/")
		if ns == "" {
			continue
		}
		skip := false
		for _, p := range systemNamespacePrefixes {
			if strings.HasPrefix(ns, p) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		result = append(result, ns)
	}
	return result
}

// listPods returns pod names in the given namespace (without the "pod/" prefix).
func listPods(namespace string) []string {
	out, err := exec.Command("kubectl", "get", "pods", "-n", namespace, "-o", "name").CombinedOutput()
	if err != nil {
		return nil
	}
	var pods []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimPrefix(line, "pod/")
		if name != "" {
			pods = append(pods, name)
		}
	}
	return pods
}

// CollectDiagnostics writes full pod logs and a cluster-wide dump of interesting
// custom resources to a timestamped directory under $TMPDIR. It also prints
// cluster-wide events and the last lines of each pod's logs to GinkgoWriter.
// User namespaces are enumerated automatically (skipping kube-* and cert-manager).
// The directory path is printed to GinkgoWriter and returned.
//
// Intended for use from AfterEach on test failure.
func CollectDiagnostics() string {
	dir, err := newDebugDir()
	if err != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create debug dir: %v\n", err)
		return ""
	}

	// Cluster-wide pod overview to file.
	kubectlToFile(dir, "pods_all.txt", "get", "pods", "-A", "-o", "wide")
	kubectlToGinkgo("\nAll pods:", "get", "pods", "-A", "-o", "wide")

	// Cluster-wide events, time-ordered.
	kubectlToGinkgo("Events (all namespaces)",
		"get", "events", "-A", "--sort-by=.lastTimestamp")

	// Per-namespace pod logs (full to file, last 10 lines to ginkgo).
	for _, ns := range listUserNamespaces() {
		for _, pod := range listPods(ns) {
			kubectlToFile(dir, fmt.Sprintf("logs_%s_%s.log", ns, pod),
				"logs", "-n", ns, pod, "--all-containers=true", "--prefix=true")
			kubectlToGinkgo(fmt.Sprintf("Last 10 log lines for %s/%s", ns, pod),
				"logs", "-n", ns, pod, "--all-containers=true", "--prefix=true", "--tail=10")
		}
	}

	// Cluster-wide dump of interesting kinds to file (best effort; some may not exist).
	for _, kind := range interestingKinds {
		safe := strings.ReplaceAll(kind, ".", "_")
		kubectlToFile(dir, fmt.Sprintf("%s.yaml", safe),
			"get", kind, "-A", "-o", "yaml")
	}

	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Debug bundle: %s\n", dir)

	return dir
}
