// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache-2.0
package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/open-edge-platform/cluster-manager/v2/pkg/api"
	"github.com/open-edge-platform/cluster-manager/v2/test/helpers"
)

func TestService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Suite")
}

const cmAddress = "localhost:8080"
const ctRelativePath = "../../default-cluster-templates/baseline-k3s.json"
const hostIdAnnotationKey = "intelmachine.infrastructure.cluster.x-k8s.io/host-id"

var testTenantID uuid.UUID
var hostIdAnnotationVal string

// deleteCluster is a flag that determines if the cluster should be deleted after the tests
// use DELETE_CLUSTER=false to keep the cluster after the tests
var deleteCluster bool = true

func init() {
	testTenantID = uuid.New()
	hostIdAnnotationVal = fmt.Sprintf("host-%v", rand.Intn(10000))
	if os.Getenv("DELETE_CLUSTER") == "false" {
		deleteCluster = false
	}
}

// we need to simulate multi-tenancy, as it is disabled in service tests because it requires a real tenant service to work
var _ = BeforeSuite(func() {
	// Generate test keys if auth is enabled
	if os.Getenv("DISABLE_AUTH") == "false" {
		fmt.Println("auth is enabled - using remote token server")
		os.Setenv("USE_REMOTE_TOKEN_SERVER", "1")

		fmt.Println("waiting for mock Keycloak to be ready")
		Eventually(func() error {
			_, err := getTokenFromClusterKeycloak()
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed())

		// Mock Vault is deployed in orch-platform namespace via Makefile
		fmt.Println("Mock Vault deployed - cluster-manager will use it for M2M credentials")
	}

	// create the namespace for the tenant
	fmt.Println("Creating namespace for tenant", testTenantID.String())
	cmd := exec.Command("kubectl", "create", "namespace", testTenantID.String())
	err := cmd.Run()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("Created namespace for tenant", testTenantID.String())

	// create the psa secret for the tenant
	fmt.Println("Creating pod security admission config secret for tenant", testTenantID.String())
	cmd = exec.Command("kubectl", "-n", testTenantID.String(), "apply", "-f", "../../deployment/charts/cluster-manager/templates/secret.yaml")
	err = cmd.Run()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("Created pod security admission config secret for tenant", testTenantID.String())

	// create the baseline cluster template in the tenant namespace
	filePath, err := resolvePath(ctRelativePath)
	Expect(err).ToNot(HaveOccurred())
	template, err := loadTemplateInfoFromFile(filePath)
	infraProviderr := api.TemplateInfoInfraprovidertype("docker")
	template.Infraprovidertype = &infraProviderr
	Expect(err).ToNot(HaveOccurred())

	cli, err := createAuthenticatedClient()
	Expect(err).ToNot(HaveOccurred())

	params := api.PostV2TemplatesParams{}
	params.Activeprojectid = testTenantID
	resp, err := cli.PostV2TemplatesWithResponse(context.Background(), &params, template)
	Expect(err).ToNot(HaveOccurred())
	if resp.StatusCode() != 201 {
		fmt.Printf("ERROR: Expected 201, got %d. Response body: %s\n", resp.StatusCode(), string(resp.Body))
	}
	Expect(resp.StatusCode()).To(Equal(201))
	fmt.Println("Created baseline template for tenant", testTenantID.String())

	// label the baseline template with the default=true
	templateName := fmt.Sprintf("%s-%v", template.Name, template.Version)
	fmt.Printf("Labeling %v template with default=true for tenant %v", templateName, testTenantID.String())
	cmd = exec.Command("kubectl", "-n", testTenantID.String(), "label", "clustertemplate", templateName, "default=true")
	err = cmd.Run()
	Expect(err).ToNot(HaveOccurred())
	fmt.Println("Labeled baseline template with default=true for tenant", testTenantID.String())
})

var _ = AfterSuite(func() {
	if deleteCluster {
		// delete the namespace for the tenant
		cmd := exec.Command("kubectl", "delete", "namespace", testTenantID.String())
		err := cmd.Run()
		Expect(err).ToNot(HaveOccurred())
		fmt.Println("Deleted namespace for tenant", testTenantID.String())
	}

	// clean up env vars
	if os.Getenv("DISABLE_AUTH") == "false" {
		os.Unsetenv("USE_REMOTE_TOKEN_SERVER")
	}
})

// Cluster create/delete flow test
var _ = Describe("Cluster create/delete flow", Ordered, func() {
	cli, err := createAuthenticatedClient()
	Expect(err).ToNot(HaveOccurred())

	Context("CM is deployed to Kubernetes cluster and is starting up", func() {
		It("Should return 200 on /v2/healthz", func() {
			// it takes some time for the service to start up so we retry the request until it succeeds with a 10 second timeout
			Eventually(func() (int, error) {
				resp, err := cli.GetV2Healthz(context.Background())
				if err != nil {
					return 0, err
				}
				return resp.StatusCode, nil
			}, 10*time.Second).Should(Equal(200))
		})
	})

	Context("CM is ready to serve API requests", func() {
		var clusterName = "test-cluster"
		var templateName string
		var templateOnlyName string
		var templateOnlyVersion string

		It("Should return 200 and list of available templates", func() {
			params := api.GetV2TemplatesParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.GetV2TemplatesWithResponse(context.Background(), &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(*resp.JSON200.TotalElements).To(Equal(int32(1)))
			Expect(*resp.JSON200.TemplateInfoList).To(HaveLen(1))
			templateInfo := resp.JSON200.DefaultTemplateInfo
			templateName = fmt.Sprintf("%s-%v", *templateInfo.Name, templateInfo.Version)
			templateOnlyName = *templateInfo.Name
			templateOnlyVersion = templateInfo.Version
		})

		It("Should return 200 and empty list of clusters on /v2/clusters", func() {
			params := api.GetV2ClustersParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.GetV2ClustersWithResponse(context.Background(), &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(resp.JSON200.TotalElements).To(Equal(int32(0)))
			Expect(*resp.JSON200.Clusters).To(HaveLen(0))
		})

		It("Should create a new cluster", func() {
			params := api.PostV2ClustersParams{}
			params.Activeprojectid = testTenantID
			body := api.PostV2ClustersJSONRequestBody{
				Name:     &clusterName,
				Template: &templateName,
				Nodes:    []api.NodeSpec{{Id: "1", Role: "all"}},
				Labels:   &map[string]string{},
			}

			// it takes some time for template to be created in the tenant namespace so we retry the request until it succeeds with a 10 second timeout
			Eventually(func() (int, error) {
				resp, err := cli.PostV2ClustersWithResponse(context.Background(), &params, body)
				if err != nil {
					return 0, err
				}
				return resp.StatusCode(), nil
			}, 30*time.Second, 3*time.Second).Should(Equal(201))

			// Unpause cluster to simulate cluster-agent behaviour
			patchData := []byte(`{"spec":{"paused":false}}`)
			cmd := exec.Command("kubectl", "-n", testTenantID.String(), "patch", "cl", clusterName, "--type=merge", "-p", string(patchData))
			err := cmd.Run()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should return 200 and a list of clusters with one element on /v2/clusters", func() {
			params := api.GetV2ClustersParams{}
			params.Activeprojectid = testTenantID

			// it takes some time for the cluster to be created so we retry the request until it succeeds with a 30 second timeout
			var resp *api.GetV2ClustersResponse
			var err error
			Eventually(func() (bool, error) {
				resp, err = cli.GetV2ClustersWithResponse(context.Background(), &params)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					return false, err
				}
				if resp.StatusCode() != 200 {
					fmt.Printf("unexpected status code: %d\n", resp.StatusCode())
					return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
				}
				if resp.JSON200.TotalElements != 1 {
					fmt.Printf("unexpected number of clusters: %d\n", resp.JSON200.TotalElements)
					return false, fmt.Errorf("unexpected number of clusters: %d", resp.JSON200.TotalElements)
				}
				if *(*resp.JSON200.Clusters)[0].NodeQuantity != 1 {
					return false, fmt.Errorf("unexpected number of nodes: %d", *(*resp.JSON200.Clusters)[0].NodeQuantity)
				}

				return true, nil
			}, 30*time.Second, 3*time.Second).Should(Equal(true))

			Expect(*resp.JSON200.Clusters).To(HaveLen(1))
			Expect(*(*resp.JSON200.Clusters)[0].Name).To(Equal(clusterName))
			Expect(*(*resp.JSON200.Clusters)[0].NodeQuantity).To(Equal(1))
		})

		// Annotate the DockerMachines with the host-id label to simulate the intel-capi-provider behaviour
		It("Should annotate the DockerMachines with the host-id label", func() {
			err := annotateDockerMachines(testTenantID.String(), clusterName, fmt.Sprintf("%s=%s", hostIdAnnotationKey, hostIdAnnotationVal))
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should label the cluster with app=wordpress", func() {
			params := api.PutV2ClustersNameLabelsParams{}
			params.Activeprojectid = testTenantID
			body := api.PutV2ClustersNameLabelsJSONRequestBody{
				Labels: &map[string]string{"app": "wordpress", "default-extension": "baseline"},
			}
			resp, err := cli.PutV2ClustersNameLabelsWithResponse(context.Background(), clusterName, &params, body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
		})

		It("Should return 200 and the cluster details on /v2/clusters/{clusterID}", func() {
			params := api.GetV2ClustersNameParams{}
			params.Activeprojectid = testTenantID

			resp, err := cli.GetV2ClustersNameWithResponse(context.Background(), clusterName, &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(*resp.JSON200.Name).To(Equal(clusterName))
			Expect(*resp.JSON200.Labels).To(HaveLen(2))
			Expect(*resp.JSON200.Labels).To(HaveKeyWithValue("app", "wordpress"))
			Expect(*resp.JSON200.Labels).To(HaveKeyWithValue("default-extension", "baseline"))
			Expect(*resp.JSON200.Nodes).To(HaveLen(1))
			nodes := *resp.JSON200.Nodes
			Expect(*nodes[0].Role).To(Equal("all"))
			Expect(*nodes[0].Id).To(Equal(hostIdAnnotationVal))
			Expect(*nodes[0].Status.Condition).To(Equal(api.STATUSCONDITIONPROVISIONING))
			Expect(*nodes[0].Status.Reason).To(Equal("Provisioning"))
			Expect(*nodes[0].Status.Timestamp).ToNot(BeNil())
			Expect(*resp.JSON200.Template).To(Equal(templateName))

			err = containsLabels(testTenantID.String(), clusterName, []string{
				"app:wordpress",
				"default-extension:baseline",
				fmt.Sprintf("edge-orchestrator.intel.com/clustername:%v", clusterName),
				fmt.Sprintf("edge-orchestrator.intel.com/project-id:%v", testTenantID.String()),
			})
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should return 404 when downloading kubeconfig for non-existent cluster", func() {
			if os.Getenv("DISABLE_AUTH") == "true" {
				Skip("kubeconfig download requires auth for M2M token generation")
			}
			params := api.GetV2ClustersNameKubeconfigsParams{}
			params.Activeprojectid = testTenantID

			resp, err := cli.GetV2ClustersNameKubeconfigsWithResponse(context.Background(), "non-existent-cluster", &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(404))
			Expect(resp.JSON404).ToNot(BeNil())
			Expect(*resp.JSON404.Message).To(ContainSubstring("kubeconfig not found"))
		})

		It("Should return 200 and download kubeconfig successfully", func() {
			if os.Getenv("DISABLE_AUTH") == "true" {
				Skip("kubeconfig download requires auth for M2M token generation")
			}
			// create a mock kubeconfig secret for the cluster
			secretName := fmt.Sprintf("%s-kubeconfig", clusterName)
			// Use simple placeholder data (not real certificates) - following pattern from getv2clustersnamekubeconfig_test.go
			// Server URL must include /kubernetes/{namespace}-{clustername} for connect-gateway format
			mockKubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: test-ca-data
    server: http://edge-connect-gateway-cluster-connect-gateway.orch-cluster.svc:8080/kubernetes/%s-%s
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-cluster-admin
  name: test-cluster-admin@test-cluster
current-context: test-cluster-admin@test-cluster
users:
- name: test-cluster-admin
  user:
    client-certificate-data: test-client-cert
    client-key-data: test-client-key`, testTenantID.String(), clusterName)

			// Delete the secret if it exists (from previous test runs)
			deleteSecretCmd := exec.Command("kubectl", "delete", "secret", secretName,
				"-n", testTenantID.String(), "--ignore-not-found")
			_ = deleteSecretCmd.Run() // Ignore errors if secret doesn't exist

			// Write kubeconfig to a temporary file
			tmpFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
			Expect(err).ToNot(HaveOccurred())
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(mockKubeconfig)
			Expect(err).ToNot(HaveOccurred())
			tmpFile.Close()

			// Create the secret using kubectl with --from-file (not --from-literal)
			// The cluster-manager expects the secret to contain base64-encoded kubeconfig
			createSecretCmd := exec.Command("kubectl", "create", "secret", "generic", secretName,
				"-n", testTenantID.String(),
				"--from-file=value="+tmpFile.Name())
			err = createSecretCmd.Run()
			Expect(err).ToNot(HaveOccurred())

			// Download the kubeconfig
			params := api.GetV2ClustersNameKubeconfigsParams{}
			params.Activeprojectid = testTenantID

			resp, err := cli.GetV2ClustersNameKubeconfigsWithResponse(context.Background(), clusterName, &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(resp.JSON200).ToNot(BeNil())
			Expect(resp.JSON200.Kubeconfig).ToNot(BeNil())

			// Verify the kubeconfig content
			kubeconfig := *resp.JSON200.Kubeconfig
			Expect(kubeconfig).ToNot(BeEmpty())
			Expect(kubeconfig).To(ContainSubstring("apiVersion: v1"))
			Expect(kubeconfig).To(ContainSubstring("kind: Config"))
			Expect(kubeconfig).To(ContainSubstring("clusters:"))
			Expect(kubeconfig).To(ContainSubstring(clusterName))
		})

		It("Should generate metrics for initial operations", func() {
			resp, err := http.Get("http://" + cmAddress + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// check if the body contains the expected metrics that will be reset after pod restart
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"200\",method=\"GET\",path=\"/v2/templates\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"201\",method=\"POST\",path=\"/v2/clusters\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"201\",method=\"POST\",path=\"/v2/templates\"}"))
		})

		It("Should respect custom kubeconfig TTL when configured", func() {
			if os.Getenv("DISABLE_AUTH") == "true" {
				Skip("kubeconfig download requires auth for M2M token generation")
			}

			restartPF := func() {
				By("Restarting port-forwarding")
				_ = exec.Command("pkill", "-f", "kubectl port-forward svc/cluster-manager").Run()
				time.Sleep(1 * time.Second)
				pfCmd := exec.Command("kubectl", "port-forward", "svc/cluster-manager", "8080:8080")
				pfCmd.Stdout = nil
				pfCmd.Stderr = nil
				err := pfCmd.Start()
				Expect(err).ToNot(HaveOccurred(), "Failed to start port-forward")
				time.Sleep(3 * time.Second)
			}

			By("Patching cluster-manager deployment to set kubeconfig-ttl-hours=5 and speed up readiness probe")
			// Patch the deployment to add the argument and speed up readiness probe
			patchCmd := exec.Command("kubectl", "patch", "deployment", "cluster-manager", "-n", "default",
				"--type=json", "-p", `[
					{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubeconfig-ttl-hours=5"},
					{"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/initialDelaySeconds", "value": 1},
					{"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/periodSeconds", "value": 1}
				]`)
			output, err := patchCmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), "Failed to patch deployment: %s", string(output))

			By("Waiting for cluster-manager rollout")
			rolloutCmd := exec.Command("kubectl", "rollout", "status", "deployment/cluster-manager", "-n", "default", "--timeout=60s")
			output, err = rolloutCmd.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for rollout: %s", string(output))

			restartPF()

			// Ensure we revert the change even if the test fails
			defer func() {
				By("Reverting cluster-manager deployment changes")
				// Get current deployment
				out, err := exec.Command("kubectl", "get", "deployment", "cluster-manager", "-n", "default", "-o", "json").Output()
				if err != nil {
					fmt.Printf("Failed to get deployment: %v\n", err)
					return
				}

				var deploy map[string]interface{}
				if err := json.Unmarshal(out, &deploy); err != nil {
					fmt.Printf("Failed to unmarshal deployment: %v\n", err)
					return
				}

				// Navigate to args
				spec := deploy["spec"].(map[string]interface{})
				tmpl := spec["template"].(map[string]interface{})
				podSpec := tmpl["spec"].(map[string]interface{})
				containers := podSpec["containers"].([]interface{})
				container := containers[0].(map[string]interface{})
				args := container["args"].([]interface{})

				// Filter out the added arg
				newArgs := []string{}
				for _, a := range args {
					s := a.(string)
					if s != "--kubeconfig-ttl-hours=5" {
						newArgs = append(newArgs, s)
					}
				}

				// Patch back the args
				argsJson, _ := json.Marshal(newArgs)
				patch := fmt.Sprintf(`[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": %s}]`, string(argsJson))

				undoCmd := exec.Command("kubectl", "patch", "deployment", "cluster-manager", "-n", "default", "--type=json", "-p", patch)
				output, err := undoCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("Failed to revert args: %s\n", string(output))
				}

				// Wait for rollout to complete
				waitCmd := exec.Command("kubectl", "rollout", "status", "deployment/cluster-manager", "-n", "default", "--timeout=60s")
				_ = waitCmd.Run()
				restartPF()
			}()

			By("Downloading kubeconfig with new TTL")
			params := api.GetV2ClustersNameKubeconfigsParams{}
			params.Activeprojectid = testTenantID

			resp, err := cli.GetV2ClustersNameKubeconfigsWithResponse(context.Background(), clusterName, &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))

			kubeconfig := *resp.JSON200.Kubeconfig

			// Extract token from kubeconfig
			tokenStart := strings.Index(kubeconfig, "token: ")
			Expect(tokenStart).To(BeNumerically(">", 0), "Token not found in kubeconfig")

			tokenSub := kubeconfig[tokenStart+7:]
			tokenEnd := strings.Index(tokenSub, "\n")
			if tokenEnd == -1 {
				tokenEnd = len(tokenSub)
			}
			tokenString := strings.TrimSpace(tokenSub[:tokenEnd])

			By("Verifying token expiration")
			token, _, err := jwt.NewParser().ParseUnverified(tokenString, jwt.MapClaims{})
			Expect(err).ToNot(HaveOccurred())

			claims, ok := token.Claims.(jwt.MapClaims)
			Expect(ok).To(BeTrue())

			exp, ok := claims["exp"].(float64)
			Expect(ok).To(BeTrue(), "exp claim not found")

			iat, ok := claims["iat"].(float64)
			Expect(ok).To(BeTrue(), "iat claim not found")

			duration := time.Duration(exp-iat) * time.Second
			// 5 hours = 18000 seconds
			expectedDuration := 5 * time.Hour
			Expect(duration).To(BeNumerically("~", expectedDuration, 1*time.Minute), "Token TTL should be 5 hours")
		})

		It("Should delete label", func() {
			params := api.PutV2ClustersNameLabelsParams{}
			params.Activeprojectid = testTenantID
			body := api.PutV2ClustersNameLabelsJSONRequestBody{
				Labels: &map[string]string{"default-extension": "baseline"},
			}
			resp, err := cli.PutV2ClustersNameLabelsWithResponse(context.Background(), clusterName, &params, body)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))

		})

		It("Deleted label should be missing in label list", func() {
			params := api.GetV2ClustersNameParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.GetV2ClustersNameWithResponse(context.Background(), clusterName, &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(200))
			Expect(*resp.JSON200.Name).To(Equal(clusterName))
			Expect(*resp.JSON200.Labels).To(HaveLen(1))
			Expect(*resp.JSON200.Labels).To(HaveKeyWithValue("default-extension", "baseline"))
		})

		It("Should fail to delete cluster template if cluster is running", func() {
			params := api.DeleteV2TemplatesNameVersionParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.DeleteV2TemplatesNameVersionWithResponse(context.Background(), templateOnlyName, templateOnlyVersion, &params)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode()).To(Equal(409))
			Expect(resp.JSON409).ToNot(BeNil())
			Expect(*resp.JSON409.Message).To(ContainSubstring("clusterTemplate is in use"))
		})

		if !deleteCluster {
			fmt.Println("Cluster will not be deleted after the tests. Skipping the deletion test.")
			return
		}

		It("Should delete the cluster", func() {
			params := api.DeleteV2ClustersNameParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.DeleteV2ClustersNameWithResponse(context.Background(), clusterName, &params)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode()).To(Equal(204))
		})

		It("Should return 200 and empty list of clusters on /v2/clusters", func() {
			params := api.GetV2ClustersParams{}
			params.Activeprojectid = testTenantID

			// it takes some time for the cluster to be deleted so we retry the request until it succeeds with a 30 second timeout
			var resp *api.GetV2ClustersResponse
			var err error
			Eventually(func() (int, error) {
				resp, err = cli.GetV2ClustersWithResponse(context.Background(), &params)
				if err != nil {
					return 0, err
				}
				if resp.StatusCode() != 200 {
					return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
				}

				return int(resp.JSON200.TotalElements), nil
			}, 30*time.Second, 3*time.Second).Should(Equal(0))

			Expect(*resp.JSON200.Clusters).To(HaveLen(0))
		})

		It("Should delete cluster template if no cluster is running", func() {
			params := api.DeleteV2TemplatesNameVersionParams{}
			params.Activeprojectid = testTenantID
			resp, err := cli.DeleteV2TemplatesNameVersionWithResponse(context.Background(), templateOnlyName, templateOnlyVersion, &params)
			Expect(err).ToNot(HaveOccurred())

			Expect(resp.StatusCode()).To(Equal(http.StatusNoContent))
		})
	})

	// metrics
	Context("Metrics are generated", func() {
		It("Should return 200 on /metrics", func() {
			resp, err := http.Get("http://" + cmAddress + "/metrics")
			Expect(err).ToNot(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())

			// check if the body contains the expected metrics
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"200\",method=\"GET\",path=\"/v2/clusters\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"200\",method=\"GET\",path=\"/v2/clusters/test-cluster\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"204\",method=\"DELETE\",path=\"/v2/clusters/test-cluster\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"204\",method=\"DELETE\",path=\"/v2/templates/baseline-k3s/v0.0.10\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"409\",method=\"DELETE\",path=\"/v2/templates/baseline-k3s/v0.0.10\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.005\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.01\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.025\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.05\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.1\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.25\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"0.5\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"1\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"2.5\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"5\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"10\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_bucket{le=\"+Inf\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_sum"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_time_seconds_histogram_count"))
		})
	})
})

// loadTemplateInfoFromFile loads a template info from a file
func loadTemplateInfoFromFile(filePath string) (api.TemplateInfo, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return api.TemplateInfo{}, err
	}
	defer file.Close()

	var t api.TemplateInfo
	err = json.NewDecoder(file).Decode(&t)
	if err != nil {
		return api.TemplateInfo{}, err
	}

	return t, nil
}

// resolvePath resolves a relative path to an absolute path
func resolvePath(relativePath string) (string, error) {
	// Get the current working directory
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Join the current working directory with the relative path
	absolutePath := filepath.Join(pwd, relativePath)

	// Clean the path to remove any redundant elements
	absolutePath = filepath.Clean(absolutePath)

	return absolutePath, nil
}

// containsLabels checks if the cluster has the expected labels
func containsLabels(projectId, clusterName string, expectedLabels []string) error {
	cmd := exec.Command("kubectl", "-n", testTenantID.String(), "get", "cl", clusterName, "-o", "custom-columns=LABELS:.metadata.labels")
	out, err := cmd.Output()
	if err != nil {
		return err
	}

	for _, label := range expectedLabels {
		if !bytes.Contains(out, []byte(label)) {
			return fmt.Errorf("label `%s` not found", label)
		}
	}
	return nil
}

// k -n b2bb1f0a-81e6-44f6-a32c-0b5dbb0830ad annotate DockerMachines -l cluster.x-k8s.io/cluster-name=test-cluster host-id=abcd
func annotateDockerMachines(projectId, clusterName string, annotations string) error {
	cmd := exec.Command("kubectl", "-n", projectId, "annotate", "DockerMachines", "-l", fmt.Sprintf("cluster.x-k8s.io/cluster-name=%s", clusterName), annotations)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func createAuthenticatedClient() (*api.ClientWithResponses, error) {
	if os.Getenv("DISABLE_AUTH") == "true" {
		return api.NewClientWithResponses("http://" + cmAddress)
	}

	fmt.Println("creating authenticated client")
	// if auth is enabled, add Bearer token to all requests
	return api.NewClientWithResponses("http://"+cmAddress, api.WithRequestEditorFn(
		func(ctx context.Context, req *http.Request) error {
			token, err := getTokenFromClusterKeycloak()
			if err != nil {
				fmt.Printf("Failed to get token: %v\n", err)
				return err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		},
	))
}

func getTokenFromClusterKeycloak() (string, error) {
	// include project-specific roles that OPA expects
	roles := []string{
		fmt.Sprintf("%s_cl-tpl-rw", testTenantID.String()),
		fmt.Sprintf("%s_cl-tpl-r", testTenantID.String()),
		fmt.Sprintf("%s_cl-rw", testTenantID.String()),
		fmt.Sprintf("%s_cl-r", testTenantID.String()),
	}
	token := helpers.CreateTestJWT(time.Now().Add(1*time.Hour), roles)

	return token, nil
}
