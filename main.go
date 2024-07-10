package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	var namespace, service, nodesFile, kubeConfig string
	var peerPort, apiPort int

	flag.StringVar(&kubeConfig, "kubeconfig", "config", "kubeconfig file in ~/.kube to work with")
	flag.StringVar(&namespace, "namespace", "typesense", "namespace that typesense is installed within")
	flag.StringVar(&service, "service", "typesense-svc", "name of the typesense service to use the endpoints of")
	flag.StringVar(&nodesFile, "nodes-file", "/usr/share/typesense/nodes", "location of the file to write node information to")
	flag.IntVar(&peerPort, "peer-port", 8107, "port on which typesense peering service listens")
	flag.IntVar(&apiPort, "api-port", 8108, "port on which typesense API service listens")
	flag.Parse()

	log.Printf("watching endpoints for service: %s/%s [peerPort: %d, apiPort: %d]\n", namespace, service, peerPort, apiPort)
	configPath := filepath.Join(homedir.HomeDir(), ".kube", kubeConfig)

	var config *rest.Config
	var err error

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		// No config file found, fall back to in-cluster config.
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Printf("failed to build local config: %s\n", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", configPath)
		if err != nil {
			log.Printf("failed to build in-cluster config: %s\n", err)
		}
	}

	clients, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("failed to create kubernetes client: %s\n", err)
	}

	watcher, err := clients.CoreV1().Endpoints(namespace).Watch(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("failed to create endpoints watcher: %s\n", err)
	}

	for range watcher.ResultChan() {
		err := os.WriteFile(nodesFile, []byte(getNodes(clients, namespace, service, peerPort, apiPort)), 0666)
		if err != nil {
			log.Printf("failed to write nodes file: %s\n", err)
		}
	}
}

func getNodes(clients *kubernetes.Clientset, namespace, service string, peerPort int, apiPort int) string {
	var nodes []string

	endpoints, err := clients.CoreV1().Endpoints(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("failed to list endpoints: %s\n", err)
		return ""
	}

	for _, e := range endpoints.Items {
		if e.Name != service {
			continue
		}

		for _, s := range e.Subsets {
			addresses := s.Addresses
			if s.Addresses == nil || len(s.Addresses) == 0 {
				addresses = s.NotReadyAddresses
			}
			for _, a := range addresses {
				for _, p := range s.Ports {
					// Typesense exporter sidecar for Prometheus runs on port 9000
					if int(p.Port) == apiPort {
						nodes = append(nodes, fmt.Sprintf("%s:%d:%d", a.IP, peerPort, p.Port))
					}
				}
			}
		}
	}

	typesenseNodes := strings.Join(nodes, ",")

	if len(nodes) != 0 {
		log.Printf("New %d node configuration: %s\n", len(nodes), typesenseNodes)
	}

	return typesenseNodes
}
