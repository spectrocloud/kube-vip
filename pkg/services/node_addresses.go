package services

import (
	"context"
	"net"

	log "log/slog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// selfNodeStatusIPSet returns this node's reported InternalIP and ExternalIP addresses for LB bind heuristics.
func selfNodeStatusIPSet(ctx context.Context, client kubernetes.Interface, nodeName string) map[string]struct{} {
	out := make(map[string]struct{})
	if client == nil || nodeName == "" {
		return out
	}
	n, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		log.Warn("could not get node for local LB IP detection; skipping Node status IPs", "node", nodeName, "err", err)
		return out
	}
	for _, a := range n.Status.Addresses {
		if a.Type != v1.NodeInternalIP && a.Type != v1.NodeExternalIP {
			continue
		}
		if ip := net.ParseIP(a.Address); ip != nil {
			out[ip.String()] = struct{}{}
		}
	}
	return out
}
