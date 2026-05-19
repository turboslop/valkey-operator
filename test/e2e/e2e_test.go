/*
Copyright 2024.

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

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hyperspike.io/valkey-operator/test/utils"
)

const (
	operatorNamespace = "valkey-operator-system"
	valkeyNamespace   = "valkey-e2e"
	valkeyName        = "ci-valkey"
	smokeJobName      = "valkey-smoke"
)

var _ = Describe("controller", Ordered, func() {
	var controllerImage, sidecarImage, valkeyImage string

	BeforeAll(func() {
		controllerImage = envOrDefault("IMG_CONTROLLER", "localhost/valkey-operator:e2e")
		sidecarImage = envOrDefault("IMG_SIDECAR", "localhost/valkey-sidecar:e2e")
		valkeyImage = envOrDefault("IMG_VALKEY", "localhost/valkey:e2e")
	})

	AfterAll(func() {
		By("removing the e2e Valkey namespace")
		warnIfFails("kubectl", "delete", "ns", valkeyNamespace, "--ignore-not-found=true", "--timeout=2m")

		By("undeploying the controller-manager")
		warnIfFails("make", "undeploy", "ignore-not-found=true")
	})

	Context("Valkey", func() {
		It("should deploy a cluster and serve commands", func() {
			By("building the controller, sidecar, and Valkey images")
			cmd := exec.Command("make", "docker-build",
				fmt.Sprintf("IMG_CONTROLLER=%s", controllerImage),
				fmt.Sprintf("IMG_SIDECAR=%s", sidecarImage),
				fmt.Sprintf("IMG_VALKEY=%s", valkeyImage),
			)
			_, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			for _, image := range []string{controllerImage, sidecarImage, valkeyImage} {
				By("loading image into the e2e cluster: " + image)
				ExpectWithOffset(1, utils.LoadImageToClusterWithName(image)).To(Succeed())
			}

			By("deploying the controller-manager")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG_CONTROLLER=%s", controllerImage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("waiting for the controller-manager rollout")
			cmd = exec.Command("kubectl", "-n", operatorNamespace,
				"rollout", "status", "deploy/valkey-operator-controller-manager", "--timeout=2m")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("waiting for the Valkey CRD")
			cmd = exec.Command("kubectl",
				"wait", "--for=condition=Established", "crd/valkeys.hyperspike.io", "--timeout=2m")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("creating the e2e namespace")
			ExpectWithOffset(1, applyManifest("", fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, valkeyNamespace))).To(Succeed())

			By("creating a Valkey custom resource")
			ExpectWithOffset(1, applyManifest(valkeyNamespace, fmt.Sprintf(`apiVersion: hyperspike.io/v1
kind: Valkey
metadata:
  name: %s
spec:
  shards: 3
  replicas: 0
  image: %q
  exporterImage: %q
  volumePermissions: true
`, valkeyName, valkeyImage, sidecarImage))).To(Succeed())

			By("waiting for the Valkey StatefulSet")
			cmd = exec.Command("kubectl", "-n", valkeyNamespace,
				"wait", "--for=create", fmt.Sprintf("sts/%s", valkeyName), "--timeout=2m")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "-n", valkeyNamespace,
				"rollout", "status", fmt.Sprintf("sts/%s", valkeyName), "--timeout=8m")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("waiting for the operator to mark Valkey ready")
			cmd = exec.Command("kubectl", "-n", valkeyNamespace,
				"wait", "--for=jsonpath={.status.ready}=true", fmt.Sprintf("valkey/%s", valkeyName), "--timeout=8m")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("running an in-cluster valkey-cli smoke test")
			ExpectWithOffset(1, applyManifest(valkeyNamespace, fmt.Sprintf(`apiVersion: batch/v1
kind: Job
metadata:
  name: %s
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 60
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smoke
        image: %q
        env:
        - name: REDISCLI_AUTH
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
        command:
        - sh
        - -ec
        - |
          valkey-cli -h %s -p 6379 ping | grep PONG
          valkey-cli -c -h %s -p 6379 set ci-smoke ok
          test "$(valkey-cli -c -h %s -p 6379 get ci-smoke)" = "ok"
          valkey-cli -h %s -p 6379 cluster info | grep "cluster_state:ok"
`, smokeJobName, valkeyImage, valkeyName, valkeyName, valkeyName, valkeyName, valkeyName))).To(Succeed())

			cmd = exec.Command("kubectl", "-n", valkeyNamespace,
				"wait", "--for=condition=complete", fmt.Sprintf("job/%s", smokeJobName), "--timeout=2m")
			_, err = utils.Run(cmd)
			if err != nil {
				dumpCommand("kubectl", "-n", valkeyNamespace, "logs", fmt.Sprintf("job/%s", smokeJobName))
			}
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
		})
	})
})

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func applyManifest(namespace, manifest string) error {
	args := []string{}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "apply", "-f", "-")
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = strings.NewReader(manifest)
	_, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("apply manifest: %w", err)
	}
	return nil
}

func warnIfFails(name string, args ...string) {
	cmd := exec.Command(name, args...)
	output, err := utils.Run(cmd)
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n%s\n", err, string(output))
	}
}

func dumpCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	output, err := utils.Run(cmd)
	if len(output) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "%s\n", string(output))
	}
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
	}
}
