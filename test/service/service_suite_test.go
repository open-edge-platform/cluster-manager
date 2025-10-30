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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
		fmt.Println("auth is enabled - generating test keys for JWT signing")

		// Run the key generation script
		scriptPath := "../../test/helpers/generate-test-keys.sh"
		cmd := exec.Command("bash", scriptPath)
		cmd.Env = append(os.Environ(), "KEY_DIR=/tmp/cluster-manager-test-keys")
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to generate test keys: %v\nOutput: %s\n", err, string(output))
			Expect(err).ToNot(HaveOccurred())
		}
		fmt.Printf("Test keys generated:\n%s\n", string(output))

		// Update the mock Keycloak ConfigMap with the generated JWK
		jwkPath := "/tmp/cluster-manager-test-keys/test-jwk.json"
		jwkData, err := os.ReadFile(jwkPath)
		if err != nil {
			fmt.Printf("Failed to read JWK file: %v\n", err)
			Expect(err).ToNot(HaveOccurred())
		}

		fmt.Println("Updating mock Keycloak ConfigMap with generated JWK...")
		// Patch the ConfigMap with the new JWK
		patchCmd := fmt.Sprintf(`{"data":{"jwks.json":%q}}`, string(jwkData))
		cmd = exec.Command("kubectl", "patch", "configmap", "mock-keycloak-config",
			"-n", "orch-platform",
			"--type=merge",
			"-p", patchCmd)
		output, err = cmd.CombinedOutput()
		if err != nil {
			fmt.Printf("Failed to patch ConfigMap: %v\nOutput: %s\n", err, string(output))
		} else {
			fmt.Println("ConfigMap patched successfully")
		}

		// Restart the mock Keycloak pod to pick up the new ConfigMap
		fmt.Println("Restarting mock Keycloak to pick up new JWK...")
		cmd = exec.Command("kubectl", "rollout", "restart", "deployment/platform-keycloak", "-n", "orch-platform")
		err = cmd.Run()
		if err != nil {
			fmt.Printf("Failed to restart deployment: %v\n", err)
		}

		cmd = exec.Command("kubectl", "rollout", "status", "deployment/platform-keycloak", "-n", "orch-platform", "--timeout=60s")
		err = cmd.Run()
		if err != nil {
			fmt.Printf("Failed waiting for deployment: %v\n", err)
		}

		// Note: We don't need to restart cluster-manager because:
		// 1. The dynamic kid (with timestamp) forces JWKS cache refresh automatically
		// 2. cluster-manager's GetSigningKey() auto-refreshes JWKS when kid not found
		// 3. Restarting would break any existing port-forwards

		fmt.Println("auth is enabled - waiting for mock Keycloak to be ready")
		Eventually(func() error {
			_, err := getTokenFromClusterKeycloak()
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed())
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

	// Clean up dynamically generated test keys
	if os.Getenv("DISABLE_AUTH") == "false" {
		keyDir := os.Getenv("KEY_DIR")
		if keyDir == "" {
			keyDir = "/tmp/cluster-manager-test-keys"
		}

		fmt.Printf("Cleaning up test keys from %s...\n", keyDir)
		if err := os.RemoveAll(keyDir); err != nil {
			fmt.Printf("Warning: Failed to clean up test keys: %v\n", err)
		} else {
			fmt.Println("Test keys cleaned up successfully")
		}
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
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"200\",method=\"GET\",path=\"/v2/templates\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"201\",method=\"POST\",path=\"/v2/clusters\"}"))
			Expect(string(body)).To(ContainSubstring("cluster_manager_http_response_codes_counter{code=\"201\",method=\"POST\",path=\"/v2/templates\"}"))
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
	// If auth is disabled, use regular client
	if os.Getenv("DISABLE_AUTH") == "true" {
		fmt.Println("Auth disabled, using regular client")
		return api.NewClientWithResponses("http://" + cmAddress)
	}

	fmt.Println("Auth enabled, creating authenticated client")
	// If auth is enabled, add Bearer token to all requests
	return api.NewClientWithResponses("http://"+cmAddress, api.WithRequestEditorFn(
		func(ctx context.Context, req *http.Request) error {
			// Get token from your mock Keycloak service running in the cluster
			fmt.Println("Getting token from Keycloak...")
			token, err := getTokenFromClusterKeycloak()
			if err != nil {
				fmt.Printf("Failed to get token: %v\n", err)
				return err
			}
			if len(token) > 20 {
				fmt.Printf("Got token: %s...\n", token[:20])
			} else {
				fmt.Printf("Got token (length %d): %s\n", len(token), token)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		},
	))
}

func getTokenFromClusterKeycloak() (string, error) {
	// First, verify that the mock Keycloak is reachable (health check)
	keycloakURL := "http://localhost:8081"

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", "test-client")
	data.Set("client_secret", "test-secret")

	fmt.Printf("Attempting to connect to Keycloak at: %s\n", keycloakURL)

	resp, err := http.PostForm(keycloakURL+"/realms/master/protocol/openid-connect/token", data)
	if err != nil {
		return "", fmt.Errorf("failed to connect to keycloak at %s: %w", keycloakURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("keycloak returned status %d: %s", resp.StatusCode, string(body))
	}

	// Generate a proper JWT token signed with our test private key
	// This ensures the token is valid and can be verified by the cluster-manager
	// Include project-specific roles that OPA expects
	roles := []string{
		fmt.Sprintf("%s_cl-tpl-rw", testTenantID.String()), // Template write access
		fmt.Sprintf("%s_cl-tpl-r", testTenantID.String()),  // Template read access
		fmt.Sprintf("%s_cl-rw", testTenantID.String()),     // Cluster write access
		fmt.Sprintf("%s_cl-r", testTenantID.String()),      // Cluster read access
	}
	token := helpers.CreateTestJWT(time.Now().Add(1*time.Hour), roles)

	return token, nil
}
