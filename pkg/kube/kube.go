// Package kube provides direct cluster access for capabilities the orcinus
// HTTP API does not expose: streaming pod logs and reading pod metrics. It uses
// the same kubeconfig orcinus writes.
package kube

import (
	"context"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Client wraps the kubernetes + metrics clientsets.
type Client struct {
	Clientset *kubernetes.Clientset
	Metrics   *metricsv.Clientset
	restCfg   *rest.Config
}

// New builds a cluster client from a kubeconfig path.
func New(kubeconfigPath string) (*Client, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	// Metrics are optional (require metrics-server); ignore construction errors.
	mc, _ := metricsv.NewForConfig(cfg)
	return &Client{Clientset: cs, Metrics: mc, restCfg: cfg}, nil
}

// Exec runs a command in a pod container and streams stdout/stderr to the
// provided writers.
func (c *Client) Exec(ctx context.Context, namespace, pod string, command []string, stdout, stderr io.Writer) error {
	req := c.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(pod).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.restCfg, "POST", req.URL())
	if err != nil {
		return err
	}
	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: stdout,
		Stderr: stderr,
	})
}

// StreamLogs streams (optionally following) the logs of a pod to w.
func (c *Client) StreamLogs(ctx context.Context, namespace, pod string, follow bool, tail int64, w io.Writer) error {
	opts := &corev1.PodLogOptions{Follow: follow}
	if tail > 0 {
		opts.TailLines = &tail
	}
	req := c.Clientset.CoreV1().Pods(namespace).GetLogs(pod, opts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()
	_, err = io.Copy(w, stream)
	return err
}

// Node summarizes a cluster node.
type Node struct {
	Name       string `json:"name"`
	Ready      bool   `json:"ready"`
	Roles      string `json:"roles"`
	Version    string `json:"version"`
	OS         string `json:"os"`
	InternalIP string `json:"internalIP"`
}

// Nodes lists the cluster's nodes.
func (c *Client) Nodes(ctx context.Context) ([]Node, error) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var out []Node
	for _, n := range list.Items {
		node := Node{
			Name:    n.Name,
			Version: n.Status.NodeInfo.KubeletVersion,
			OS:      n.Status.NodeInfo.OSImage,
		}
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady {
				node.Ready = c.Status == corev1.ConditionTrue
			}
		}
		for _, a := range n.Status.Addresses {
			if a.Type == corev1.NodeInternalIP {
				node.InternalIP = a.Address
			}
		}
		var roles []string
		for k := range n.Labels {
			if r, ok := strings.CutPrefix(k, "node-role.kubernetes.io/"); ok {
				roles = append(roles, r)
			}
		}
		node.Roles = strings.Join(roles, ",")
		out = append(out, node)
	}
	return out, nil
}

// PodMetric is CPU/memory usage for one pod.
type PodMetric struct {
	Pod         string `json:"pod"`
	CPUMillicores int64 `json:"cpuMillicores"`
	MemoryBytes   int64 `json:"memoryBytes"`
}

// PodMetrics returns usage for pods matching labelSelector in namespace.
// Returns an error if metrics-server is not installed/reachable.
func (c *Client) PodMetrics(ctx context.Context, namespace, labelSelector string) ([]PodMetric, error) {
	if c.Metrics == nil {
		return nil, fmt.Errorf("metrics client unavailable")
	}
	list, err := c.Metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	var out []PodMetric
	for _, pm := range list.Items {
		var cpu, mem int64
		for _, ctr := range pm.Containers {
			cpu += ctr.Usage.Cpu().MilliValue()
			mem += ctr.Usage.Memory().Value()
		}
		out = append(out, PodMetric{Pod: pm.Name, CPUMillicores: cpu, MemoryBytes: mem})
	}
	return out, nil
}
