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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"hyperspike.io/valkey-operator/test/utils"
)

const (
	operatorNamespace  = "valkey-operator-system"
	valkeyNamespace    = "valkey-e2e"
	valkeyName         = "ci-valkey"
	smokeJobName       = "valkey-smoke"
	defaultValkeyImage = "valkey/valkey-bundle:9.1-rc2"
	kubectlDumpTimeout = "10s"
)

var _ = Describe("controller", Ordered, func() {
	var controllerImage, sidecarImage, valkeyImage string
	var controllerDeployed, namespaceCreated bool

	BeforeAll(func() {
		controllerImage = envOrDefault("IMG_CONTROLLER", "localhost/valkey-operator:e2e")
		sidecarImage = envOrDefault("IMG_SIDECAR", "localhost/valkey-sidecar:e2e")
		valkeyImage = envOrDefault("IMG_VALKEY", defaultValkeyImage)
		_, _ = fmt.Fprintf(GinkgoWriter, "e2e images: controller=%s sidecar=%s valkey=%s\n",
			controllerImage, sidecarImage, valkeyImage)
		_, _ = fmt.Fprintf(GinkgoWriter, "e2e cluster runtime: %s\n",
			envOrDefault("E2E_CLUSTER_RUNTIME", "kind"))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			By("dumping e2e diagnostics")
			dumpE2EDiagnostics()
		}
	})

	AfterAll(func() {
		if namespaceCreated {
			By("removing the e2e Valkey namespace")
			warnIfFails("kubectl", "--request-timeout="+kubectlDumpTimeout,
				"delete", "ns", valkeyNamespace, "--ignore-not-found=true", "--timeout=2m")
		}

		if controllerDeployed {
			By("undeploying the controller-manager")
			warnIfFails("make", "undeploy", "ignore-not-found=true")
		}
	})

	Context("Valkey", func() {
		It("should deploy a cluster and serve commands", func() {
			By("building the controller and sidecar images")
			cmd := exec.Command("make", "docker-build-manager", "docker-build-sidecar",
				fmt.Sprintf("IMG_CONTROLLER=%s", controllerImage),
				fmt.Sprintf("IMG_SIDECAR=%s", sidecarImage),
			)
			_, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())

			By("ensuring the Valkey bundle image is available")
			ExpectWithOffset(1, ensureDockerImage(valkeyImage)).To(Succeed())

			for _, image := range []string{controllerImage, sidecarImage, valkeyImage} {
				By("loading image into the e2e cluster: " + image)
				ExpectWithOffset(1, utils.LoadImageToClusterWithName(image)).To(Succeed())
			}

			By("deploying the controller-manager")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG_CONTROLLER=%s", controllerImage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			controllerDeployed = true

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
			namespaceCreated = true

			By("creating a Valkey custom resource")
			ExpectWithOffset(1, applyManifest(valkeyNamespace, fmt.Sprintf(`apiVersion: hyperspike.io/v1
kind: Valkey
metadata:
  name: %s
spec:
  shards: 2
  replicas: 2
  image: %q
  exporterImage: %q
  volumePermissions: true
  modules:
  - path: /usr/lib/valkey/libjson.so
  - path: /usr/lib/valkey/libvalkey_bloom.so
  - path: /usr/lib/valkey/libsearch.so
    args:
    - --use-coordinator
  extraConfig: |
    latency-monitor-threshold 1
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
  activeDeadlineSeconds: 240
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: smoke
        image: %q
        env:
        - name: VALKEYCLI_AUTH
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
        - name: REDISCLI_AUTH
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
        command:
        - sh
        - -ec
        - |
          set -eux

          step() {
            printf '\n### %%s\n' "$*"
          }

          cli() {
            valkey-cli -t 5 "$@"
          }

          host=%s
          headless=%s-headless
          step ping
          cli -h "$host" -p 6379 ping | grep PONG
          step key-value
          cli -c -h "$host" -p 6379 set ci-smoke ok
          test "$(cli -c -h "$host" -p 6379 get ci-smoke)" = "ok"
          step cluster-info
          cli -h "$host" -p 6379 cluster info | grep "cluster_state:ok"
          step roles
          roles="$(for i in $(seq 0 5); do cli -h "$host-$i.$headless" -p 6379 info replication | tr -d '\r' | sed -n 's/^role://p'; done)"
          printf 'roles:\n%%s\n' "$roles"
          test "$(printf '%%s\n' "$roles" | grep -c '^master$')" = "2"
          test "$(printf '%%s\n' "$roles" | grep -Ec '^(slave|replica)$')" = "4"

          step modules
          set +x
          modules="$(cli -h "$host" -p 6379 info modules | tr -d '\r')"
          module_names="$(printf '%%s\n' "$modules" | sed -n 's/^module:name=\([^,]*\).*/\1/p')"
          coordinator_port="$(printf '%%s\n' "$modules" | sed -n 's/^search_coordinator_server_listening_port://p')"
          set -x
          printf 'module names:\n%%s\n' "$module_names"
          printf 'search coordinator port: %%s\n' "$coordinator_port"
          printf '%%s\n' "$module_names" | grep "^json$"
          printf '%%s\n' "$module_names" | grep "^bf$"
          printf '%%s\n' "$module_names" | grep "^search$"
          test -n "$coordinator_port"
          cli -h "$host" -p 6379 config get latency-monitor-threshold | tr -d '\r' | grep -E '^1$'

          step json
          cli -c -h "$host" -p 6379 JSON.SET json:ci '$' '{"hello":"world"}' | grep OK
          cli -c -h "$host" -p 6379 JSON.GET json:ci '$.hello' | grep world
          step bloom
          cli -c -h "$host" -p 6379 BF.RESERVE bf:ci 0.01 100 | grep OK
          test "$(cli -c -h "$host" -p 6379 BF.ADD bf:ci item)" = "1"
          test "$(cli -c -h "$host" -p 6379 BF.EXISTS bf:ci item)" = "1"

          step search
          created=""
          for _ in $(seq 1 30); do
            create_result="$(cli -c -h "$host" -p 6379 FT.CREATE idx:ci ON HASH PREFIX 1 doc: SCHEMA title TEXT 2>&1 || true)"
            printf 'create index result:\n%%s\n' "$create_result"
            if printf '%%s\n' "$create_result" | grep -E '^(OK|Index already exists)$'; then
              created=yes
              break
            fi
            sleep 2
          done
          test "$created" = "yes"
          cli -c -h "$host" -p 6379 HSET doc:1 title "hello valkey search"
          found=""
          for _ in $(seq 1 30); do
            result="$(cli -c -h "$host" -p 6379 FT.SEARCH idx:ci '@title:valkey' NOCONTENT || true)"
            printf 'search result:\n%%s\n' "$result"
            if printf '%%s\n' "$result" | grep "doc:1"; then
              found=yes
              break
            fi
            sleep 2
          done
          test "$found" = "yes"
`, smokeJobName, valkeyImage, valkeyName, valkeyName, valkeyName, valkeyName))).To(Succeed())

			By("waiting for the smoke job to complete")
			err = waitForJobComplete(valkeyNamespace, smokeJobName, 4*time.Minute)
			if err != nil {
				dumpSmokeJob()
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

func ensureDockerImage(image string) error {
	cmd := exec.Command("docker", "image", "inspect", "--format={{.Id}}", image)
	if _, err := utils.Run(cmd); err == nil {
		return nil
	}

	cmd = exec.Command("docker", "pull", image)
	_, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	return nil
}

func applyManifest(namespace, manifest string) error {
	if namespace == "" {
		_, _ = fmt.Fprintf(GinkgoWriter, "applying cluster-scoped manifest:\n%s\n", manifest)
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "applying manifest in namespace %s:\n%s\n", namespace, manifest)
	}
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

func waitForJobComplete(namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		cmd := exec.Command("kubectl", "-n", namespace,
			"get", fmt.Sprintf("job/%s", name), "-o", "jsonpath={.status.conditions[*].type}")
		output, err := utils.Run(cmd)
		if err != nil {
			return fmt.Errorf("get job status: %w", err)
		}

		for _, condition := range strings.Fields(string(output)) {
			switch condition {
			case "Complete":
				return nil
			case "Failed":
				return fmt.Errorf("job %s/%s failed", namespace, name)
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for job %s/%s to complete", namespace, name)
		}
		time.Sleep(2 * time.Second)
	}
}

func dumpE2EDiagnostics() {
	dumpKubectl("config", "current-context")
	if !dumpKubectl("cluster-info") {
		return
	}
	dumpKubectl("-n", operatorNamespace, "get", "pods,deploy,rs,cm,events", "-o", "wide")
	dumpKubectl("-n", operatorNamespace, "describe", "deploy/valkey-operator-controller-manager")
	dumpKubectl("-n", operatorNamespace, "logs", "deploy/valkey-operator-controller-manager", "--all-containers", "--tail=100")

	dumpKubectl("-n", valkeyNamespace, "get", "valkey,sts,pods,svc,cm,secret,job,events", "-o", "wide")
	dumpKubectl("-n", valkeyNamespace, "describe", fmt.Sprintf("valkey/%s", valkeyName))
	dumpKubectl("-n", valkeyNamespace, "describe", fmt.Sprintf("sts/%s", valkeyName))
	dumpKubectl("-n", valkeyNamespace, "describe", "pods")
	dumpSmokeJob()
	dumpKubectl("-n", valkeyNamespace, "logs", "-l", fmt.Sprintf("app.kubernetes.io/instance=%s", valkeyName), "--all-containers", "--tail=30", "--prefix=true")
}

func dumpSmokeJob() {
	dumpKubectl("-n", valkeyNamespace, "logs", "-l", fmt.Sprintf("job-name=%s", smokeJobName), "--all-containers", "--tail=-1", "--prefix=true")
	dumpKubectl("-n", valkeyNamespace, "get", fmt.Sprintf("job/%s", smokeJobName), "-o", "wide")
	dumpKubectl("-n", valkeyNamespace, "describe", fmt.Sprintf("job/%s", smokeJobName))
	dumpKubectl("-n", valkeyNamespace, "get", "pods", "-l", fmt.Sprintf("job-name=%s", smokeJobName), "-o", "wide")
	dumpKubectl("-n", valkeyNamespace, "describe", "pods", "-l", fmt.Sprintf("job-name=%s", smokeJobName))
}

func warnIfFails(name string, args ...string) {
	cmd := exec.Command(name, args...)
	_, err := utils.Run(cmd)
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
	}
}

func dumpKubectl(args ...string) bool {
	kubectlArgs := append([]string{"--request-timeout=" + kubectlDumpTimeout}, args...)
	cmd := exec.Command("kubectl", kubectlArgs...)
	_, err := utils.Run(cmd)
	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
		return false
	}
	return true
}
