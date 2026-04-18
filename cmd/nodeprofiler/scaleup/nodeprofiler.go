// Package scaleup provides tooling to profile Kubernetes node scale-up events.
package scaleup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	// ErrNoKubeconfig is returned when no kubeconfig is found.
	ErrNoKubeconfig = errors.New("KUBECONFIG not set and default config file does not exist")
)

// NodeProfiler handles node watching and profiling.
type NodeProfiler struct {
	clientset *kubernetes.Clientset
	timeline  *Timeline
	logger    *zap.Logger
}

// NewNodeProfiler creates a new profiler.
func NewNodeProfiler(logger *zap.Logger) (*NodeProfiler, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		// Default to ~/.kube/config if KUBECONFIG is not set.
		homedir := homedir.HomeDir()
		kubeconfig = homedir + "/.kube/config"

		_, statErr := os.Stat(kubeconfig)
		if os.IsNotExist(statErr) {
			return nil, fmt.Errorf("failed to find kubeconfig: %w: %s", ErrNoKubeconfig, kubeconfig)
		}
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &NodeProfiler{
		clientset: clientset,
		logger:    logger,
		timeline:  NewTimeline(),
	}, nil
}

// Run begins watching nodes and profiling the first added node.
func (np *NodeProfiler) Run(ctx context.Context) error {
	np.logger.Info("Watching for new node creation")

	// Get the list of existing nodes to avoid treating them as new.
	existingNodes, err := np.clientset.CoreV1().Nodes().List(ctx, meta.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list existing nodes: %w", err)
	}

	// Create a map of existing node names for quick lookup.
	existingNodeMap := make(map[string]bool)
	for _, node := range existingNodes.Items {
		existingNodeMap[node.Name] = true
	}

	watcher, err := np.clientset.CoreV1().Nodes().Watch(ctx, meta.ListOptions{
		Watch:         true,
		FieldSelector: fields.Everything().String(),
	})
	if err != nil {
		return fmt.Errorf("failed to watch nodes: %w", err)
	}

	waitGroup := &sync.WaitGroup{}

	for event := range watcher.ResultChan() {
		if event.Type == watch.Added {
			node, ok := event.Object.(*v1.Node)
			if !ok {
				np.logger.Warn("Received invalid node type", zap.Any("type", reflect.TypeOf(event.Object)))

				continue
			}

			// Skip this node if it already existed when we started watching
			if existingNodeMap[node.Name] {
				np.logger.Info("Skipping existing node", zap.String("node", node.Name))

				continue
			}

			np.timeline.Add(time.Now(), "Node/Event", "NodeAdded").Log(np.logger, zap.String("node", node.Name))

			waitGroup.Go(func() {
				np.recordEvents(ctx, node.Name)
			})

			waitGroup.Go(func() {
				np.recordConditions(ctx, node.Name)
			})

			// wg.Go(func() {
			// 	np.recordLabels(ctx, node.Name)
			// })

			// wg.Go(func() {
			// 	np.recordPods(ctx, node.Name)
			// })

			// Only accept the first node added.
			watcher.Stop()

			break
		}
	}

	waitGroup.Wait()

	return nil
}

// Print outputs the timeline to the console in a tabular format.
func (np *NodeProfiler) Print(_ context.Context) error {
	_, _ = fmt.Fprintf(os.Stdout, "\n\n")

	np.timeline.Print()

	return nil
}

// recordEvents watches for events related to the specified node and logs them to the timeline.
// It listens for Kubernetes events involving the node and records their reasons until the context is cancelled.
func (np *NodeProfiler) recordEvents(ctx context.Context, nodeName string) {
	watcher, err := np.clientset.CoreV1().Events("").Watch(ctx, meta.ListOptions{
		FieldSelector: fields.AndSelectors(
			fields.OneTermEqualSelector("involvedObject.kind", "Node"),
			fields.OneTermEqualSelector("involvedObject.name", nodeName),
		).String(),
		Watch: true,
	})
	if err != nil {
		np.logger.Error("Failed to watch node events", zap.Error(err))

		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			np.logger.Info("Stopping event watcher", zap.String("reason", "ContextCancelled"))

			return
		case rawEvent := <-watcher.ResultChan():
			event, ok := rawEvent.Object.(*v1.Event)
			if !ok {
				np.logger.Warn("Received invalid event type", zap.Any("type", reflect.TypeOf(rawEvent.Object)))

				continue
			}

			np.timeline.Add(time.Now(), "Node/Event", event.Reason).Log(np.logger)

			// Timeout to avoid blocking indefinitely.
			continue
		}
	}
}

func (np *NodeProfiler) recordConditionTransition(
	cond v1.NodeCondition,
	conditionStates map[v1.NodeConditionType]v1.ConditionStatus,
) {
	prevStatus, exists := conditionStates[cond.Type]
	if !exists || prevStatus != cond.Status {
		transitionMsg := fmt.Sprintf("%s: %s", cond.Type, cond.Status)
		if exists {
			transitionMsg = fmt.Sprintf("%s: %s >> %s", cond.Type, prevStatus, cond.Status)
		}

		np.timeline.Add(time.Now(), "Node/Condition", transitionMsg).Log(np.logger)

		conditionStates[cond.Type] = cond.Status
	}
}

// recordConditions watches for changes in node conditions and logs them to the timeline.
// It listens for node condition updates and records transitions in their status until the context is cancelled.
// It tracks the previous state of each condition to detect transitions.
// If a condition changes, it logs the transition with the previous and current status.
// If a condition is added for the first time, it logs the initial state.
func (np *NodeProfiler) recordConditions(ctx context.Context, nodeName string) {
	watcher, err := np.clientset.CoreV1().Nodes().Watch(ctx, meta.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", nodeName).String(),
	})
	if err != nil {
		np.logger.Error("Failed to watch node conditions", zap.Error(err))

		return
	}
	defer watcher.Stop()

	// Track previous condition states to detect transitions.
	conditionStates := make(map[v1.NodeConditionType]v1.ConditionStatus)

	for {
		select {
		case <-ctx.Done():
			np.logger.Info("Stopping condition watcher due to context cancellation")

			return
		case event, chanOpen := <-watcher.ResultChan():
			if !chanOpen {
				np.logger.Info("Node ci watcher channel closed")

				return
			}

			if event.Type != watch.Modified {
				continue
			}

			node, nodeOK := event.Object.(*v1.Node)
			if !nodeOK {
				continue
			}

			// Check for condition transitions
			for _, cond := range node.Status.Conditions {
				np.recordConditionTransition(cond, conditionStates)
			}
		}
	}
}
