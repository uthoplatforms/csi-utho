package driver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/uthoplatforms/utho-go/utho"
	"golang.org/x/exp/rand"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Get current Node Id from k8s node label
func GetNodeId(client utho.Client) (string, error) {
	// Retrieve the current node name from the environment variable
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		return "", fmt.Errorf("NODE_NAME environment variable not set")
	}
	fmt.Printf("env nodeName: '%s'\n", nodeName)

	config, err := rest.InClusterConfig()
	if err != nil {
		return "", fmt.Errorf("error creating in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("error creating Kubernetes client: %w", err)
	}

	// Get the node object for the current node
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error retrieving node: %w", err)
	}

	// Retrieve the nodepool_id label
	cluster_id, found := node.Labels["cluster_id"]
	fmt.Printf("cluster id '%s'\n", cluster_id)
	if !found {
		return "", fmt.Errorf("cluster_id label not found on node '%s'", nodeName)
	}
	fmt.Printf("cluster_id: '%s'\n", cluster_id)
	nodepool_id, found := node.Labels["nodepool_id"]
	fmt.Printf("nodepool id '%s'\n", nodepool_id)
	if !found {
		return "", fmt.Errorf("nodepool_id label not found on node '%s'", nodeName)
	}
	fmt.Printf("nodepool_id: '%s'\n", nodepool_id)

	k8s, err := client.Kubernetes().Read(cluster_id)
	if err != nil {
		return "", fmt.Errorf("error retrieving Kubernetes with id '%s' %w", cluster_id, err)
	}

	var node_id string

	if nodepool, exists := k8s.Nodepools[nodepool_id]; exists {
		for _, node := range nodepool.Workers {
			hostName := node.Hostname
			fmt.Printf("Node hostName: '%s'\n", hostName)
			fmt.Printf("nodeName: '%s'\n", nodeName)

			if strings.EqualFold(hostName, nodeName) {
				node_id = node.Cloudid
				fmt.Printf("node_id inside if: '%s'\n", node_id)
				fmt.Printf("node name if: '%s'=>'%s'\n", hostName, nodeName)
				break
			}
			fmt.Printf("node_id outside if: '%s'\n", node_id)
		}
	} else {
		fmt.Printf("node with name '%s' does not exist in the NodePool '%s'.\n", nodeName, nodepool_id)
	}

	fmt.Printf("node id '%s'\n", node_id)

	return node_id, nil
}

func GenerateRandomString(length int) string {
	rand.Seed(uint64(time.Now().UnixNano()))

	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)

	for i := 0; i < length; i++ {
		result[i] = chars[rand.Intn(len(chars))]
	}

	return string(result)
}
