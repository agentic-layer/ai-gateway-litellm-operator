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

package utils

import (
	"fmt"
	"os/exec"
	"time"
)

// VerifyDeploymentReady verifies that a deployment is ready within the given timeout
func VerifyDeploymentReady(name, namespace string, timeout time.Duration) error {
	cmd := exec.Command("kubectl", "wait", "deployment", name, "-n", namespace,
		"--for=condition=Available", "--timeout="+timeout.String())
	if output, err := Run(cmd); err != nil {
		describeDeployment, _ := Run(exec.Command("kubectl", "describe", "deployment", name, "-n", namespace))
		describePods, _ := Run(exec.Command("kubectl", "describe", "pod", "-l", "app="+name, "-n", namespace))
		return fmt.Errorf("deployment is not ready (%s):\n%s\nPods:\n%s",
			output, describeDeployment, describePods,
		)
	}
	return nil
}

// WaitForAllDeploymentsReady waits for every Deployment in every namespace to
// reach condition=Available within the given timeout.
func WaitForAllDeploymentsReady(timeout time.Duration) error {
	cmd := exec.Command("kubectl", "wait", "deployment", "--all", "-A",
		"--for=condition=Available", "--timeout="+timeout.String())
	if output, err := Run(cmd); err != nil {
		return fmt.Errorf("not all deployments became ready: %s", output)
	}
	return nil
}
