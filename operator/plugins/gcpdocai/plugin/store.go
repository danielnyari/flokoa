package plugin

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// stateConfigMapName is the name of the ConfigMap used to persist task state.
	stateConfigMapName = "flokoa-gcpdocai-plugin-state"

	// stateConfigMapLabel identifies the ConfigMap as managed by the plugin.
	stateConfigMapLabel = "flokoa.ai/component"
	stateConfigMapValue = "gcpdocai-plugin-state"
)

// StateStore persists GCP Document AI LRO progress state across plugin restarts using a
// ConfigMap as the backing store and a sync.Map as a write-through cache.
// If the Kubernetes client is nil (e.g. in development), it falls back to
// in-memory-only storage.
type StateStore struct {
	cache     sync.Map
	mu        sync.Mutex // serializes ConfigMap writes to coalesce concurrent syncs
	k8sClient client.Client
	namespace string
}

// NewStateStore creates a StateStore. If k8sClient is nil, the store operates
// in memory only. The namespace is used for the backing ConfigMap.
func NewStateStore(k8sClient client.Client, namespace string) *StateStore {
	s := &StateStore{
		k8sClient: k8sClient,
		namespace: namespace,
	}

	// Load existing state from ConfigMap on startup
	if k8sClient != nil && namespace != "" {
		s.loadFromConfigMap()
	}

	return s
}

// Store persists the progress state for the given key.
func (s *StateStore) Store(key string, state *ProgressState) {
	s.cache.Store(key, state)
	s.syncToConfigMap()
}

// Load retrieves the progress state for the given key.
func (s *StateStore) Load(key string) (*ProgressState, bool) {
	val, ok := s.cache.Load(key)
	if !ok {
		return nil, false
	}
	state, ok := val.(*ProgressState)
	return state, ok
}

// Delete removes the progress state for the given key.
func (s *StateStore) Delete(key string) {
	s.cache.Delete(key)
	s.syncToConfigMap()
}

// cmKey converts a task key (e.g. "uid/template") into a valid ConfigMap data
// key by replacing '/' with '_'.
func cmKey(key string) string {
	return strings.ReplaceAll(key, "/", "_")
}

// cmKeyToTaskKey recovers the original task key from a ConfigMap data key.
func cmKeyToTaskKey(k string) string {
	return strings.Replace(k, "_", "/", 1)
}

// loadFromConfigMap restores cached state from the backing ConfigMap.
func (s *StateStore) loadFromConfigMap() {
	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: stateConfigMapName, Namespace: s.namespace}
	if err := s.k8sClient.Get(context.Background(), key, cm); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Printf("Warning: failed to load GCP DocAI plugin state ConfigMap: %v", err)
		}
		return
	}

	count := 0
	for k, v := range cm.Data {
		var state ProgressState
		if err := json.Unmarshal([]byte(v), &state); err != nil {
			log.Printf("Warning: failed to unmarshal state for key %s: %v", k, err)
			continue
		}
		s.cache.Store(cmKeyToTaskKey(k), &state)
		count++
	}

	if count > 0 {
		log.Printf("Restored %d GCP DocAI LRO state(s) from ConfigMap", count)
	}
}

// syncToConfigMap writes all cached state to the backing ConfigMap.
func (s *StateStore) syncToConfigMap() {
	if s.k8sClient == nil || s.namespace == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data := make(map[string]string)
	s.cache.Range(func(key, value any) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		state, ok := value.(*ProgressState)
		if !ok {
			return true
		}
		b, err := json.Marshal(state)
		if err != nil {
			log.Printf("Warning: failed to marshal state for key %s: %v", k, err)
			return true
		}
		data[cmKey(k)] = string(b)
		return true
	})

	cm := &corev1.ConfigMap{}
	key := types.NamespacedName{Name: stateConfigMapName, Namespace: s.namespace}
	err := s.k8sClient.Get(context.Background(), key, cm)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Printf("Warning: failed to get GCP DocAI plugin state ConfigMap: %v", err)
			return
		}
		// Create the ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stateConfigMapName,
				Namespace: s.namespace,
				Labels: map[string]string{
					stateConfigMapLabel: stateConfigMapValue,
				},
			},
			Data: data,
		}
		if createErr := s.k8sClient.Create(context.Background(), cm); createErr != nil {
			log.Printf("Warning: failed to create GCP DocAI plugin state ConfigMap: %v", createErr)
		}
		return
	}

	// Update existing ConfigMap
	cm.Data = data
	if updateErr := s.k8sClient.Update(context.Background(), cm); updateErr != nil {
		log.Printf("Warning: failed to update GCP DocAI plugin state ConfigMap: %v", updateErr)
	}
}
