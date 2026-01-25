package watcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	watchtools "k8s.io/client-go/tools/watch"

	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func (w *Watch) Watch() error {
	var err error
	w.configMapName = w.podname + "-kube-gateway"
	slog.Info("starting watcher", "name", w.configMapName, "namespace", w.namespace)

	c, err := w.client()
	if err != nil {
		return err
	}
	opts := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.name", w.configMapName).String(),
	}

	rw, err := watchtools.NewRetryWatcherWithContext(context.TODO(), "1", &cache.ListWatch{
		WatchFunc: func(_ metav1.ListOptions) (watch.Interface, error) {
			return c.CoreV1().ConfigMaps(string(w.namespace)).Watch(context.Background(), opts)
		},
	})
	if err != nil {
		return fmt.Errorf("error creating annotations watcher: %s", err.Error())
	}

	ch := rw.ResultChan()
	for event := range ch {
		// We need to inspect the event and get ResourceVersion out of it
		switch event.Type {

		case watch.Added, watch.Modified:
			updatedConfigMap, ok := event.Object.(*v1.ConfigMap)
			if !ok {
				slog.Error("configmap", "err", "unable to process")
			}
			slog.Info("configmap updated", "name", updatedConfigMap.Name)
			if updatedConfigMap.Name == w.configMapName && updatedConfigMap.Namespace == string(w.namespace) {
				data := updatedConfigMap.Data["config"]

				err := json.Unmarshal([]byte(data), w.config)
				if err != nil {
					slog.Error("unable to read JSON from configMap", "err", err)
				}
				fmt.Println(data)
			}
		}
	}
	return nil
}
