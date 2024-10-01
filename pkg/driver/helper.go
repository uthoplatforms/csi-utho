package driver

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/exp/rand"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Get current Node Id from k8s node label
func GetNodeId() (string, error) {
	// Retrieve the current node name from the environment variable
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		return "", fmt.Errorf("NODE_NAME environment variable not set")
	}

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
	nodepoolID, found := node.Labels["nodepool_id"]
	if !found {
		return "", fmt.Errorf("nodepool_id label not found on node %s", nodeName)
	}

	return nodepoolID, nil
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
